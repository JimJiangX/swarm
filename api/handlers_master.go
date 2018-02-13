package api

import (
	"encoding/json"
	stderr "errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

const dateLayout = "2006-01-02"

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
func validNFSParams(nfs database.NFS) error {
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

	err := validNFSParams(nfs)
	if err != nil {
		ec := errCodeV1(_NFS, invalidParamsError, 12, "URL parameters are invalid", "URL参数校验错误，包含无效参数")
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
		if database.IsNotFound(err) {
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

func validBackupTaskCallback(bt structs.BackupTaskCallback) error {
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

	err = validBackupTaskCallback(req)
	if err != nil {
		ec := errCodeV1(_Task, invalidParamsError, 32, "Body parameters are invalid", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil || gd.Ormer() == nil {
		httpJSONNilGarden(w)
		return
	}
	orm := gd.Ormer()
	now := time.Now()
	t := database.Task{}
	t.FinishedAt = now
	t.ID = req.TaskID

	if req.Code != 0 {
		t.Status = database.TaskFailedStatus
		t.Errors = req.Msg

		err := orm.SetTask(t)
		if err != nil {
			ec := errCodeV1(_Task, dbExecError, 33, "fail to exec records into database", "数据库更新错误（任务表）")
			httpJSONError(w, err, ec, http.StatusInternalServerError)
		}
		return
	}

	if req.Retention == 0 {
		// default keep a week
		req.Retention = 7
	}

	bf := database.BackupFile{
		ID:         utils.Generate32UUID(),
		TaskID:     req.TaskID,
		UnitID:     req.UnitID,
		Tag:        req.Tag,
		Type:       req.Type,
		Tables:     req.Tables,
		Path:       req.Path,
		Remark:     req.Remark,
		SizeByte:   req.Size,
		Retention:  now.AddDate(0, 0, req.Retention),
		CreatedAt:  time.Unix(req.Created, 0),
		FinishedAt: time.Unix(req.Finished, 0),
	}

	t.Status = database.TaskDoneStatus
	t.SetErrors(nil)

	err = orm.InsertBackupFileWithTask(bf, t)
	if err != nil {
		ec := errCodeV1(_Task, dbTxError, 34, "fail to exec records in into database in a Tx", "数据库事务处理错误（备份文件表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func setTaskFailed(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil {
		httpJSONNilGarden(w)
		return
	}

	name := mux.Vars(r)["name"]

	err := gd.Ormer().SetTaskFail(name)
	if err != nil {
		ec := errCodeV1(_Task, dbExecError, 41, "fail to update database", "数据库更新错误（任务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// -----------------/datacenter handler-----------------
func validDatacenter(v structs.RegisterDC) error {
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

	if err := validDatacenter(req); err != nil {
		ec := errCodeV1(_DC, invalidParamsError, 12, "Body parameters are invalid", "Body参数校验错误，包含无效参数")
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
		if database.IsNotFound(err) {
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
				ID:    images[i].ID,
				Name:  images[i].Name,
				Major: images[i].Major,
				Minor: images[i].Minor,
				Patch: images[i].Patch,
				Dev:   images[i].Dev,
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

func validLoadImageRequest(v structs.PostLoadImageRequest) error {
	errs := make([]string, 0, 2)

	if v.Name == "" ||
		(v.Major == 0 && v.Minor == 0 &&
			v.Patch == 0 && v.Dev == 0) {
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

	if err := validLoadImageRequest(req); err != nil {
		ec := errCodeV1(_Image, invalidParamsError, 33, "Body parameters are invalid", "Body参数校验错误，包含无效参数")
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
			version.Minor == req.Minor {

			found = true
			break
		}
	}

	if !found {
		ec := errCodeV1(_Image, objectNotExist, 35, "unsupported image:"+req.Image(), "不支持镜像:"+req.Image())
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
		if database.IsNotFound(err) {
			writeJSONNull(w, http.StatusOK)
			return
		}

		ec := errCodeV1(_Image, dbQueryError, 51, "fail to query database", "数据库查询错误（镜像表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	imResp := structs.ImageResponse{
		ImageVersion: structs.ImageVersion{
			ID:    im.ID,
			Name:  im.Name,
			Major: im.Major,
			Minor: im.Minor,
			Patch: im.Patch,
			Dev:   im.Dev,
		},
		Size:     im.Size,
		ID:       im.ID,
		ImageID:  im.ImageID,
		Labels:   im.Labels,
		UploadAt: utils.TimeToString(im.UploadAt),
	}

	t, err := gd.PluginClient().GetImage(ctx, im.Image())
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
		if database.IsNotFound(err) {
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
		ID:               c.ID,
		NetworkPartition: c.NetworkPartition,
		MaxNode:          c.MaxNode,
		UsageLimit:       c.UsageLimit,
		NodeNum:          n,
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
			ID:               list[i].ID,
			MaxNode:          list[i].MaxNode,
			UsageLimit:       list[i].UsageLimit,
			NetworkPartition: list[i].NetworkPartition,
			NodeNum:          n,
		}
	}

	writeJSON(w, out, http.StatusOK)
}

func validPostClusterRequest(v structs.PostClusterRequest) error {
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

	if err := validPostClusterRequest(req); err != nil {
		ec := errCodeV1(_Cluster, invalidParamsError, 32, "Body parameters are invalid", "Body参数校验错误，包含无效参数")
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
		ID:               utils.Generate32UUID(),
		NetworkPartition: req.NetworkPartition,
		MaxNode:          req.Max,
		UsageLimit:       req.UsageLimit,
	}

	err = gd.Ormer().InsertCluster(c)
	if err != nil {
		ec := errCodeV1(_Cluster, dbExecError, 33, "fail to insert records into database", "数据库新增记录错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%q}", "id", c.ID)
}

func validPutClusterRequest(v structs.PutClusterRequest) error {
	if v.Max == nil && v.UsageLimit == nil {
		return errors.Errorf("neither MaxNode nor UsageLimit is non-nil")
	}

	return nil
}

func putClusterParams(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	req := structs.PutClusterRequest{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		ec := errCodeV1(_Cluster, decodeError, 41, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if err := validPutClusterRequest(req); err != nil {
		ec := errCodeV1(_Cluster, invalidParamsError, 42, "Body parameters are invalid", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	c, err := gd.Ormer().GetCluster(name)
	if err != nil {
		ec := errCodeV1(_Cluster, dbQueryError, 43, "fail to query records from database", "数据库查询记录错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	if req.Max != nil {
		c.MaxNode = *req.Max
	}

	if req.UsageLimit != nil {
		c.UsageLimit = *req.UsageLimit
	}

	err = gd.Ormer().SetClusterParams(c)
	if err != nil {
		ec := errCodeV1(_Cluster, dbExecError, 44, "fail to update records into database", "数据库更新记录错误")
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
		ID:            n.ID,
		Cluster:       n.ClusterID,
		Room:          n.Room,
		Seat:          n.Seat,
		MaxContainer:  n.MaxContainer,
		Enabled:       n.Enabled,
		RegisterAt:    utils.TimeToString(n.RegisterAt),
		VolumeDrivers: []structs.VolumeDriver{},
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
			if d == nil || d.Type() == "NFS" ||
				d.Type() == storage.SANStore {
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

		if len(vds) > 0 {
			info.VolumeDrivers = vds
		}
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
		if database.IsNotFound(err) {
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

func validNodeRequest(node structs.Node) error {
	errs := make([]string, 0, 3)

	if node.Cluster == "" {
		errs = append(errs, "Cluster is required")
	}

	if node.Addr == "" {
		errs = append(errs, "Addr is required")
	} else if net.ParseIP(node.Addr) == nil {
		errs = append(errs, "parse Addr error")
	}

	// valid ssh config
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

	if err := validNodeRequest(n); err != nil {
		ec := errCodeV1(_Host, invalidParamsError, 32, "Body parameters are invalid", "Body参数校验错误，包含无效参数")
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

	cl, err := orm.GetCluster(n.Cluster)
	if err != nil {
		ec := errCodeV1(_Host, dbQueryError, 33, "fail to query database", "数据库查询错误（集群表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	num, err := orm.CountNodeByCluster(cl.ID)
	if err != nil {
		ec := errCodeV1(_Host, dbQueryError, 34, "fail to query database", "数据库查询错误（物理主机表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	if num >= cl.MaxNode {
		ec := errCodeV1(_Host, invalidParamsError, 35, fmt.Sprintf("Exceeded cluster max node limit,%d>=%d", num, cl.MaxNode), fmt.Sprintf("超出集群数量限制，%d>%d", num, cl.MaxNode))
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if n.Storage != "" {
		_, err = orm.GetStorageByID(n.Storage)
		if err != nil {
			ec := errCodeV1(_Host, dbQueryError, 36, "fail to query database", "数据库查询错误（外部存储表）")
			httpJSONError(w, err, ec, http.StatusInternalServerError)
			return
		}
	}

	node := database.Node{
		ID:           utils.Generate32UUID(),
		ClusterID:    n.Cluster,
		Addr:         n.Addr,
		EngineID:     "",
		Room:         n.Room,
		Seat:         n.Seat,
		Storage:      n.Storage,
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

	nodes := resource.NewNodeWithTaskList(1)
	nodes[0] = resource.NewNodeWithTask(node, n.HDD, n.SSD, n.SSHConfig)

	horus, err := gd.KVClient().GetHorusAddr(ctx)
	if err != nil {
		ec := errCodeV1(_Host, internalError, 37, "fail to query third-part monitor server addr", "获取第三方监控服务地址错误")
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
		ec := errCodeV1(_Host, internalError, 38, "fail to install host", "主机入库错误")
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
		N *int `json:"max_container"`
	}{}

	err := json.NewDecoder(r.Body).Decode(&max)
	if err != nil {
		ec := errCodeV1(_Host, decodeError, 61, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if max.N == nil {
		ec := errCodeV1(_Host, invalidParamsError, 62, "URL parameters are invalid", "URL参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	err = gd.Ormer().SetNodeParam(name, *max.N)
	if err != nil {
		ec := errCodeV1(_Host, dbExecError, 63, "fail to update records into database", "数据库更新记录错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func validDelNodesRequest(name, user string) error {
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

	port := r.FormValue("port")
	username := r.FormValue("username")
	password := r.FormValue("password")

	if err := validDelNodesRequest(node, username); err != nil {
		ec := errCodeV1(_Host, invalidParamsError, 72, "URL parameters are invalid", "URL参数校验错误，包含无效参数")
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
	err = m.RemoveNode(ctx, port, horus, node, username, password, force, timeout, gd.KVClient())
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
		ec := errCodeV1(_Networking, invalidParamsError, 12, "Body parameters are invalid", "Body参数校验错误，包含无效参数")
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
				ec := errCodeV1(_Networking, invalidParamsError, 24, fmt.Sprintf("IP %s is not in networking %s", body[i], name), fmt.Sprintf("IP %s 不属于指定网络集群(%s)", body[i], name))
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

	writeJSONNull(w, http.StatusOK)
}

// -----------------/services handlers-----------------
func getServices(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ec := errCodeV1(_Service, urlParamError, 11, "parse Request URL parameter error", "解析请求URL参数错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil || gd.Ormer() == nil || gd.Cluster == nil {
		httpJSONNilGarden(w)
		return
	}

	services, err := gd.ListServices(ctx)
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 12, "fail to query database", "数据库查询错误（服务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	if len(services) == 0 {
		writeJSON(w, services, http.StatusOK)
		return
	}

	var out []structs.ServiceSpec
	name := r.FormValue("image_name")

	if name == "" {
		out = services
	} else {
		images := strings.Split(name, ",")
		out = make([]structs.ServiceSpec, 0, len(services))

		for i := range images {

			for k := range services {
				if services[k].Image.Name == images[i] {
					out = append(out, services[k])
				}
			}
		}
	}

	bfs, err := gd.Ormer().ListBackupFiles()
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 13, "fail to query database", "数据库查询错误（备份表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	resp := make([]structs.ServiceResponse, 0, len(out))

	for i := range out {
		sum := 0
		for j := range bfs {

		units:
			for k := range out[i].Units {
				if bfs[j].UnitID == out[i].Units[k].ID {
					sum += bfs[j].SizeByte
					break units
				}
			}
		}

		resp = append(resp, structs.ServiceResponse{
			ServiceSpec:   out[i],
			BuckupFileSum: sum,
		})
	}

	writeJSON(w, resp, http.StatusOK)
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
		if database.IsNotFound(err) {
			writeJSONNull(w, http.StatusOK)
			return
		}

		ec := errCodeV1(_Service, dbQueryError, 21, "fail to query database", "数据库查询错误（服务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	sum, err := getBackupFileSumByService(gd.Ormer(), spec.ID)
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 22, "fail to query database", "数据库查询错误（备份表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	resp := structs.ServiceResponse{
		ServiceSpec:   spec,
		BuckupFileSum: sum,
	}

	writeJSON(w, resp, http.StatusOK)
}

func getBackupFileSumByService(biface database.BackupFileIface, id string) (int, error) {
	files, err := biface.ListBackupFilesByService(id)
	if err != nil {
		return 0, err
	}

	sum := 0
	for i := range files {
		sum += files[i].SizeByte
	}

	return sum, nil
}

func validPostServiceRequest(spec structs.ServiceSpec) error {
	errs := make([]string, 0, 5)

	if spec.Image.ID == "" {
		errs = append(errs, fmt.Sprintf("Image ID is required,%+v", spec.Image))
	}

	if spec.Arch.Code == "" || spec.Arch.Mode == "" || spec.Arch.Replicas == 0 {
		errs = append(errs, fmt.Sprintf("Arch invalid,%+v", spec.Arch))
	}

	if spec.Require == nil {
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

	compose := boolValue(r, "compose")
	timeout := intValueOrZero(r, "timeout")

	spec := structs.ServiceSpec{}
	err := json.NewDecoder(r.Body).Decode(&spec)
	if err != nil {
		ec := errCodeV1(_Service, decodeError, 32, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if err := validPostServiceRequest(spec); err != nil {
		ec := errCodeV1(_Service, invalidParamsError, 33, "Body parameters are invalid", "Body参数校验错误，包含无效参数")
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

	out, err := d.Deploy(ctx, spec, compose)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 34, "fail to deploy service", "创建服务错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSON(w, out, http.StatusCreated)
}

func validPostServiceScaledRequest(v structs.ServiceScaleRequest) error {
	errs := make([]string, 0, 2)

	if v.Arch.Code == "" || v.Arch.Mode == "" || v.Arch.Replicas == 0 {
		errs = append(errs, fmt.Sprintf("Arch invalid,%+v", v.Arch))
	}

	for i := range v.Candidates {
		if v.Candidates[i] == "" {
			errs = append(errs, "Candidate value is null")
		}
	}

	if len(errs) == 0 {
		return nil
	}

	return fmt.Errorf("ServiceScaleRequest:%v,%s", v, errs)
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

	if err := validPostServiceScaledRequest(scale); err != nil {
		ec := errCodeV1(_Service, invalidParamsError, 42, "Body parameters are invalid", "Body参数校验错误，包含无效参数")
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

	resp, err := d.ServiceScale(ctx, name, scale)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 43, "fail to scale service", "服务水平扩展错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSON(w, resp, http.StatusCreated)
}

func validPostServiceLinkRequest(v structs.ServicesLink) error {
	if v.Len() == 0 {
		return fmt.Errorf("invalid params")
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

	if err := validPostServiceLinkRequest(links); err != nil {
		ec := errCodeV1(_Service, invalidParamsError, 52, "Body parameters are invalid", "Body参数校验错误，包含无效参数")
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
		ctx, _ = goctx.WithTimeout(goctx.Background(), 5*time.Minute)
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

func validPostServiceUpdateRequest(v structs.UpdateUnitRequire) error {
	if v.Require.CPU == nil && v.Require.Memory == nil &&
		len(v.Volumes) == 0 && len(v.Networks) == 0 {

		return fmt.Errorf("UnitRequire:%v,%s", v, "no resource update required")
	}

	errs := make([]string, 0, 3)

	for _, vr := range v.Volumes {
		if vr.Name == "" && vr.Type == "" {
			errs = append(errs, fmt.Sprintf("VolumeRequire is invalid,%+v", vr))
		}
	}

	if len(errs) == 0 {
		return nil
	}

	return fmt.Errorf("ServiceUpdateRequest:%s", errs)
}

func postServiceUpdate(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	config := structs.UpdateUnitRequire{}

	err := json.NewDecoder(r.Body).Decode(&config)
	if err != nil {
		ec := errCodeV1(_Service, decodeError, 71, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if err := validPostServiceUpdateRequest(config); err != nil {
		ec := errCodeV1(_Service, invalidParamsError, 72, "Body parameters are invalid", "Body参数校验错误，包含无效参数")
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

	id, err := d.ServiceUpdate(ctx, name, config)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 73, "fail to update service containers", "服务垂直扩容错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%q}", "task_id", id)
}

func postServiceStart(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ec := errCodeV1(_Service, urlParamError, 81, "parse Request URL parameter error", "解析请求URL参数错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	name := mux.Vars(r)["name"]
	unit := r.FormValue("unit")

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil || gd.KVClient() == nil {

		httpJSONNilGarden(w)
		return
	}

	svc, err := gd.Service(name)
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

	task := database.NewTask(svc.Name(), database.ServiceStartTask, svc.ID(), "", nil, 300)

	err = svc.InitStart(ctx, unit, gd.KVClient(), nil, &task, true, nil)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 83, "fail to init start service", "服务初始化启动错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%q}", "task_id", task.ID)
}

func validPostServiceUpdateConfigsRequest(req structs.ModifyUnitConfig) error {
	if len(req.Keysets) == 0 &&
		req.ConfigFile == nil &&
		req.DataMount == nil &&
		req.LogMount == nil &&
		req.Content == nil &&
		len(req.Cmds) == 0 {

		return stderr.New("nothing new for update for service configs")
	}

	return nil
}

func mergeServiceConfigsChange(ctx goctx.Context, svc *garden.Service, change structs.ModifyUnitConfig) (structs.ServiceConfigs, bool, error) {
	restart := false

	configs, err := svc.GetUnitsConfigs(ctx)
	if err != nil {
		return nil, false, err
	}

	for i := range configs {
		cc, ok, err := mergeUnitConfigChange(configs[i], change)
		if err != nil {
			return nil, false, err
		}

		configs[i] = cc

		if ok {
			restart = ok
		}
	}

	return configs, restart, nil
}

func mergeUnitConfigChange(cc structs.UnitConfig, change structs.ModifyUnitConfig) (structs.UnitConfig, bool, error) {
	if change.LogMount != nil {
		cc.LogMount = *change.LogMount
	}
	if change.DataMount != nil {
		cc.DataMount = *change.DataMount
	}
	if change.ConfigFile != nil {
		cc.ConfigFile = *change.ConfigFile
	}
	if change.Content != nil {
		cc.Content = *change.Content
	}

	m := make(map[string]structs.Keyset, len(cc.Keysets))

	for i := range cc.Keysets {
		m[cc.Keysets[i].Key] = cc.Keysets[i]
	}

	restart := false

	for i := range change.Keysets {
		val, ok := m[change.Keysets[i].Key]
		if !ok {
			return cc, false, errors.Errorf("UnitConfig key:%s not exist", change.Keysets[i].Key)
		}

		if !val.CanSet {
			return cc, false, errors.Errorf("UnitConfig key:%s forbit to reset", val.Key)
		}

		if val.MustRestart {
			restart = true
		}

		val.Value = change.Keysets[i].Value

		m[change.Keysets[i].Key] = val
	}

	ks := make([]structs.Keyset, 0, len(m))

	for _, val := range m {
		ks = append(ks, val)
	}

	cc.Keysets = ks

	return cc, restart, nil
}

func postServiceUpdateConfigs(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	change := structs.ModifyUnitConfig{}

	err := json.NewDecoder(r.Body).Decode(&change)
	if err != nil {
		ec := errCodeV1(_Service, decodeError, 91, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if err := validPostServiceUpdateConfigsRequest(change); err != nil {
		ec := errCodeV1(_Service, invalidParamsError, 92, "Body parameters are invalid", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	svc, err := gd.Service(name)
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 93, "fail to query database", "数据库查询错误（服务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	configs, restart, err := mergeServiceConfigsChange(ctx, svc, change)
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 94, "fail to update config file", "数据合并错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	// new Context with deadline
	if deadline, ok := ctx.Deadline(); !ok {
		ctx = goctx.Background()
	} else {
		ctx, _ = goctx.WithDeadline(goctx.Background(), deadline)
	}

	task := database.NewTask(svc.Name(), database.ServiceUpdateConfigTask, svc.ID(), "", nil, 300)

	err = svc.UpdateUnitsConfigs(ctx, configs, &task, restart, true)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 95, "fail to update service config files", "服务配置文件更新错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%q}", "task_id", task.ID)
}

func validPostServiceExecRequest(v structs.ServiceExecConfig) error {
	if len(v.Cmd) == 0 {
		return stderr.New("ServiceExecConfig.Cmd is required")
	}

	return nil
}

func postServiceExec(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ec := errCodeV1(_Service, urlParamError, 101, "parse Request URL parameter error", "解析请求URL参数错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	name := mux.Vars(r)["name"]

	config := structs.ServiceExecConfig{}
	err := json.NewDecoder(r.Body).Decode(&config)
	if err != nil {
		ec := errCodeV1(_Service, decodeError, 102, "JSON Decode Request Body error", "JSON解析请求Body错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	if err := validPostServiceExecRequest(config); err != nil {
		ec := errCodeV1(_Service, invalidParamsError, 103, "Body parameters are invalid", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil || gd.Cluster == nil {

		httpJSONNilGarden(w)
		return
	}

	svc, err := gd.Service(name)
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 104, "fail to query database", "数据库查询错误（服务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	if boolValue(r, "sync") {

		var resp []structs.ContainerExecOutput

		execFunc := func() error {
			resp, err = svc.ContainerExec(ctx, config.Container, config.Cmd, config.Detach)

			return err
		}

		err = svc.ExecLock(execFunc, false, nil)
		if err != nil {
			ec := errCodeV1(_Service, internalError, 105, "fail to exec command in service containers", "服务容器远程命令执行错误（container exec）")
			httpJSONError(w, err, ec, http.StatusInternalServerError)
			return
		}

		if config.Container != "" && len(resp) == 1 {
			writeJSON(w, resp[0], http.StatusCreated)
		} else {
			writeJSON(w, resp, http.StatusCreated)
		}

		return
	}

	// new Context with deadline
	if deadline, ok := ctx.Deadline(); !ok {
		ctx = goctx.Background()
	} else {
		ctx, _ = goctx.WithDeadline(goctx.Background(), deadline)
	}

	task := database.NewTask(svc.Name(), database.ServiceExecTask, svc.ID(), "", nil, 300)

	err = svc.Exec(ctx, config, true, &task)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 106, "fail to exec command in service containers", "服务容器远程命令执行错误（container exec）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%q}", "task_id", task.ID)
}

func postServiceStop(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ec := errCodeV1(_Service, urlParamError, 111, "parse Request URL parameter error", "解析请求URL参数错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	name := mux.Vars(r)["name"]
	unit := r.FormValue("unit")

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	svc, err := gd.Service(name)
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

	task := database.NewTask(svc.Name(), database.ServiceStopTask, svc.ID(), "", nil, 300)

	err = svc.Stop(ctx, unit, true, true, &task)
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

	table, err := gd.Ormer().GetService(name)
	if err != nil {
		if database.IsNotFound(err) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		ec := errCodeV1(_Service, dbQueryError, 122, "fail to query database", "数据库查询错误（服务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	ctx, cancel := goctx.WithTimeout(ctx, time.Minute*5)
	defer cancel()

	svc := gd.NewService(nil, &table)
	err = svc.Remove(ctx, gd.KVClient(), force)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 123, "fail to remove service", "删除服务错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func validPostServiceBackupRequest(v structs.ServiceBackupConfig) error {
	if v.Container == "" {
		return stderr.New("not assigned unit nameOrID")
	}

	return nil
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

	if err := validPostServiceBackupRequest(config); err != nil {
		ec := errCodeV1(_Service, invalidParamsError, 132, "Body parameters are invalid", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil || gd.Cluster == nil {

		httpJSONNilGarden(w)
		return
	}

	if config.BackupDir == "" {
		sys, err := gd.Ormer().GetSysConfig()
		if err != nil {
			ec := errCodeV1(_Service, dbQueryError, 133, "fail to query database", "数据库查询错误（系统配置表）")
			httpJSONError(w, err, ec, http.StatusInternalServerError)
			return
		}

		config.BackupDir = sys.BackupDir
	}
	if config.Type == "" {
		config.Type = "full"
	}
	if config.Tables == "" {
		config.Tables = "null"
	}
	if config.FilesRetention == 0 {
		config.FilesRetention = 7
	}

	svc, err := gd.Service(name)
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

	task := database.NewTask(svc.Name(), database.ServiceBackupTask, svc.ID(), "", nil, 300)

	err = svc.Backup(ctx, r.Host, config, true, &task)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 135, "fail to back service", "服务备份错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%q}", "task_id", task.ID)
}

func validPostServiceRestoreRequest(v structs.ServiceRestoreRequest) error {
	errs := make([]string, 0, 2)

	if v.File == "" {
		errs = append(errs, "restore file is required")
	}

	if len(v.Units) == 0 {
		errs = append(errs, "restore unit without assigned")
	}

	if len(errs) == 0 {
		return nil
	}

	return fmt.Errorf("ServiceRestoreRequest:%v,%s", v, errs)
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

	if err := validPostServiceRestoreRequest(req); err != nil {
		ec := errCodeV1(_Service, invalidParamsError, 142, "Body parameters are invalid", "Body参数校验错误，包含无效参数")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	// new Context with deadline
	if deadline, ok := ctx.Deadline(); !ok {
		ctx = goctx.Background()
	} else {
		ctx, _ = goctx.WithDeadline(goctx.Background(), deadline)
	}

	svc, err := gd.Service(name)
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 143, "fail to query database", "数据库查询错误（服务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	id, err := svc.UnitRestore(ctx, req.Units, req.File, true)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 144, "fail to restore unit data", "服务单元数据恢复错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%q}", "task_id", id)
}

func validPostUnitRebuildRequest(v structs.UnitRebuildRequest) error {
	errs := make([]string, 0, 2)

	if v.NameOrID == "" {
		errs = append(errs, "Unit name or ID is required")
	}

	for i := range v.Candidates {
		if v.Candidates[i] == "" {
			errs = append(errs, "Candidate value is null")
		}
	}

	if len(errs) == 0 {
		return nil
	}

	return fmt.Errorf("UnitRebuildRequest:%v,%s", v, errs)
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

	if err := validPostUnitRebuildRequest(req); err != nil {
		ec := errCodeV1(_Service, invalidParamsError, 152, "Body parameters are invalid", "Body参数校验错误，包含无效参数")
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

func validPostUnitMigrateRequest(v structs.PostUnitMigrate) error {
	errs := make([]string, 0, 2)

	if v.NameOrID == "" {
		errs = append(errs, "Unit name or ID is required")
	}

	for i := range v.Candidates {
		if v.Candidates[i] == "" {
			errs = append(errs, "Candidate value is null")
		}
	}

	if len(errs) == 0 {
		return nil
	}

	return fmt.Errorf("PostUnitMigrate:%v,%s", v, errs)
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

	if err := validPostUnitMigrateRequest(req); err != nil {
		ec := errCodeV1(_Service, invalidParamsError, 162, "Body parameters are invalid", "Body参数校验错误，包含无效参数")
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

	id, err := gd.ServiceMigrate(ctx, svc, req, true)
	if err != nil {
		ec := errCodeV1(_Service, internalError, 164, "fail to scale service", "服务水平扩展错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSONFprintf(w, http.StatusCreated, "{%q:%q}", "task_id", id)
}

func getServiceConfigFiles(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ec := errCodeV1(_Service, urlParamError, 171, "parse Request URL parameter error", "解析请求URL参数错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	name := mux.Vars(r)["name"]
	canset := boolValue(r, "canset")

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	svc, err := gd.Service(name)
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 172, "fail to query database", "数据库查询错误（服务表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	out, err := svc.GetUnitsConfigs(ctx)
	if err != nil {
		ec := errCodeV1(_Service, dbQueryError, 173, "fail to query units configs from kv", "获取服务单元配置错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	if !canset {
		writeJSON(w, out, http.StatusOK)
		return
	}

	config := structs.UnitConfig{}

	if len(out) > 0 {
		config = out[0]
		config.ID = ""
		config.Service = ""
		config.Content = ""
		config.Cmds = nil

		keysets := make([]structs.Keyset, 0, len(out[0].Keysets))
		for i := range out[0].Keysets {
			if out[0].Keysets[i].CanSet {
				keysets = append(keysets, out[0].Keysets[i])
			}
		}

		config.Keysets = keysets
	}

	writeJSON(w, structs.ServiceConfigs{config}, http.StatusOK)
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

	respCh := make(chan structs.SANStorageResponse, len(stores))
	for i := range stores {
		go func(store storage.Store, ch chan structs.SANStorageResponse) {
			info, _ := getSanStoreInfo(store)
			ch <- info
		}(stores[i], respCh)
	}

	resp := make([]structs.SANStorageResponse, len(stores))
	for i := 0; i < len(stores); i++ {
		resp[i] = <-respCh
	}

	writeJSON(w, resp, http.StatusOK)
}

// GET /storage/san/{name:.*}
func getSANStorageInfo(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ds := storage.DefaultStores()
	store, err := ds.Get(name)
	if err != nil {
		if database.IsNotFound(err) {
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
	if store == nil {
		return structs.SANStorageResponse{}, nil
	}

	info, err := store.Info()
	if err != nil {
		return structs.SANStorageResponse{
			ID:     store.ID(),
			Vendor: store.Vendor(),
			Driver: store.Driver(),
			Error:  err.Error(),
		}, err
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

func validPostSanStorageRequest(v structs.PostSANStoreRequest) error {
	errs := make([]string, 0, 2)

	if v.Vendor == "" {
		errs = append(errs, "Vendor is required")
	}

	if v.HostLunStart > v.HostLunEnd || v.HostLunEnd < 0 {
		errs = append(errs, "host_lun_end or host_lun_start is invalid")

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

	if err := validPostSanStorageRequest(req); err != nil {
		ec := errCodeV1(_Storage, invalidParamsError, 32, "Body parameters are invalid", "Body参数校验错误，包含无效参数")
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

// -----------------/backupfiles handlers-----------------
// DELETE /backupfiles
func deleteBackupFiles(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ec := errCodeV1(_Backup, urlParamError, 11, "parse Request URL parameter error", "解析请求URL参数错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	id := r.FormValue("id")
	tag := r.FormValue("tag")
	service := r.FormValue("service")
	deadline := r.FormValue("expired")
	nfs := r.FormValue("nfs_mount")

	if nfs == "" {
		ec := errCodeV1(_Backup, urlParamError, 12, "Request URL parameter error", "请求URL参数错误,miss nfs_mount")
		httpJSONError(w, errors.New("miss nfs_mount"), ec, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil || gd.Ormer() == nil {
		httpJSONNilGarden(w)
		return
	}

	var (
		orm = gd.Ormer()

		err   error
		files []database.BackupFile
	)

	if id != "" {
		bf, err := orm.GetBackupFile(id)
		if err != nil {
			ec := errCodeV1(_Backup, dbQueryError, 13, "fail to query database", "数据库查询错误（备份文件表）")
			httpJSONError(w, err, ec, http.StatusInternalServerError)
			return
		}

		files = []database.BackupFile{bf}

		err = removeBackupFiles(orm, nfs, files)
		if err != nil {
			ec := errCodeV1(_Backup, dbExecError, 14, "fail to remove backup files", "删除备份文件错误")
			httpJSONError(w, err, ec, http.StatusInternalServerError)
		}

		return
	}

	if tag != "" {
		orm.ListBackupFilesByTag(tag)
	} else if service != "" {
		files, err = orm.ListBackupFilesByService(service)
	} else {
		files, err = orm.ListBackupFiles()
	}

	if err != nil {
		ec := errCodeV1(_Backup, dbQueryError, 15, "fail to query database", "数据库查询错误（备份文件表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	t := time.Now()
	if deadline != "" {
		d, err := time.Parse(dateLayout, deadline)
		if err == nil && d.Before(t) {
			t = d
		}
	}

	expired := make([]database.BackupFile, 0, len(files))
	for i := range files {
		if t.After(files[i].Retention) {
			expired = append(expired, files[i])
		}
	}

	if len(expired) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	err = removeBackupFiles(orm, nfs, expired)
	if err != nil {
		ec := errCodeV1(_Backup, dbExecError, 16, "fail to remove backup files", "删除备份文件错误")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type rmBackupFilesIface interface {
	GetSysConfig() (database.SysConfig, error)
	DelBackupFiles(files []database.BackupFile) error
}

func removeBackupFiles(orm rmBackupFilesIface, nfs string, files []database.BackupFile) error {
	sys, err := orm.GetSysConfig()
	if err != nil {
		return err
	}

	rm := make([]database.BackupFile, 0, len(files))
	for i := range files {
		path := getNFSBackupFile(files[i].Path, sys.BackupDir, nfs)

		err := os.RemoveAll(path)
		if err == nil {
			rm = append(rm, files[i])
		} else {
			logrus.Warnf("fail to delete backup file:%s", path)
		}
	}

	return orm.DelBackupFiles(rm)
}

func getNFSBackupFile(file, backup, nfs string) string {
	file = strings.Replace(file, backup, nfs, 1)

	return filepath.Clean(file)
}

func getBackupFiles(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ec := errCodeV1(_Backup, urlParamError, 21, "parse Request URL parameter error", "解析请求URL参数错误")
		httpJSONError(w, err, ec, http.StatusBadRequest)
		return
	}

	tag := r.FormValue("tag")
	service := r.FormValue("service")
	start := r.FormValue("start")
	end := r.FormValue("end")

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil || gd.Ormer() == nil {
		httpJSONNilGarden(w)
		return
	}

	var (
		orm = gd.Ormer()

		err   error
		files []database.BackupFile
	)

	if tag != "" {
		files, err = orm.ListBackupFilesByTag(tag)
	} else if service != "" {
		files, err = orm.ListBackupFilesByService(service)
	} else {
		files, err = orm.ListBackupFiles()
		files = filterBackupFilesByTime(files, start, end)
	}
	if err != nil {
		ec := errCodeV1(_Backup, dbQueryError, 22, "fail to query database", "数据库查询错误（备份文件表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	if files == nil {
		files = []database.BackupFile{}
	}

	writeJSON(w, files, http.StatusOK)
}

func filterBackupFilesByTime(files []database.BackupFile, start, end string) []database.BackupFile {
	if len(files) == 0 || (start == "" && end == "") {
		return files
	}

	var (
		err    error
		st, et time.Time
	)

	if start != "" {
		st, err = time.Parse(dateLayout, start)
		if err != nil {
			return files
		}
	}

	if end != "" {
		et, err = time.Parse(dateLayout, start)
		if err != nil {
			return files
		}
	}

	if st.After(et) {
		st, et = et, st
	}

	out := make([]database.BackupFile, 0, len(files))

	sN0 := !st.IsZero()
	eN0 := !et.IsZero()

	for i := range files {
		at := files[i].CreatedAt

		if sN0 && at.Before(st) {
			continue
		}

		if eN0 && at.After(et) {
			continue
		}

		out = append(out, files[i])
	}

	return out
}

func getBackupFile(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil || gd.Ormer() == nil {
		httpJSONNilGarden(w)
		return
	}

	bf, err := gd.Ormer().GetBackupFile(id)
	if err != nil {
		ec := errCodeV1(_Backup, dbQueryError, 31, "fail to query database", "数据库查询错误（备份文件表）")
		httpJSONError(w, err, ec, http.StatusInternalServerError)
		return
	}

	writeJSON(w, bf, http.StatusOK)
}
