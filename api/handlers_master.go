package api

import (
	"database/sql"
	"encoding/json"
	stderr "errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/deploy"
	"github.com/docker/swarm/garden/resource"
	"github.com/docker/swarm/garden/resource/alloc/driver"
	"github.com/docker/swarm/garden/resource/storage"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/utils"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	goctx "golang.org/x/net/context"
)

var (
	nilGardenCode = errCode{
		code:    100000000,
		comment: "internal error,not supported 'Garden'",
		chinese: "不支持 'Garden' 模式",
	}
	errUnsupportGarden = stderr.New("unsupport Garden yet")
)

func httpJSONNilGarden(w http.ResponseWriter) {
	httpJSONError(w, errUnsupportGarden, nilGardenCode, http.StatusInternalServerError)
}

// Emit an HTTP error and log it.
func httpJSONError(w http.ResponseWriter, err error, ec errCode, status int) {
	field := logrus.WithFields(logrus.Fields{
		"status": status,
		"code":   ec.code,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err != nil {

		json.NewEncoder(w).Encode(structs.ResponseHead{
			Result:   false,
			Code:     ec.code,
			Error:    err.Error(),
			Category: ec.chinese,
		})

		field.Errorf("HTTP error: %+v", err)
	}
}

func writeJSON(w http.ResponseWriter, obj interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if obj != nil {
		err := json.NewEncoder(w).Encode(obj)
		if err != nil {
			logrus.Errorf("write JSON:%d,%s", status, err)
		}
	}
}

func writeJSONNull(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprint(w, "{}")
}

func writeJSONFprintf(w http.ResponseWriter, status int, format string, args ...interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, format, args...)
}

// -----------------/nfs_backups handlers-----------------
func vaildNFSParams(nfs database.NFS) error {
	errs := make([]string, 0, 4)

	if nfs.Addr == "" {
		errs = append(errs, "nfs:Addr is required")
	}

	if nfs.Dir == "" {
		errs = append(errs, "nfs:Dir is required")
	}

	if nfs.MountDir == "" {
		errs = append(errs, "nfs:Mount Dir is required")
	}

	if nfs.Options == "" {
		errs = append(errs, "nfs:Option is required")
	}

	if len(errs) == 0 {
		return nil
	}

	return fmt.Errorf("nfs:%v,%s", nfs, errs)
}

func getNFSSPace(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ec := errCodeV1(_NFS, urlParamError, 11, "parse Request URL parameter error", "解析请求URL参数错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	nfs := database.NFS{}

	nfs.Addr = r.FormValue("nfs_ip")
	nfs.Dir = r.FormValue("nfs_dir")
	nfs.MountDir = r.FormValue("nfs_mount_dir")
	nfs.Options = r.FormValue("nfs_mount_opts")

	err := vaildNFSParams(nfs)
	if err != nil {
		ec := errCodeV1(_NFS, invaildParamsError, 12, "URL parameters are invaild", "URL参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	sys, err := gd.Ormer().GetSysConfig()
	if err != nil {
		ec := errCodeV1(_NFS, dbQueryError, 13, "fail to query database", "数据库查询错误（配置参数表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	abs, err := utils.GetAbsolutePath(true, sys.SourceDir)
	if err != nil {
		ec := errCodeV1(_NFS, objectNotExist, 14, sys.SourceDir+":dir is not exist", sys.SourceDir+":目录不存在")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	d := driver.NewNFSDriver(nfs, filepath.Dir(abs), sys.BackupDir)

	space, err := d.Space()
	if err != nil {
		ec := errCodeV1(_NFS, internalError, 15, "fail to get NFS space info", "NFS 容量信息查询错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusOK, `{"total_space": %d,"free_space": %d}`, space.Total, space.Free)
}

// -----------------/tasks handlers-----------------
func getTask(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	name := mux.Vars(r)["name"]

	t, err := gd.Ormer().GetTask(name)
	if err != nil {
		if errors.Cause(err) == sql.ErrNoRows {
			writeJSONNull(w, http.StatusOK)
			return
		}

		ec := errCodeV1(_Task, dbQueryError, 11, "fail to query database", "数据库查询错误（任务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSON(w, t, http.StatusOK)
}

func getTasks(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ec := errCodeV1(_Task, urlParamError, 21, "parse Request URL parameter error", "解析请求URL参数错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	var (
		err error
		out []database.Task
	)

	all := boolValue(r, "all")
	if all {
		out, err = gd.Ormer().ListTasks("", 0)
	} else {
		status := intValueOrZero(r, "status")
		link := r.FormValue("link")

		out, err = gd.Ormer().ListTasks(link, status)
	}

	if err != nil {
		ec := errCodeV1(_Task, dbQueryError, 22, "fail to query database", "数据库查询错误（任务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSON(w, out, http.StatusOK)
}

func vaildBackupTaskCallback(bt structs.BackupTaskCallback) error {
	errs := make([]string, 0, 3)

	if bt.UnitID == "" {
		errs = append(errs, "Unit is required")
	}

	if bt.TaskID == "" {
		errs = append(errs, "TaskID is required")
	}

	if bt.Path == "" {
		errs = append(errs, "Path is required")
	}

	if len(errs) == 0 {
		return nil
	}

	return fmt.Errorf("BackupTaskCallback errors:%v,%s", bt, errs)
}

func postBackupCallback(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.BackupTaskCallback{}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		ec := errCodeV1(_Task, decodeError, 31, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	err = vaildBackupTaskCallback(req)
	if err != nil {
		ec := errCodeV1(_Task, invaildParamsError, 32, "Body parameters are invaild", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil || gd.Ormer() == nil {
		httpJSONNilGarden(w)
		return
	}

	if req.Retention == 0 {
		// default keep a week
		req.Retention = 7
	}
	orm := gd.Ormer()
	now := time.Now()

	bf := database.BackupFile{
		ID:         utils.Generate32UUID(),
		TaskID:     req.TaskID,
		UnitID:     req.UnitID,
		Type:       req.Type,
		Path:       req.Path,
		SizeByte:   req.Size,
		Retention:  now.AddDate(0, 0, req.Retention),
		CreatedAt:  now,
		FinishedAt: now,
	}

	t := database.Task{}
	t.ID = req.TaskID
	t.Status = database.TaskDoneStatus
	t.SetErrors(nil)
	t.FinishedAt = now

	err = orm.InsertBackupFileWithTask(bf, t)
	if err != nil {
		ec := errCodeV1(_Task, dbTxError, 34, "fail to exec records in into database in a Tx", "数据库事务处理错误（备份文件表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// -----------------/datacenter handler-----------------
func vaildDatacenter(v structs.RegisterDC) error {
	// TODO:
	return nil
}

// TODO:remove this?
func postRegisterDC(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.RegisterDC{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		ec := errCodeV1(_DC, decodeError, 11, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if err := vaildDatacenter(req); err != nil {
		ec := errCodeV1(_DC, invaildParamsError, 12, "Body parameters are invaild", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil {
		httpJSONNilGarden(w)
		return
	}

	err = gd.Register(req)
	if err != nil {
		ec := errCodeV1(_DC, internalError, 13, "fail to register datacenter", "注册数据中心错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func getSystemConfig(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil {
		httpJSONNilGarden(w)
		return
	}

	sys, err := gd.Ormer().GetSysConfig()
	if err != nil {
		if errors.Cause(err) == sql.ErrNoRows {
			writeJSONNull(w, http.StatusOK)
			return
		}

		ec := errCodeV1(_DC, dbQueryError, 21, "fail to query database", "数据库查询错误（配置参数表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSON(w, sys, http.StatusOK)
}

// -----------------/softwares/images handlers-----------------
func listImages(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	images, err := gd.Ormer().ListImages()
	if err != nil {
		ec := errCodeV1(_Image, dbQueryError, 11, "fail to query database", "数据库查询错误（镜像表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	out := make([]structs.ImageResponse, len(images))

	for i := range images {
		out[i] = structs.ImageResponse{
			ImageVersion: structs.ImageVersion{
				Name:  images[i].Name,
				Major: images[i].Major,
				Minor: images[i].Minor,
				Patch: images[i].Patch,
				Build: images[i].Build,
			},
			Size:     images[i].Size,
			ID:       images[i].ID,
			ImageID:  images[i].ImageID,
			Labels:   images[i].Labels,
			UploadAt: utils.TimeToString(images[i].UploadAt),
		}
	}

	writeJSON(w, out, http.StatusOK)
}

func getSupportImages(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.PluginClient() == nil {

		httpJSONNilGarden(w)
		return
	}

	pc := gd.PluginClient()
	out, err := pc.GetImageSupport(ctx)
	if err != nil {
		ec := errCodeV1(_Image, internalError, 21, "fail to get supported image list", "获取已支持的镜像列表错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSON(w, out, http.StatusOK)
}

func vaildLoadImageRequest(v structs.PostLoadImageRequest) error {
	errs := make([]string, 0, 2)

	if v.Name == "" ||
		(v.Major == 0 && v.Minor == 0 &&
			v.Patch == 0 && v.Build == 0) {
		errs = append(errs, "ImageVersion is required")
	}

	if v.Path == "" {
		errs = append(errs, "Path is required")
	}

	if len(errs) == 0 {
		return nil
	}

	return fmt.Errorf("PostLoadImageRequest:%v,%s", v, errs)
}

func postImageLoad(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ec := errCodeV1(_Image, urlParamError, 31, "parse Request URL parameter error", "解析请求URL参数错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	timeout := intValueOrZero(r, "timeout")

	req := structs.PostLoadImageRequest{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		ec := errCodeV1(_Image, decodeError, 32, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if err := vaildLoadImageRequest(req); err != nil {
		ec := errCodeV1(_Image, invaildParamsError, 33, "Body parameters are invaild", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil ||
		gd.PluginClient() == nil {

		httpJSONNilGarden(w)
		return
	}

	if timeout > 0 {
		var cancel goctx.CancelFunc
		ctx, cancel = goctx.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}

	pc := gd.PluginClient()
	supports, err := pc.GetImageSupport(ctx)
	if err != nil {
		ec := errCodeV1(_Image, internalError, 34, "fail to get supported image list", "获取已支持的镜像列表错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	found := false
	for _, version := range supports {
		if version.Name == req.Name &&
			version.Major == req.Major &&
			version.Minor == req.Minor &&
			version.Build == req.Build {

			found = true
			break
		}
	}

	if !found {
		ec := errCodeV1(_Image, objectNotExist, 35, "unsupported image:"+req.Version(), "不支持镜像:"+req.Version())
		httpJSONError(w, stderr.New(ec.comment), ec, http.StatusInternalServerError)
		return
	}

	// new context with deadline
	if deadline, ok := ctx.Deadline(); !ok {
		ctx = goctx.Background()
	} else {
		ctx, _ = goctx.WithDeadline(goctx.Background(), deadline)
	}

	// database.Image.ID
	id, taskID, err := resource.LoadImage(ctx, gd.Ormer(), req, timeout)
	if err != nil {
		ec := errCodeV1(_Image, internalError, 36, "fail to load image", "镜像入库失败")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%q,%q:%q}", "id", id, "task_id", taskID)
}

func deleteImage(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	img := mux.Vars(r)["image"]

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil || gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	err := gd.Ormer().DelImage(img)
	if err != nil {
		ec := errCodeV1(_Image, dbTxError, 41, "fail to exec records in into database in a Tx", "数据库事务处理错误（镜像表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func getImage(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	im, err := gd.Ormer().GetImageVersion(name)
	if err != nil {
		if errors.Cause(err) == sql.ErrNoRows {
			writeJSONNull(w, http.StatusOK)
			return
		}

		ec := errCodeV1(_Image, dbQueryError, 51, "fail to query database", "数据库查询错误（镜像表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	imResp := structs.ImageResponse{
		ImageVersion: structs.ImageVersion{
			Name:  im.Name,
			Major: im.Major,
			Minor: im.Minor,
			Patch: im.Patch,
			Build: im.Build,
		},
		Size:     im.Size,
		ID:       im.ID,
		ImageID:  im.ImageID,
		Labels:   im.Labels,
		UploadAt: utils.TimeToString(im.UploadAt),
	}

	t, err := gd.PluginClient().GetImage(ctx, im.Version())
	if err != nil {
		ec := errCodeV1(_Image, internalError, 52, "fail to query image", "获取镜像错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	resp := structs.GetImageResponse{
		ImageResponse: imResp,
		Template:      t,
	}

	writeJSON(w, resp, http.StatusOK)
}

func putImageTemplate(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	ct := structs.ConfigTemplate{}

	err := json.NewDecoder(r.Body).Decode(&ct)
	if err != nil {
		ec := errCodeV1(_Cluster, decodeError, 61, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	err = gd.PluginClient().PostImageTemplate(ctx, ct)
	if err != nil {
		ec := errCodeV1(_Image, internalError, 62, "fail to update image template", "更新镜像模板错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// -----------------/clusters handlers-----------------
func getClustersByID(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil || gd.Ormer() == nil || gd.Cluster == nil {

		httpJSONNilGarden(w)
		return
	}

	orm := gd.Ormer()
	c, err := orm.GetCluster(name)
	if err != nil {
		if errors.Cause(err) == sql.ErrNoRows {
			writeJSONNull(w, http.StatusOK)
			return
		}

		ec := errCodeV1(_Cluster, dbQueryError, 11, "fail to query database", "数据库查询错误（集群表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	n, err := orm.CountNodeByCluster(c.ID)
	if err != nil {
		ec := errCodeV1(_Cluster, dbQueryError, 12, "fail to query database", "数据库查询错误（主机表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	resp := structs.GetClusterResponse{
		ID:         c.ID,
		MaxNode:    c.MaxNode,
		UsageLimit: c.UsageLimit,
		NodeNum:    n,
	}

	writeJSON(w, resp, http.StatusOK)
}

func getClusters(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil || gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	orm := gd.Ormer()

	list, err := orm.ListClusters()
	if err != nil {
		ec := errCodeV1(_Cluster, dbQueryError, 21, "fail to query database", "数据库查询错误（集群表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	out := make([]structs.GetClusterResponse, len(list))
	for i := range list {
		n, err := orm.CountNodeByCluster(list[i].ID)
		if err != nil {
			ec := errCodeV1(_Cluster, dbQueryError, 22, "fail to query database", "数据库查询错误（主机表）")
			httpJSONError(w, err, ec, http.StatusInternalServerError)
			return
		}

		out[i] = structs.GetClusterResponse{
			ID:         list[i].ID,
			MaxNode:    list[i].MaxNode,
			UsageLimit: list[i].UsageLimit,
			NodeNum:    n,
		}
	}

	writeJSON(w, out, http.StatusOK)
}

func vaildPostClusterRequest(v structs.PostClusterRequest) error {
	return nil
}

func postCluster(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.PostClusterRequest{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		ec := errCodeV1(_Cluster, decodeError, 31, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if err := vaildPostClusterRequest(req); err != nil {
		ec := errCodeV1(_Cluster, invaildParamsError, 32, "Body parameters are invaild", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	c := database.Cluster{
		ID:         utils.Generate32UUID(),
		MaxNode:    req.Max,
		UsageLimit: req.UsageLimit,
	}

	err = gd.Ormer().InsertCluster(c)
	if err != nil {
		ec := errCodeV1(_Cluster, dbExecError, 33, "fail to insert records into database", "数据库新增记录错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%q}", "id", c.ID)
}

func putClusterParams(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	req := structs.PostClusterRequest{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		ec := errCodeV1(_Cluster, decodeError, 41, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if err := vaildPostClusterRequest(req); err != nil {
		ec := errCodeV1(_Cluster, invaildParamsError, 42, "Body parameters are invaild", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	c := database.Cluster{
		ID:         name,
		MaxNode:    req.Max,
		UsageLimit: req.UsageLimit,
	}

	err = gd.Ormer().SetClusterParams(c)
	if err != nil {
		ec := errCodeV1(_Cluster, dbExecError, 43, "fail to update records into database", "数据库更新记录错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func deleteCluster(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil || gd.Cluster == nil {

		httpJSONNilGarden(w)
		return
	}

	master := resource.NewHostManager(gd.Ormer(), gd.Cluster, nil)
	err := master.RemoveCluster(name)
	if err != nil {
		ec := errCodeV1(_Cluster, dbExecError, 51, "fail to delete records into database", "数据库删除记录错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// -----------------/hosts handlers-----------------
func getNodeInfo(gd *garden.Garden, n database.Node, e *cluster.Engine) structs.NodeInfo {
	info := structs.NodeInfo{
		ID:           n.ID,
		Cluster:      n.ClusterID,
		Room:         n.Room,
		Seat:         n.Seat,
		MaxContainer: n.MaxContainer,
		Enabled:      n.Enabled,
		RegisterAt:   utils.TimeToString(n.RegisterAt),
	}

	info.SetByEngine(e)

	if info.Engine.IP == "" {
		info.Engine.IP = n.Addr
	}

	if e == nil {
		return info
	}

	drivers, err := driver.FindEngineVolumeDrivers(gd.Ormer(), e)
	if err != nil && len(drivers) == 0 {
		logrus.WithField("Node", n.Addr).Errorf("find Node VolumeDrivers error,%+v", err)
	} else {
		vds := make([]structs.VolumeDriver, 0, len(drivers))

		for _, d := range drivers {
			if d == nil || d.Type() == "NFS" {
				continue
			}

			space, err := d.Space()
			if err != nil {
				logrus.WithField("Node", n.Addr).Errorf("get %s space,%+v", d.Name(), err)
				continue
			}

			vds = append(vds, structs.VolumeDriver{
				Total:  space.Total,
				Free:   space.Free,
				Name:   d.Name(),
				Driver: d.Driver(),
				Type:   d.Type(),
				VG:     space.VG,
			})
		}

		info.VolumeDrivers = vds
	}

	return info
}

func getNode(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil || gd.Cluster == nil {

		httpJSONNilGarden(w)
		return
	}

	n, err := gd.Ormer().GetNode(name)
	if err != nil {
		if errors.Cause(err) == sql.ErrNoRows {
			writeJSONNull(w, http.StatusOK)
			return
		}

		ec := errCodeV1(_Host, dbQueryError, 11, "fail to query database", "数据库查询错误（主机表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	e := gd.Cluster.Engine(n.EngineID)

	info := getNodeInfo(gd, n, e)

	writeJSON(w, info, http.StatusOK)
}

func getAllNodes(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		ec := errCodeV1(_Host, urlParamError, 21, "parse Request URL parameter error", "解析请求URL参数错误")

		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	name := r.FormValue("cluster")

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil || gd.Cluster == nil {

		httpJSONNilGarden(w)
		return
	}

	var nodes []database.Node
	engines := gd.Cluster.ListEngines()

	if name == "" {
		nodes, err = gd.Ormer().ListNodes()
	} else {
		nodes, err = gd.Ormer().ListNodesByCluster(name)
	}
	if err != nil {
		ec := errCodeV1(_Host, dbQueryError, 22, "fail to query database", "数据库查询错误（主机表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	out := make([]structs.NodeInfo, 0, len(nodes))

	for i := range nodes {
		var engine *cluster.Engine

		for _, e := range engines {
			if e.IP == nodes[i].Addr {
				engine = e
				break
			}
		}

		out = append(out, getNodeInfo(gd, nodes[i], engine))
	}

	writeJSON(w, out, http.StatusOK)
}

func vaildNodeRequest(node structs.Node) error {
	errs := make([]string, 0, 3)

	if node.Cluster == "" {
		errs = append(errs, "Cluster is required")
	}

	if node.Addr == "" {
		errs = append(errs, "Addr is required")
	}

	// vaild ssh config
	if node.SSHConfig.Username == "" {
		errs = append(errs, "SSHConfig.Username is required")
	}

	if len(errs) == 0 {
		return nil
	}

	return fmt.Errorf("PostNodeRequest:%v,%s", node, errs)
}

func postNode(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	n := structs.Node{}
	err := json.NewDecoder(r.Body).Decode(&n)
	if err != nil {
		ec := errCodeV1(_Host, decodeError, 31, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if err := vaildNodeRequest(n); err != nil {
		ec := errCodeV1(_Host, invaildParamsError, 32, "Body parameters are invaild", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil ||
		gd.KVClient() == nil {
		httpJSONNilGarden(w)
		return
	}

	orm := gd.Ormer()

	_, err = orm.GetCluster(n.Cluster)
	if err != nil {
		ec := errCodeV1(_Host, dbQueryError, 33, "fail to query database", "数据库查询错误（集群表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	if n.Storage != "" {
		_, err = orm.GetStorageByID(n.Storage)
		if err != nil {
			ec := errCodeV1(_Host, dbQueryError, 34, "fail to query database", "数据库查询错误（外部存储表）")
			httpJSONError(w, err, ec, http.StatusInternalServerError)
			return
		}
	}

	nodes := resource.NewNodeWithTaskList(1)

	node := database.Node{
		ID:           utils.Generate32UUID(),
		ClusterID:    n.Cluster,
		Addr:         n.Addr,
		EngineID:     "",
		Room:         n.Room,
		Seat:         n.Seat,
		MaxContainer: n.MaxContainer,
		Status:       0,
		Enabled:      false,
		NFS: database.NFS{
			Addr:     n.NFS.Address,
			Dir:      n.NFS.Dir,
			MountDir: n.NFS.MountDir,
			Options:  n.NFS.Options,
		},
	}

	nodes[0] = resource.NewNodeWithTask(node, n.HDD, n.SSD, n.SSHConfig)

	horus, err := gd.KVClient().GetHorusAddr(ctx)
	if err != nil {
		ec := errCodeV1(_Host, internalError, 35, "fail to query third-part monitor server addr", "获取第三方监控服务地址错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	// new Context with deadline
	if deadline, ok := ctx.Deadline(); !ok {
		ctx = goctx.Background()
	} else {
		ctx, _ = goctx.WithDeadline(goctx.Background(), deadline)
	}

	master := resource.NewHostManager(orm, gd.Cluster, nodes)
	err = master.InstallNodes(ctx, horus, gd.KVClient())
	if err != nil {
		ec := errCodeV1(_Host, internalError, 36, "fail to install host", "主机入库错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	out := structs.PostNodeResponse{
		ID:   nodes[0].Node.ID,
		Addr: nodes[0].Node.Addr,
		Task: nodes[0].Task.ID,
	}

	writeJSON(w, out, http.StatusCreated)
}

func putNodeEnable(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	err := gd.Ormer().SetNodeEnable(name, true)
	if err != nil {
		ec := errCodeV1(_Host, dbExecError, 41, "fail to update records into database", "数据库更新记录错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func putNodeDisable(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	err := gd.Ormer().SetNodeEnable(name, false)
	if err != nil {
		ec := errCodeV1(_Host, dbExecError, 51, "fail to update records into database", "数据库更新记录错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func putNodeParam(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	var max = struct {
		N int `json:"max_container"`
	}{}

	err := json.NewDecoder(r.Body).Decode(&max)
	if err != nil {
		ec := errCodeV1(_Host, decodeError, 61, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	err = gd.Ormer().SetNodeParam(name, max.N)
	if err != nil {
		ec := errCodeV1(_Host, dbExecError, 62, "fail to update records into database", "数据库更新记录错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func vaildDelNodesRequest(name, user string) error {
	errs := make([]string, 0, 2)

	if name == "" {
		errs = append(errs, "host nameOrID is required")
	}

	if user == "" {
		errs = append(errs, "ssh.Username is required")
	}

	if len(errs) == 0 {
		return nil
	}

	return fmt.Errorf("%s", errs)
}

// DELETE /clusters/nodes/{node:.*}
//
// 204 删除成功
// 400 request header 读取失败
// 412 因未满足条件（主机还有未删除的容器）取消出库操作
// 500 数据库读写错误
// 503 向 Horus 注销主机失败
// 510 SSH 出库脚本执行失败
func deleteNode(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ec := errCodeV1(_Host, urlParamError, 71, "parse Request URL parameter error", "解析请求URL参数错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	node := mux.Vars(r)["node"]
	force := boolValue(r, "force")
	t := intValueOrZero(r, "timeout")
	timeout := time.Duration(t) * time.Second

	username := r.FormValue("username")
	password := r.FormValue("password")

	if err := vaildDelNodesRequest(node, username); err != nil {
		ec := errCodeV1(_Host, invaildParamsError, 72, "URL parameters are invaild", "URL参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil ||
		gd.KVClient() == nil {

		httpJSONNilGarden(w)
		return
	}

	horus, err := gd.KVClient().GetHorusAddr(ctx)
	if err != nil {
		ec := errCodeV1(_Host, internalError, 73, "fail to query third-part monitor server addr", "获取第三方监控服务地址错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	m := resource.NewHostManager(gd.Ormer(), gd.Cluster, nil)

	err = m.RemoveNode(ctx, horus, node, username, password, force, timeout, gd.KVClient())
	if err != nil {
		ec := errCodeV1(_Host, internalError, 74, "fail to uninstall host agents", "主机出库错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// -----------------/networkings handlers-----------------
func vailPostNetworkingRequest(v structs.PostNetworkingRequest) error {
	errs := make([]string, 0, 5)

	if v.Prefix < 0 || v.Prefix > 32 {
		errs = append(errs, fmt.Sprintf("illegal Prefix:%d not in 1~32", v.Prefix))
	}

	start := net.ParseIP(v.Start)
	if start == nil {
		errs = append(errs, fmt.Sprintf("illegal IP:'%s' error", v.Start))
	}

	end := net.ParseIP(v.End)
	if end == nil {
		errs = append(errs, fmt.Sprintf("illegal IP:'%s' error", v.End))
	}

	if mask := net.CIDRMask(v.Prefix, 32); !start.Mask(mask).Equal(end.Mask(mask)) {
		errs = append(errs, fmt.Sprintf("%s-%s is different network segments", start, end))
	}

	if ip := net.ParseIP(v.Gateway); ip == nil {
		errs = append(errs, fmt.Sprintf("illegal Gateway:'%s' error", v.Gateway))
	}

	if len(errs) == 0 {
		return nil
	}

	return fmt.Errorf("PostNetworkingRequest:%v,%s", v, errs)
}

func postNetworking(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	var req structs.PostNetworkingRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		ec := errCodeV1(_Networking, decodeError, 11, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if err := vailPostNetworkingRequest(req); err != nil {
		ec := errCodeV1(_Networking, invaildParamsError, 12, "Body parameters are invaild", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if name == "" && req.Networking != "" {
		name = req.Networking
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	nw := resource.NewNetworks(gd.Ormer())
	n, err := nw.AddNetworking(req.Start, req.End, req.Gateway, name, req.VLAN, req.Prefix)
	if err != nil {
		ec := errCodeV1(_Networking, dbExecError, 13, "fail to insert records into database", "数据库新增记录错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%d}", "num", n)
}

func setNetworking(ctx goctx.Context, w http.ResponseWriter, r *http.Request, enable bool) {
	if err := r.ParseForm(); err != nil {
		ec := errCodeV1(_Networking, urlParamError, 21, "parse Request URL parameter error", "解析请求URL参数错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}
	name := mux.Vars(r)["name"]
	all := boolValue(r, "all")

	var (
		err  error
		body []string
	)

	if !all {
		err := json.NewDecoder(r.Body).Decode(&body)
		if err != nil {
			ec := errCodeV1(_Networking, decodeError, 22, "JSON Decode Request Body error", "JSON解析请求Body错误")
			httpJSONError(w, err, ec, http.StatusBadRequest)
			return
		}
	}
	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	orm := gd.Ormer()
	filters := make([]uint32, 0, len(body))

	if len(body) > 0 {
		list, err := orm.ListIPByNetworking(name)
		if err != nil {
			ec := errCodeV1(_Networking, dbQueryError, 23, "fail to query database", "数据库查询错误（网络IP表）")
			httpJSONError(w, err, ec, http.StatusInternalServerError)
			return
		}

		for i := range body {
			n := utils.IPToUint32(body[i])
			if n > 0 {
				filters = append(filters, n)
			}

			exist := false
			for i := range list {
				if list[i].IPAddr == n {
					exist = true
					break
				}
			}
			if !exist {
				ec := errCodeV1(_Networking, invaildParamsError, 24, fmt.Sprintf("IP %s is not in networking %s", body[i], name), fmt.Sprintf("IP %s 不属于指定网络集群(%s)", body[i], name))
				httpJSONError(w, stderr.New(ec.comment), ec, http.StatusInternalServerError)
				return
			}
		}
	}

	if len(filters) == 0 {
		err = orm.SetNetworkingEnable(name, enable)
	} else {
		err = orm.SetIPEnable(filters, name, enable)
	}
	if err != nil {
		ec := errCodeV1(_Networking, dbTxError, 25, "fail to exec records in into database in a Tx", "数据库事务处理错误（网络IP表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func putNetworkingEnable(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	setNetworking(ctx, w, r, true)
}

func putNetworkingDisable(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	setNetworking(ctx, w, r, false)
}

func deleteNetworking(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil || gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	err := gd.Ormer().DelNetworking(name)
	if err != nil {
		ec := errCodeV1(_Networking, dbExecError, 41, "fail to delete records into database", "数据库删除记录错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func convertNetworking(list []database.IP) []structs.NetworkingInfo {
	if len(list) == 0 {
		return []structs.NetworkingInfo{}
	}

	nws := make([]structs.NetworkingInfo, 0, 5)

loop:
	for i := range list {
		for n := range nws {
			if nws[n].Networking == list[i].Networking {

				nws[n].IPs = append(nws[n].IPs, structs.IP{
					Enabled:   list[i].Enabled,
					IPAddr:    utils.Uint32ToIP(list[i].IPAddr).String(),
					UnitID:    list[i].UnitID,
					Engine:    list[i].Engine,
					Bond:      list[i].Bond,
					Bandwidth: list[i].Bandwidth,
				})

				continue loop
			}
		}

		nw := structs.NetworkingInfo{
			Prefix:     list[i].Prefix,
			VLAN:       list[i].VLAN,
			Networking: list[i].Networking,
			Gateway:    list[i].Gateway,
			IPs:        make([]structs.IP, 1, 30),
		}

		nw.IPs[0] = structs.IP{
			Enabled:   list[i].Enabled,
			IPAddr:    utils.Uint32ToIP(list[i].IPAddr).String(),
			UnitID:    list[i].UnitID,
			Engine:    list[i].Engine,
			Bond:      list[i].Bond,
			Bandwidth: list[i].Bandwidth,
		}

		nws = append(nws, nw)
	}

	return nws
}

func listNetworkings(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	out, err := gd.Ormer().ListIPs()
	if err != nil {
		ec := errCodeV1(_Networking, dbQueryError, 51, "fail to query database", "数据库查询错误（网络IP表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	nws := convertNetworking(out)

	writeJSON(w, nws, http.StatusOK)
}

func getNetworking(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	out, err := gd.Ormer().ListIPByNetworking(name)
	if err != nil {
		ec := errCodeV1(_Networking, dbQueryError, 61, "fail to query database", "数据库查询错误（网络IP表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	nws := convertNetworking(out)
	if len(nws) == 1 {
		writeJSON(w, nws[0], http.StatusOK)
		return
	}

	for i := range nws {
		if nws[i].Networking == name {
			writeJSON(w, nws[i], http.StatusOK)
			return
		}
	}

	writeJSON(w, "{}", http.StatusOK)
}

// -----------------/services handlers-----------------
func getServices(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil || gd.Ormer() == nil || gd.Cluster == nil {
		httpJSONNilGarden(w)
		return
	}

	services, err := gd.ListServices(ctx)
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 11, "fail to query database", "数据库查询错误（服务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSON(w, services, http.StatusOK)
}

func getServicesByNameOrID(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil || gd.Ormer() == nil || gd.Cluster == nil {
		httpJSONNilGarden(w)
		return
	}

	spec, err := gd.ServiceSpec(name)
	if err != nil {
		if errors.Cause(err) == sql.ErrNoRows {
			writeJSONNull(w, http.StatusOK)
			return
		}

		ec := errCodeV1(_Service, dbQueryError, 21, "fail to query database", "数据库查询错误（服务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSON(w, spec, http.StatusOK)
}

func vaildPostServiceRequest(spec structs.ServiceSpec) error {
	errs := make([]string, 0, 4)

	if spec.Arch.Code == "" || spec.Arch.Mode == "" || spec.Arch.Replicas == 0 {
		errs = append(errs, fmt.Sprintf("Arch invaild,%+v", spec.Arch))
	}

	if spec.Require == nil || spec.Require.Require.CPU == 0 || spec.Require.Require.Memory == 0 {
		errs = append(errs, "unit require is nil")
	}

	if len(spec.Require.Networks) > 0 && len(spec.Networkings) == 0 {
		errs = append(errs, "networkings is required")
	}

	if len(spec.Clusters) == 0 {
		errs = append(errs, "clusters is required")
	}

	if len(errs) == 0 {
		return nil
	}

	return fmt.Errorf("ServiceSpec:%v,%s", spec, errs)
}

func postService(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ec := errCodeV1(_Service, urlParamError, 31, "parse Request URL parameter error", "解析请求URL参数错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	timeout := intValueOrZero(r, "timeout")

	spec := structs.ServiceSpec{}
	err := json.NewDecoder(r.Body).Decode(&spec)
	if err != nil {
		ec := errCodeV1(_Service, decodeError, 32, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if err := vaildPostServiceRequest(spec); err != nil {
		ec := errCodeV1(_Service, invaildParamsError, 33, "Body parameters are invaild", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil ||
		gd.KVClient() == nil ||
		gd.PluginClient() == nil {

		httpJSONNilGarden(w)
		return
	}

	// new Context with deadline
	if timeout > 0 {
		ctx, _ = goctx.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	}
	if deadline, ok := ctx.Deadline(); !ok {
		ctx = goctx.Background()
	} else {
		ctx, _ = goctx.WithDeadline(goctx.Background(), deadline)
	}

	d := deploy.New(gd)

	out, err := d.Deploy(ctx, spec)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 34, "fail to deploy service", "创建服务错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSON(w, out, http.StatusCreated)
}

func vaildPostServiceScaledRequest(v structs.ServiceScaleRequest) error {

	if v.Arch.Code == "" || v.Arch.Mode == "" || v.Arch.Replicas == 0 {
		return fmt.Errorf("Arch invaild,%+v", v.Arch)
	}

	return nil
}

func postServiceScaled(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	scale := structs.ServiceScaleRequest{}
	err := json.NewDecoder(r.Body).Decode(&scale)
	if err != nil {
		ec := errCodeV1(_Service, decodeError, 41, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if err := vaildPostServiceScaledRequest(scale); err != nil {
		ec := errCodeV1(_Service, invaildParamsError, 42, "Body parameters are invaild", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil ||
		gd.KVClient() == nil ||
		gd.PluginClient() == nil {

		httpJSONNilGarden(w)
		return
	}

	// new Context with deadline
	if deadline, ok := ctx.Deadline(); !ok {
		ctx = goctx.Background()
	} else {
		ctx, _ = goctx.WithDeadline(goctx.Background(), deadline)
	}

	d := deploy.New(gd)

	id, err := d.ServiceScale(ctx, name, scale)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 43, "fail to scale service", "服务水平扩展错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%q}", "task_id", id)
}

func vaildPostServiceLinkRequest(v structs.ServicesLink) error {
	if v.Len() == 0 {
		return fmt.Errorf("invaild params")
	}

	errs := make([]string, 0, 3)

	for i := range v.Links {
		if v.Links[i] != nil && v.Links[i].ID == "" {
			errs = append(errs, "ServiceLink.ID is required")
		}
	}

	if len(errs) == 0 {
		return nil
	}

	return fmt.Errorf("ServicesLink:%v,%s", v, errs)
}

func postServiceLink(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	links := structs.ServicesLink{}
	err := json.NewDecoder(r.Body).Decode(&links)
	if err != nil {
		ec := errCodeV1(_Service, decodeError, 51, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if err := vaildPostServiceLinkRequest(links); err != nil {
		ec := errCodeV1(_Service, invaildParamsError, 52, "Body parameters are invaild", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil ||
		gd.KVClient() == nil ||
		gd.PluginClient() == nil {

		httpJSONNilGarden(w)
		return
	}

	// new Context with deadline
	if deadline, ok := ctx.Deadline(); !ok {
		ctx = goctx.Background()
	} else {
		ctx, _ = goctx.WithDeadline(goctx.Background(), deadline)
	}

	d := deploy.New(gd)

	// task ID
	id, err := d.Link(ctx, links)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 53, "fail to link services", "关联服务错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%q}", "task_id", id)
}

func postServiceVersionUpdate(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ec := errCodeV1(_Service, urlParamError, 61, "parse Request URL parameter error", "解析请求URL参数错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	name := mux.Vars(r)["name"]
	version := r.FormValue("image")

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil || gd.PluginClient() == nil {

		httpJSONNilGarden(w)
		return
	}

	// new Context with deadline
	if deadline, ok := ctx.Deadline(); !ok {
		ctx = goctx.Background()
	} else {
		ctx, _ = goctx.WithDeadline(goctx.Background(), deadline)
	}

	d := deploy.New(gd)

	id, err := d.ServiceUpdateImage(ctx, name, version, true)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 62, "fail to update service units image version", "服务容器镜像版本升级错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%q}", "task_id", id)
}

func vaildPostServiceUpdateRequest(v structs.UnitRequire) error {
	if v.Require.CPU == 0 && v.Require.Memory == 0 && len(v.Volumes) == 0 {
		return fmt.Errorf("UnitRequire:%v,%s", v, "no resource update required")
	}

	errs := make([]string, 0, 3)

	for _, vr := range v.Volumes {
		if vr.Name == "" && vr.Type == "" {
			errs = append(errs, fmt.Sprintf("VolumeRequire is invaild,%+v", vr))
		}
	}

	if len(errs) == 0 {
		return nil
	}

	return fmt.Errorf("ServiceUpdateRequest:%s", errs)
}

func postServiceUpdate(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	var update structs.UnitRequire

	err := json.NewDecoder(r.Body).Decode(&update)
	if err != nil {
		ec := errCodeV1(_Service, decodeError, 71, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if err := vaildPostServiceUpdateRequest(update); err != nil {
		ec := errCodeV1(_Service, invaildParamsError, 72, "Body parameters are invaild", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil || gd.Cluster == nil {

		httpJSONNilGarden(w)
		return
	}

	// new Context with deadline
	if deadline, ok := ctx.Deadline(); !ok {
		ctx = goctx.Background()
	} else {
		ctx, _ = goctx.WithDeadline(goctx.Background(), deadline)
	}

	d := deploy.New(gd)

	id, err := d.ServiceUpdate(ctx, name, update)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 73, "fail to update service containers", "服务垂直扩容错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%q}", "task_id", id)
}

func postServiceStart(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil || gd.KVClient() == nil {

		httpJSONNilGarden(w)
		return
	}

	svc, err := gd.GetService(name)
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 81, "fail to query database", "数据库查询错误（服务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	spec, err := svc.Spec()
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 82, "fail to query database", "数据库查询错误（服务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	// new Context with deadline
	if deadline, ok := ctx.Deadline(); !ok {
		ctx = goctx.Background()
	} else {
		ctx, _ = goctx.WithDeadline(goctx.Background(), deadline)
	}

	task := database.NewTask(spec.Name, database.ServiceStartTask, spec.ID, spec.Desc, nil, 300)

	err = svc.InitStart(ctx, gd.KVClient(), nil, &task, true, nil)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 83, "fail to init start service", "服务初始化启动错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%q}", "task_id", task.ID)
}

func vaildPostServiceUpdateConfigsRequest(req structs.ServiceConfigs) error {
	if len(req) == 0 {
		return stderr.New("nothing new for update for service configs")
	}

	return nil
}

func postServiceUpdateConfigs(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	var req structs.ServiceConfigs

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		ec := errCodeV1(_Service, decodeError, 91, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if err := vaildPostServiceUpdateConfigsRequest(req); err != nil {
		ec := errCodeV1(_Service, invaildParamsError, 92, "Body parameters are invaild", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	svc, err := gd.GetService(name)
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 93, "fail to query database", "数据库查询错误（服务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	spec, err := svc.Spec()
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 94, "fail to query database", "数据库查询错误（服务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	// new Context with deadline
	if deadline, ok := ctx.Deadline(); !ok {
		ctx = goctx.Background()
	} else {
		ctx, _ = goctx.WithDeadline(goctx.Background(), deadline)
	}

	task := database.NewTask(spec.Name, database.ServiceUpdateConfigTask, spec.ID, spec.Desc, nil, 300)

	err = svc.UpdateUnitsConfigs(ctx, req, &task, true)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 95, "fail to update service config files", "服务配置文件更新错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%q}", "task_id", task.ID)
}

func vaildPostServiceExecRequest(v structs.ServiceExecConfig) error {
	if len(v.Cmd) == 0 {
		return stderr.New("ServiceExecConfig.Cmd is required")
	}

	return nil
}

func postServiceExec(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	config := structs.ServiceExecConfig{}
	err := json.NewDecoder(r.Body).Decode(&config)
	if err != nil {
		ec := errCodeV1(_Service, decodeError, 101, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if err := vaildPostServiceExecRequest(config); err != nil {
		ec := errCodeV1(_Service, invaildParamsError, 102, "Body parameters are invaild", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil || gd.Cluster == nil {

		httpJSONNilGarden(w)
		return
	}

	svc, err := gd.GetService(name)
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 103, "fail to query database", "数据库查询错误（服务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	spec, err := svc.Spec()
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 104, "fail to query database", "数据库查询错误（服务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	// new Context with deadline
	if deadline, ok := ctx.Deadline(); !ok {
		ctx = goctx.Background()
	} else {
		ctx, _ = goctx.WithDeadline(goctx.Background(), deadline)
	}

	task := database.NewTask(spec.Name, database.ServiceExecTask, spec.ID, spec.Desc, nil, 300)

	err = svc.Exec(ctx, config, true, &task)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 105, "fail to exec command in service containers", "服务容器远程命令执行错误（container exec）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%q}", "task_id", task.ID)
}

func postServiceStop(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	svc, err := gd.GetService(name)
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 111, "fail to query database", "数据库查询错误（服务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	spec, err := svc.Spec()
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 112, "fail to query database", "数据库查询错误（服务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	// new Context with deadline
	if deadline, ok := ctx.Deadline(); !ok {
		ctx = goctx.Background()
	} else {
		ctx, _ = goctx.WithDeadline(goctx.Background(), deadline)
	}

	task := database.NewTask(spec.Name, database.ServiceStopTask, spec.ID, spec.Desc, nil, 300)

	err = svc.Stop(ctx, true, true, &task)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 113, "fail to stop service", "服务关闭错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%q}", "task_id", task.ID)
}

func deleteService(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ec := errCodeV1(_Service, urlParamError, 121, "parse Request URL parameter error", "解析请求URL参数错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	name := mux.Vars(r)["name"]
	force := boolValue(r, "force")

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil || gd.KVClient() == nil {

		httpJSONNilGarden(w)
		return
	}

	svc, err := gd.GetService(name)
	if err != nil {
		if errors.Cause(err) == sql.ErrNoRows {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		ec := errCodeV1(_Service, dbQueryError, 122, "fail to query database", "数据库查询错误（服务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	err = svc.Remove(ctx, gd.KVClient(), force)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 123, "fail to remove service", "删除服务错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func vaildPostServiceBackupRequest(v structs.ServiceBackupConfig) error {
	return nil
	//	errs := make([]string, 0, 4)

	//	if v.BackupDir == "" {
	//		errs = append(errs, "BackupDir is required")
	//	}

	//	if v.Type == "" {
	//		errs = append(errs, "Type is required")
	//	}

	//	if v.MaxSizeByte <= 0 {
	//		errs = append(errs, "MaxSizeByte is required")
	//	}

	//	if v.FilesRetention == 0 {
	//		errs = append(errs, "FilesRetention is required")
	//	}

	//	if len(errs) == 0 {
	//		return nil
	//	}

	//	return fmt.Errorf("ServiceBackupConfig:%v,%s", v, errs)
}

func postServiceBackup(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	config := structs.ServiceBackupConfig{}
	err := json.NewDecoder(r.Body).Decode(&config)
	if err != nil {
		ec := errCodeV1(_Service, decodeError, 131, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if err := vaildPostServiceBackupRequest(config); err != nil {
		ec := errCodeV1(_Service, invaildParamsError, 132, "Body parameters are invaild", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if config.BackupDir == "" {
		config.BackupDir = "/backup"
	}
	if config.Type == "" {
		config.Type = "full"
	}
	if config.FilesRetention == 0 {
		config.FilesRetention = 7
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil || gd.Cluster == nil {

		httpJSONNilGarden(w)
		return
	}

	svc, err := gd.GetService(name)
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 133, "fail to query database", "数据库查询错误（服务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	spec, err := svc.Spec()
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 134, "fail to query database", "数据库查询错误（服务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	// new Context with deadline
	if deadline, ok := ctx.Deadline(); !ok {
		ctx = goctx.Background()
	} else {
		ctx, _ = goctx.WithDeadline(goctx.Background(), deadline)
	}

	task := database.NewTask(spec.Name, database.ServiceBackupTask, spec.ID, spec.Desc, nil, 300)

	err = svc.Backup(ctx, r.Host, config, true, &task)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 135, "fail to back service", "服务备份错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%q}", "task_id", task.ID)
}

func vaildPostServiceRestoreRequest(v structs.ServiceRestoreRequest) error {
	if v.File != "" {
		return nil
	}

	return fmt.Errorf("ServiceRestoreRequest:%v,restore file is required", v)
}

func postServiceRestore(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	req := structs.ServiceRestoreRequest{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		ec := errCodeV1(_Service, decodeError, 141, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if err := vaildPostServiceRestoreRequest(req); err != nil {
		ec := errCodeV1(_Service, invaildParamsError, 142, "Body parameters are invaild", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	orm := gd.Ormer()

	bf, err := orm.GetBackupFile(req.File)
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 143, "fail to query database", "数据库查询错误（备份文件表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	// new Context with deadline
	if deadline, ok := ctx.Deadline(); !ok {
		ctx = goctx.Background()
	} else {
		ctx, _ = goctx.WithDeadline(goctx.Background(), deadline)
	}

	svc, err := gd.GetService(name)
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 144, "fail to query database", "数据库查询错误（服务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	id, err := svc.UnitRestore(ctx, req.Units, bf.Path, true)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 145, "fail to restore unit data", "服务单元数据恢复错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%q}", "task_id", id)
}

func vaildPostUnitRebuildRequest(v structs.UnitRebuildRequest) error {
	if len(v.Units) < 1 {
		return stderr.New("Units is required")
	}

	return nil
}

func postUnitRebuild(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	req := structs.UnitRebuildRequest{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		ec := errCodeV1(_Service, decodeError, 151, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if err := vaildPostUnitRebuildRequest(req); err != nil {
		ec := errCodeV1(_Service, invaildParamsError, 152, "Body parameters are invaild", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil ||
		gd.KVClient() == nil ||
		gd.PluginClient() == nil {

		httpJSONNilGarden(w)
		return
	}

	svc, err := gd.Service(name)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 153, "not found the service", "查询指定服务错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	// new Context with deadline
	if deadline, ok := ctx.Deadline(); !ok {
		ctx = goctx.Background()
	} else {
		ctx, _ = goctx.WithDeadline(goctx.Background(), deadline)
	}

	id, err := gd.RebuildUnits(ctx, nil, svc, req, true)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 154, "fail to scale service", "服务水平扩展错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%q}", "task_id", id)
}

func vaildPostUnitMigrateRequest(v structs.PostUnitMigrate) error {
	if v.NameOrID == "" {
		return stderr.New("Unit name or ID is required")
	}

	return nil
}

func postUnitMigrate(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	req := structs.PostUnitMigrate{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		ec := errCodeV1(_Service, decodeError, 161, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if err := vaildPostUnitMigrateRequest(req); err != nil {
		ec := errCodeV1(_Service, invaildParamsError, 162, "Body parameters are invaild", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil ||
		gd.KVClient() == nil ||
		gd.PluginClient() == nil {

		httpJSONNilGarden(w)
		return
	}

	svc, err := gd.Service(name)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 163, "not found the service", "查询指定服务错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	// new Context with deadline
	if deadline, ok := ctx.Deadline(); !ok {
		ctx = goctx.Background()
	} else {
		ctx, _ = goctx.WithDeadline(goctx.Background(), deadline)
	}

	id, err := gd.ServiceMigrate(ctx, svc, req.NameOrID, req.Candidates, true)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 164, "fail to scale service", "服务水平扩展错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%q}", "task_id", id)
}

func getServiceBackupFiles(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	files, err := gd.Ormer().ListBackupFilesByService(name)
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 171, "fail to query database", "数据库查询错误（备份文件表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSON(w, files, http.StatusOK)
}

func getServiceConfigFiles(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	svc, err := gd.GetService(name)
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 181, "fail to query database", "数据库查询错误（服务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	out, err := svc.GetUnitsConfigs(ctx)
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 182, "fail to query units configs from kv", "获取服务单元配置错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSON(w, out, http.StatusOK)
}

// -----------------/units handlers-----------------
func proxySpecialLogic(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ec := errCodeV1(_Unit, urlParamError, 11, "parse Request URL parameter error", "解析请求URL参数错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	name := mux.Vars(r)["name"]
	proxyURL := mux.Vars(r)["proxy"]
	port := r.Header.Get("X-Service-Port")
	orm := gd.Ormer()

	u, err := orm.GetUnit(name)
	if err != nil {
		ec := errCodeV1(_Unit, dbQueryError, 11, "fail to query database", "数据库查询错误（服务单元表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	ips, err := orm.ListIPByUnitID(u.ID)
	if err != nil {
		ec := errCodeV1(_Unit, dbQueryError, 12, "fail to query database", "数据库查询错误（网络IP表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	if len(ips) == 0 {
		ec := errCodeV1(_Unit, objectNotExist, 13, fmt.Sprintf("not found the unit %s server addr", name), "找不到单元的服务地址")
		httpJSONError(w, stderr.New(ec.comment), ec, http.StatusInternalServerError)
		return
	}

	r.URL.Path = "/" + proxyURL
	addr := utils.Uint32ToIP(ips[0].IPAddr).String()
	addr = net.JoinHostPort(addr, port)

	err = hijack(nil, addr, w, r)
	if err != nil {
		ec := errCodeV1(_Unit, internalError, 14, "fail to connect the special container", "连接容器服务错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}
}

// -----------------/storage handlers-----------------
// GET /storage/san
func getSANStoragesInfo(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	ds := storage.DefaultStores()
	stores, err := ds.List()
	if err != nil {
		ec := errCodeV1(_Storage, dbQueryError, 11, "fail to query database", "数据库查询错误（外部存储表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	resp := make([]structs.SANStorageResponse, len(stores))
	for i := range stores {
		resp[i], err = getSanStoreInfo(stores[i])
		if err != nil {
			ec := errCodeV1(_Storage, internalError, 12, "fail to get san storage info", "外部存储查询错误")
			httpJSONError(w, err, ec, http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, resp, http.StatusOK)
}

// GET /storage/san/{name:.*}
func getSANStorageInfo(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ds := storage.DefaultStores()
	store, err := ds.Get(name)
	if err != nil {
		if errors.Cause(err) == sql.ErrNoRows {
			writeJSONNull(w, http.StatusOK)
			return
		}

		ec := errCodeV1(_Storage, dbQueryError, 21, "fail to query database", "数据库查询错误（外部存储表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	resp, err := getSanStoreInfo(store)
	if err != nil {
		ec := errCodeV1(_Storage, internalError, 22, "fail to get san storage info", "外部存储查询错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSON(w, resp, http.StatusOK)
}

func getSanStoreInfo(store storage.Store) (structs.SANStorageResponse, error) {
	info, err := store.Info()
	if err != nil {
		return structs.SANStorageResponse{}, err
	}

	spaces := make([]structs.Space, 0, len(info.List))
	for _, val := range info.List {
		spaces = append(spaces, structs.Space{
			Enable: val.Enable,
			ID:     val.ID,
			Total:  val.Total,
			Free:   val.Free,
			LunNum: val.LunNum,
			State:  val.State,
		})

	}

	return structs.SANStorageResponse{
		ID:     info.ID,
		Vendor: info.Vendor,
		Driver: info.Driver,
		Total:  info.Total,
		Free:   info.Free,
		Used:   info.Total - info.Free,
		Spaces: spaces,
	}, nil
}

func vaildPostSanStorageRequest(v structs.PostSANStoreRequest) error {
	errs := make([]string, 0, 2)

	if v.Vendor == "" {
		errs = append(errs, "Vendor is required")
	}

	if v.HostLunStart < v.HostLunEnd || v.HostLunEnd < 0 {
		errs = append(errs, "host_lun_end or host_lun_start is invaild")

	}

	if len(errs) == 0 {
		return nil
	}

	return fmt.Errorf("PostSANStoreRequest:%v,%s", v, errs)
}

// POST /storage/san
func postSanStorage(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.PostSANStoreRequest{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ec := errCodeV1(_Storage, decodeError, 31, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if err := vaildPostSanStorageRequest(req); err != nil {
		ec := errCodeV1(_Storage, invaildParamsError, 32, "Body parameters are invaild", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ds := storage.DefaultStores()
	if ds == nil {
		ec := errCodeV1(_Storage, internalError, 33, "storage:func DefaultStores called before SetDefaultStores", "内部逻辑错误")
		httpJSONError(w, stderr.New(ec.comment), ec, http.StatusInternalServerError)
		return
	}

	s, err := ds.Add(req.Vendor, req.Version, req.Addr,
		req.Username, req.Password, req.Admin,
		req.LunStart, req.LunEnd, req.HostLunStart, req.HostLunEnd)
	if err != nil {
		ec := errCodeV1(_Storage, internalError, 34, "fail to add new storage", "新增外部存储错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%q}", "id", s.ID())
}

// POST /storage/san/{name}/raidgroup
func postRGToSanStorage(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	san := mux.Vars(r)["name"]
	rg := mux.Vars(r)["rg"]

	ds := storage.DefaultStores()
	store, err := ds.Get(san)
	if err != nil {
		ec := errCodeV1(_Storage, dbQueryError, 41, "fail to query database", "数据库查询错误（外部存储表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	space, err := store.AddSpace(rg)
	if err != nil {
		ec := errCodeV1(_Storage, internalError, 42, "fail to add new raidgroup to the storage", "外部存储新增RG错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%d}", "size", space.Total)
}

// PUT /storage/san/{name}/raid_group/{rg:[0-9]+}/enable
func putEnableRaidGroup(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	san := mux.Vars(r)["name"]
	rg := mux.Vars(r)["rg"]

	ds := storage.DefaultStores()
	store, err := ds.Get(san)
	if err != nil {
		ec := errCodeV1(_Storage, dbQueryError, 51, "fail to query database", "数据库查询错误（外部存储表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	err = store.EnableSpace(rg)
	if err != nil {
		ec := errCodeV1(_Storage, dbExecError, 52, "fail to update records into database", "数据库更新记录错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// PUT /storage/san/{name}/raid_group/{rg:[0-9]+}/disable
func putDisableRaidGroup(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	san := mux.Vars(r)["name"]
	rg := mux.Vars(r)["rg"]

	ds := storage.DefaultStores()
	store, err := ds.Get(san)
	if err != nil {
		ec := errCodeV1(_Storage, dbQueryError, 61, "fail to query database", "数据库查询错误（外部存储表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	err = store.DisableSpace(rg)
	if err != nil {
		ec := errCodeV1(_Storage, dbExecError, 62, "fail to update records into database", "数据库更新记录错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// DELETE /storage/san/{name}
func deleteStorage(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ds := storage.DefaultStores()

	err := ds.Remove(name)
	if err != nil {
		ec := errCodeV1(_Storage, internalError, 71, "fail to remove storage", "删除外部存储系统错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DELETE /storage/san/{name}/raid_group/{rg:[0-9]+}
func deleteRaidGroup(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	san := mux.Vars(r)["name"]
	rg := mux.Vars(r)["rg"]

	ds := storage.DefaultStores()

	err := ds.RemoveStoreSpace(san, rg)
	if err != nil {
		ec := errCodeV1(_Storage, internalError, 81, "fail to remove RG from storage", "删除外部存储系统的RG错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
