package api

import (
	"database/sql"
	"encoding/json"
	stderr "errors"
	"fmt"
	"net"
	"net/http"
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

var errUnsupportGarden = stderr.New("unsupport Garden yet")

func httpJSONNilGarden(w http.ResponseWriter) {
	const nilGardenCode = 100000000

	httpJSONError(w, errUnsupportGarden, nilGardenCode, http.StatusInternalServerError)
}

// Emit an HTTP error and log it.
func httpJSONError(w http.ResponseWriter, err error, code, status int) {
	field := logrus.WithField("status", status)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err != nil {
		json.NewEncoder(w).Encode(structs.ResponseHead{
			Result:  false,
			Code:    code,
			Message: err.Error(),
		})

		field.Errorf("HTTP error: %+v", err)
	}
}

// Emit an HTTP error with object and log it.
func httpJSONErrorWithBody(w http.ResponseWriter, obj interface{}, err error, status int) {
	field := logrus.WithField("status", status)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err != nil {
		json.NewEncoder(w).Encode(structs.ResponseHead{
			Result:  false,
			Code:    status,
			Message: err.Error(),
			Object:  obj,
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

func proxySpecialLogic(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ec := errCodeV1(r.Method, _NFS, urlParamError, 11)
		httpJSONError(w, err, ec.code, http.StatusBadRequest)
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
	port := r.FormValue("port")
	orm := gd.Ormer()

	u, err := orm.GetUnit(name)
	if err != nil {
		ec := errCodeV1(r.Method, _Unit, dbQueryError, 11)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	ips, err := orm.ListIPByUnitID(u.ID)
	if err != nil {
		ec := errCodeV1(r.Method, _Unit, dbQueryError, 12)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	if len(ips) == 0 {
		ec := errCodeV1(r.Method, _Unit, objectNotExist, 13)
		httpJSONError(w, errors.Errorf("not found Unit %s address", u.Name), ec.code, http.StatusInternalServerError)
		return
	}

	r.URL.Path = "/" + proxyURL
	addr := utils.Uint32ToIP(ips[0].IPAddr).String()
	addr = net.JoinHostPort(addr, port)

	err = hijack(nil, addr, w, r)
	if err != nil {
		ec := errCodeV1(r.Method, _Unit, internalError, 14)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}
}

// -----------------/nfs_backups handlers-----------------
func getNFSSPace(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ec := errCodeV1(r.Method, _NFS, urlParamError, 11)
		httpJSONError(w, err, ec.code, http.StatusBadRequest)
		return
	}

	nfs := database.NFS{}

	nfs.Addr = r.FormValue("nfs_ip")
	nfs.Dir = r.FormValue("nfs_dir")
	nfs.MountDir = r.FormValue("nfs_mount_dir")
	nfs.Options = r.FormValue("nfs_mount_opts")

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONNilGarden(w)
		return
	}

	sys, err := gd.Ormer().GetSysConfig()
	if err != nil {
		ec := errCodeV1(r.Method, _NFS, dbQueryError, 12)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	abs, err := utils.GetAbsolutePath(true, sys.SourceDir)
	if err != nil {
		ec := errCodeV1(r.Method, _NFS, objectNotExist, 13)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	d := driver.NewNFSDriver(nfs, filepath.Dir(abs), sys.BackupDir)

	space, err := d.Space()
	if err != nil {
		ec := errCodeV1(r.Method, _NFS, internalError, 14)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"total_space": %d,"free_space": %d}`, space.Total, space.Free)
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
		ec := errCodeV1(r.Method, _Task, dbQueryError, 11)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	writeJSON(w, t, http.StatusOK)
}

func getTasks(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ec := errCodeV1(r.Method, _Task, urlParamError, 21)
		httpJSONError(w, err, ec.code, http.StatusBadRequest)
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
		ec := errCodeV1(r.Method, _Task, dbQueryError, 22)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	writeJSON(w, out, http.StatusOK)
}

// -----------------/datacenter handler-----------------
func postRegisterDC(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.RegisterDC{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		ec := errCodeV1(r.Method, _DC, decodeError, 11)
		httpJSONError(w, err, ec.code, http.StatusBadRequest)
		return
	}
	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil {
		httpJSONNilGarden(w)
		return
	}

	err = gd.Register(req)
	if err != nil {
		ec := errCodeV1(r.Method, _DC, internalError, 12)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	writeJSON(w, nil, http.StatusCreated)
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
		ec := errCodeV1(r.Method, _Image, dbQueryError, 11)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	out := make([]structs.GetImageResponse, len(images))

	for i := range images {
		out[i] = structs.GetImageResponse{
			ImageVersion: structs.ImageVersion{
				Name:  images[i].Name,
				Major: images[i].Major,
				Minor: images[i].Minor,
				Patch: images[i].Patch,
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
		ec := errCodeV1(r.Method, _Image, internalError, 21)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	writeJSON(w, out, http.StatusOK)
}

func postImageLoad(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.PostLoadImageRequest{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		ec := errCodeV1(r.Method, _Image, decodeError, 31)
		httpJSONError(w, err, ec.code, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil ||
		gd.PluginClient() == nil {

		httpJSONNilGarden(w)
		return
	}

	if req.Timeout > 0 {
		var cancel goctx.CancelFunc
		ctx, cancel = goctx.WithTimeout(ctx, time.Duration(req.Timeout)*time.Second)
		defer cancel()
	}

	pc := gd.PluginClient()
	supports, err := pc.GetImageSupport(ctx)
	if err != nil {
		ec := errCodeV1(r.Method, _Image, internalError, 32)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
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
		ec := errCodeV1(r.Method, _Image, objectNotExist, 33)
		httpJSONError(w, fmt.Errorf("%s unsupported yet", req.Version()), ec.code, http.StatusInternalServerError)
		return
	}

	// database.Image.ID
	id, taskID, err := resource.LoadImage(goctx.Background(), gd.Ormer(), req)
	if err != nil {
		ec := errCodeV1(r.Method, _Image, internalError, 34)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q,%q:%q}", "id", id, "task_id", taskID)
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
		ec := errCodeV1(r.Method, _Image, dbTxError, 41)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
		ec := errCodeV1(r.Method, _Cluster, dbQueryError, 11)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	n, err := orm.CountNodeByCluster(c.ID)
	if err != nil {
		ec := errCodeV1(r.Method, _Cluster, dbQueryError, 12)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
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
		ec := errCodeV1(r.Method, _Cluster, dbQueryError, 21)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	out := make([]structs.GetClusterResponse, len(list))
	for i := range list {
		n, err := orm.CountNodeByCluster(list[i].ID)
		if err != nil {
			ec := errCodeV1(r.Method, _Cluster, dbQueryError, 22)
			httpJSONError(w, err, ec.code, http.StatusInternalServerError)
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

func postCluster(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.PostClusterRequest{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		ec := errCodeV1(r.Method, _Cluster, decodeError, 31)
		httpJSONError(w, err, ec.code, http.StatusBadRequest)
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
		ec := errCodeV1(r.Method, _Cluster, dbExecError, 32)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "id", c.ID)
}

func putClusterParams(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	req := structs.PostClusterRequest{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		ec := errCodeV1(r.Method, _Cluster, decodeError, 41)
		httpJSONError(w, err, ec.code, http.StatusBadRequest)
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
		ec := errCodeV1(r.Method, _Cluster, dbExecError, 42)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
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

	master := resource.NewHostManager(gd.Ormer(), gd.Cluster)
	err := master.RemoveCluster(name)
	if err != nil {
		ec := errCodeV1(r.Method, _Cluster, dbExecError, 51)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
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
		ec := errCodeV1(r.Method, _Host, dbQueryError, 11)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	e := gd.Cluster.Engine(n.EngineID)

	info := getNodeInfo(gd, n, e)

	writeJSON(w, info, http.StatusOK)
}

func getAllNodes(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		ec := errCodeV1(r.Method, _Host, urlParamError, 21)
		httpJSONError(w, err, ec.code, http.StatusBadRequest)
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
		ec := errCodeV1(r.Method, _Host, dbQueryError, 22)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
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

func postNodes(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	list := structs.PostNodesRequest{}
	err := json.NewDecoder(r.Body).Decode(&list)
	if err != nil {
		ec := errCodeV1(r.Method, _Host, decodeError, 31)
		httpJSONError(w, err, ec.code, http.StatusBadRequest)
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
	clusters, err := orm.ListClusters()
	if n := len(clusters); err != nil || n == 0 {
		if n == 0 {
			err = errors.New("clusters is nil")
		}
		ec := errCodeV1(r.Method, _Host, dbQueryError, 32)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	for i := range list {
		if list[i].Cluster == "" {
			ec := errCodeV1(r.Method, _Host, bodyParamsError, 33)
			httpJSONError(w, fmt.Errorf("host:%s ClusterID is required", list[i].Addr), ec.code, http.StatusInternalServerError)
			return
		}

		exist := false
		for c := range clusters {
			if clusters[c].ID == list[i].Cluster {
				exist = true
				break
			}
		}
		if !exist {
			ec := errCodeV1(r.Method, _Host, bodyParamsError, 34)
			httpJSONError(w, fmt.Errorf("host:%s unknown ClusterID:%s", list[i].Addr, list[i].Cluster), ec.code, http.StatusInternalServerError)
			return
		}

		if list[i].Storage != "" {
			_, _, err = orm.GetStorageByID(list[i].Storage)
		}
		if err != nil {
			ec := errCodeV1(r.Method, _Host, dbQueryError, 35)
			httpJSONError(w, err, ec.code, http.StatusInternalServerError)
			return
		}
	}

	nodes := resource.NewNodeWithTaskList(len(list))
	for i, n := range list {
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

		nodes[i] = resource.NewNodeWithTask(node, n.HDD, n.SSD, n.SSHConfig)
	}

	horus, err := gd.KVClient().GetHorusAddr()
	if err != nil {
		ec := errCodeV1(r.Method, _Host, internalError, 36)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	master := resource.NewHostManager(orm, gd.Cluster)
	err = master.InstallNodes(ctx, horus, nodes, gd.KVClient())
	if err != nil {
		ec := errCodeV1(r.Method, _Host, internalError, 37)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	out := make([]structs.PostNodeResponse, len(list))

	for i := range nodes {
		out[i] = structs.PostNodeResponse{
			ID:   nodes[i].Node.ID,
			Addr: nodes[i].Node.Addr,
			Task: nodes[i].Task.ID,
		}
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
		ec := errCodeV1(r.Method, _Host, dbExecError, 41)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
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
		ec := errCodeV1(r.Method, _Host, dbExecError, 51)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
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
		ec := errCodeV1(r.Method, _Host, decodeError, 61)
		httpJSONError(w, err, ec.code, http.StatusBadRequest)
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
		ec := errCodeV1(r.Method, _Host, dbExecError, 62)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
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
		ec := errCodeV1(r.Method, _Host, urlParamError, 71)
		httpJSONError(w, err, ec.code, http.StatusBadRequest)
		return
	}

	node := mux.Vars(r)["node"]
	force := boolValue(r, "force")
	username := r.FormValue("username")
	password := r.FormValue("password")

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil ||
		gd.KVClient() == nil {

		httpJSONNilGarden(w)
		return
	}

	horus, err := gd.KVClient().GetHorusAddr()
	if err != nil {
		ec := errCodeV1(r.Method, _Host, internalError, 72)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	m := resource.NewHostManager(gd.Ormer(), gd.Cluster)

	err = m.RemoveNode(ctx, horus, node, username, password, force, gd.KVClient())
	if err != nil {
		ec := errCodeV1(r.Method, _Host, internalError, 73)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// -----------------/networkings handlers-----------------
func postNetworking(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	var req structs.PostNetworkingRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		ec := errCodeV1(r.Method, _Networking, decodeError, 11)
		httpJSONError(w, err, ec.code, http.StatusBadRequest)
		return
	}

	errs := make([]string, 0, 4)
	if req.Prefix < 0 || req.Prefix > 32 {
		errs = append(errs, fmt.Sprintf("illegal Prefix:%d not in 1~32", req.Prefix))
	}

	if ip := net.ParseIP(req.Start); ip == nil {
		errs = append(errs, fmt.Sprintf("illegal IP:'%s' error", req.Start))
	}

	if ip := net.ParseIP(req.End); ip == nil {
		errs = append(errs, fmt.Sprintf("illegal IP:'%s' error", req.End))
	}
	if ip := net.ParseIP(req.Gateway); ip == nil {
		errs = append(errs, fmt.Sprintf("illegal Gateway:'%s' error", req.Gateway))
	}

	if len(errs) > 0 {
		ec := errCodeV1(r.Method, _Networking, bodyParamsError, 12)
		httpJSONError(w, errors.Errorf("%s", strings.Join(errs, ";")), ec.code, http.StatusBadRequest)
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
		ec := errCodeV1(r.Method, _Networking, internalError, 13)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%d}", "num", n)
}

func putNetworkingEnable(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ec := errCodeV1(r.Method, _Networking, urlParamError, 21)
		httpJSONError(w, err, ec.code, http.StatusBadRequest)
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
			ec := errCodeV1(r.Method, _Networking, decodeError, 22)
			httpJSONError(w, err, ec.code, http.StatusBadRequest)
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
			ec := errCodeV1(r.Method, _Networking, dbQueryError, 23)
			httpJSONError(w, err, ec.code, http.StatusInternalServerError)
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
				ec := errCodeV1(r.Method, _Networking, internalError, 24)
				httpJSONError(w, fmt.Errorf("IP %s is not in networking %s", body[i], name), ec.code, http.StatusInternalServerError)
				return
			}
		}
	}

	if len(filters) == 0 {
		err = orm.SetNetworkingEnable(name, true)
	} else {
		err = orm.SetIPEnable(filters, name, true)
	}
	if err != nil {
		ec := errCodeV1(r.Method, _Networking, dbTxError, 25)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func putNetworkingDisable(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ec := errCodeV1(r.Method, _Networking, urlParamError, 31)
		httpJSONError(w, err, ec.code, http.StatusBadRequest)
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
			ec := errCodeV1(r.Method, _Networking, decodeError, 32)
			httpJSONError(w, err, ec.code, http.StatusBadRequest)
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

	if !all && len(body) > 0 {
		list, err := orm.ListIPByNetworking(name)
		if err != nil {
			ec := errCodeV1(r.Method, _Networking, dbQueryError, 33)
			httpJSONError(w, err, ec.code, http.StatusInternalServerError)
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
				ec := errCodeV1(r.Method, _Networking, internalError, 34)
				httpJSONError(w, fmt.Errorf("IP %s is not in networking %s", body[i], name), ec.code, http.StatusInternalServerError)
				return
			}
		}
	}

	if len(filters) == 0 {
		err = orm.SetNetworkingEnable(name, false)
	} else {
		err = orm.SetIPEnable(filters, name, false)
	}
	if err != nil {
		ec := errCodeV1(r.Method, _Networking, dbTxError, 35)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
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
		ec := errCodeV1(r.Method, _Networking, internalError, 41)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
		ec := errCodeV1(r.Method, _Service, dbQueryError, 11)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(services)
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
		ec := errCodeV1(r.Method, _Service, dbQueryError, 21)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(spec)
}

func postService(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ec := errCodeV1(r.Method, _Service, urlParamError, 31)
		httpJSONError(w, err, ec.code, http.StatusBadRequest)
		return
	}

	timeout := intValueOrZero(r, "timeout")
	if timeout > 0 {
		ctx, _ = goctx.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	}

	spec := structs.ServiceSpec{}
	err := json.NewDecoder(r.Body).Decode(&spec)
	if err != nil {
		ec := errCodeV1(r.Method, _Service, decodeError, 32)
		httpJSONError(w, err, ec.code, http.StatusBadRequest)
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

	d := deploy.New(gd)

	out, err := d.Deploy(ctx, spec)
	if err != nil {
		ec := errCodeV1(r.Method, _Service, internalError, 33)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	writeJSON(w, out, http.StatusCreated)
}

func postServiceScaled(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	scale := structs.ServiceScaleRequest{}
	err := json.NewDecoder(r.Body).Decode(&scale)
	if err != nil {
		ec := errCodeV1(r.Method, _Service, decodeError, 41)
		httpJSONError(w, err, ec.code, http.StatusBadRequest)
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

	d := deploy.New(gd)

	id, err := d.ServiceScale(ctx, name, scale)
	if err != nil {
		ec := errCodeV1(r.Method, _Service, internalError, 42)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "task_id", id)
}

func postServiceLink(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	links := structs.ServicesLink{}
	err := json.NewDecoder(r.Body).Decode(&links)
	if err != nil {
		ec := errCodeV1(r.Method, _Service, decodeError, 51)
		httpJSONError(w, err, ec.code, http.StatusBadRequest)
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

	d := deploy.New(gd)

	// task ID
	id, err := d.Link(ctx, links)
	if err != nil {
		ec := errCodeV1(r.Method, _Service, internalError, 52)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "task_id", id)
}

func postServiceVersionUpdate(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ec := errCodeV1(r.Method, _Service, decodeError, 61)
		httpJSONError(w, err, ec.code, http.StatusBadRequest)
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

	d := deploy.New(gd)

	id, err := d.ServiceUpdateImage(ctx, name, version, true)
	if err != nil {
		ec := errCodeV1(r.Method, _Service, internalError, 62)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "task_id", id)
}

func postServiceUpdate(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	var update structs.UnitRequire

	err := json.NewDecoder(r.Body).Decode(&update)
	if err != nil {
		ec := errCodeV1(r.Method, _Service, decodeError, 71)
		httpJSONError(w, err, ec.code, http.StatusBadRequest)
		return
	}

	if update.Require.CPU == 0 && update.Require.Memory == 0 && len(update.Volumes) == 0 {
		ec := errCodeV1(r.Method, _Service, bodyParamsError, 72)
		httpJSONError(w, fmt.Errorf("no updateConfig required"), ec.code, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil || gd.Cluster == nil {

		httpJSONNilGarden(w)
		return
	}

	d := deploy.New(gd)

	id, err := d.ServiceUpdate(ctx, name, update)
	if err != nil {
		ec := errCodeV1(r.Method, _Service, internalError, 73)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "task_id", id)

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
		ec := errCodeV1(r.Method, _Service, dbQueryError, 81)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	spec, err := svc.Spec()
	if err != nil {
		ec := errCodeV1(r.Method, _Service, internalError, 82)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	task := database.NewTask(spec.Name, database.ServiceStartTask, spec.ID, spec.Desc, nil, 300)

	err = svc.InitStart(ctx, gd.KVClient(), nil, &task, true, nil)
	if err != nil {
		ec := errCodeV1(r.Method, _Service, internalError, 83)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "task_id", task.ID)
}

func postServiceUpdateConfigs(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	var req = struct {
		Configs structs.ConfigsMap
		Args    map[string]interface{}
	}{}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		ec := errCodeV1(r.Method, _Service, decodeError, 91)
		httpJSONError(w, err, ec.code, http.StatusBadRequest)
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
		ec := errCodeV1(r.Method, _Service, dbQueryError, 92)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	spec, err := svc.Spec()
	if err != nil {
		ec := errCodeV1(r.Method, _Service, internalError, 93)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	task := database.NewTask(spec.Name, database.ServiceUpdateConfigTask, spec.ID, spec.Desc, nil, 300)

	err = svc.UpdateUnitsConfigs(ctx, req.Configs, req.Args, &task, true)
	if err != nil {
		ec := errCodeV1(r.Method, _Service, internalError, 94)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "task_id", task.ID)
}

func postServiceExec(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	config := structs.ServiceExecConfig{}
	err := json.NewDecoder(r.Body).Decode(&config)
	if err != nil {
		ec := errCodeV1(r.Method, _Service, decodeError, 101)
		httpJSONError(w, err, ec.code, http.StatusBadRequest)
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
		ec := errCodeV1(r.Method, _Service, dbQueryError, 102)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	spec, err := svc.Spec()
	if err != nil {
		ec := errCodeV1(r.Method, _Service, internalError, 103)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	task := database.NewTask(spec.Name, database.ServiceStopTask, spec.ID, spec.Desc, nil, 300)

	err = svc.Exec(ctx, config, true, &task)
	if err != nil {
		ec := errCodeV1(r.Method, _Service, internalError, 104)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "task_id", task.ID)
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
		ec := errCodeV1(r.Method, _Service, dbQueryError, 111)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	spec, err := svc.Spec()
	if err != nil {
		ec := errCodeV1(r.Method, _Service, internalError, 112)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	task := database.NewTask(spec.Name, database.ServiceStopTask, spec.ID, spec.Desc, nil, 300)

	err = svc.Stop(ctx, false, true, &task)
	if err != nil {
		ec := errCodeV1(r.Method, _Service, internalError, 113)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "task_id", task.ID)
}

func deleteService(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

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
		ec := errCodeV1(r.Method, _Service, dbQueryError, 121)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	err = svc.Remove(ctx, gd.KVClient())
	if err != nil {
		ec := errCodeV1(r.Method, _Service, internalError, 122)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// -----------------/storage handlers-----------------
// GET /storage/san
func getSANStoragesInfo(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	ds := storage.DefaultStores()
	stores, err := ds.List()
	if err != nil {
		ec := errCodeV1(r.Method, _Storage, dbQueryError, 11)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	resp := make([]structs.SANStorageResponse, len(stores))
	for i := range stores {
		resp[i], err = getSanStoreInfo(stores[i])
		if err != nil {
			ec := errCodeV1(r.Method, _Storage, internalError, 12)
			httpJSONError(w, err, ec.code, http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// GET /storage/san/{name:.*}
func getSANStorageInfo(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ds := storage.DefaultStores()
	store, err := ds.Get(name)
	if err != nil {
		ec := errCodeV1(r.Method, _Storage, dbQueryError, 21)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	resp, err := getSanStoreInfo(store)
	if err != nil {
		ec := errCodeV1(r.Method, _Storage, internalError, 22)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
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

// POST /storage/san
func postSanStorage(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.PostSANStoreRequest{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ec := errCodeV1(r.Method, _Storage, decodeError, 31)
		httpJSONError(w, err, ec.code, http.StatusBadRequest)
		return
	}

	ds := storage.DefaultStores()
	s, err := ds.Add(req.Vendor, req.Addr,
		req.Username, req.Password, req.Admin,
		req.LunStart, req.LunEnd, req.HostLunStart, req.HostLunEnd)
	if err != nil {
		ec := errCodeV1(r.Method, _Storage, internalError, 32)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "ID", s.ID())
}

// POST /storage/san/{name}/raidgroup
func postRGToSanStorage(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	san := mux.Vars(r)["name"]
	rg := mux.Vars(r)["rg"]

	ds := storage.DefaultStores()
	store, err := ds.Get(san)
	if err != nil {
		ec := errCodeV1(r.Method, _Storage, dbQueryError, 41)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	space, err := store.AddSpace(rg)
	if err != nil {
		ec := errCodeV1(r.Method, _Storage, internalError, 42)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%d}", "Size", space.Total)
}

// POST /storage/san/{name}/raid_group/{rg:[0-9]+}/enable
func postEnableRaidGroup(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	san := mux.Vars(r)["name"]
	rg := mux.Vars(r)["rg"]

	ds := storage.DefaultStores()
	store, err := ds.Get(san)
	if err != nil {
		ec := errCodeV1(r.Method, _Storage, dbQueryError, 51)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	err = store.EnableSpace(rg)
	if err != nil {
		ec := errCodeV1(r.Method, _Storage, dbExecError, 52)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// POST /storage/san/{name}/raid_group/{rg:[0-9]+}/disable
func postDisableRaidGroup(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	san := mux.Vars(r)["name"]
	rg := mux.Vars(r)["rg"]

	ds := storage.DefaultStores()
	store, err := ds.Get(san)
	if err != nil {
		ec := errCodeV1(r.Method, _Storage, dbQueryError, 61)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	err = store.DisableSpace(rg)
	if err != nil {
		ec := errCodeV1(r.Method, _Storage, dbExecError, 62)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
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
		ec := errCodeV1(r.Method, _Storage, internalError, 71)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
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
		ec := errCodeV1(r.Method, _Storage, internalError, 81)
		httpJSONError(w, err, ec.code, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
