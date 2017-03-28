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
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/deploy"
	"github.com/docker/swarm/garden/resource"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/utils"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	goctx "golang.org/x/net/context"
)

var errUnsupportGarden = stderr.New("unsupport Garden yet")

// Emit an HTTP error and log it.
func httpJSONError(w http.ResponseWriter, err error, status int) {
	field := logrus.WithField("status", status)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err != nil {
		json.NewEncoder(w).Encode(structs.ResponseHead{
			Result:  false,
			Code:    status,
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
		json.NewEncoder(w).Encode(obj)
	}
}

func proxySpecialLogic(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	name := mux.Vars(r)["name"]
	orm := gd.Ormer()

	u, err := orm.GetUnit(name)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	ips, err := orm.ListIPByUnitID(u.ID)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	if len(ips) == 0 {
		httpJSONError(w, errors.Errorf("not found Unit %s address", u.Name), http.StatusInternalServerError)
		return
	}

	addr := utils.Uint32ToIP(ips[0].IPAddr).String()

	// TODO:get service port

	index := strings.Index(r.URL.Path, "/proxy/")
	r.URL.Path = r.URL.Path[index+len("/proxy"):]

	err = hijack(nil, addr, w, r)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}
}

// -----------------/nfs_backups handlers-----------------
func getNFSSPace(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpJSONError(w, err, http.StatusBadRequest)
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

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	sys, err := gd.Ormer().GetSysConfig()
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	abs, err := utils.GetAbsolutePath(true, sys.SourceDir)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	d := resource.NewNFSDriver(nfs, filepath.Dir(abs), sys.BackupDir)

	space, err := d.Space()
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
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

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	name := mux.Vars(r)["name"]

	t, err := gd.Ormer().GetTask(name)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	writeJSON(w, t, http.StatusOK)
}

func getTasks(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpJSONError(w, err, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
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
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	writeJSON(w, out, http.StatusOK)
}

// -----------------/datacenter handler-----------------
func postRegisterDC(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.RegisterDC{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		httpJSONError(w, err, http.StatusBadRequest)
		return
	}
	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil {
		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	err = gd.Register(req)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	writeJSON(w, nil, http.StatusCreated)
}

// -----------------/softwares/images handlers-----------------
func listImages(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	images, err := gd.Ormer().ListImages()
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
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

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	pc := gd.PluginClient()
	out, err := pc.GetImageSupport(ctx)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	writeJSON(w, out, http.StatusOK)
}

func postImageLoad(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.PostLoadImageRequest{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		httpJSONError(w, err, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil ||
		gd.PluginClient() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
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
		httpJSONError(w, err, http.StatusInternalServerError)
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
		httpJSONError(w, fmt.Errorf("%s unsupported yet", req.Version()), http.StatusInternalServerError)
		return
	}

	// database.Image.ID
	id, taskID, err := resource.LoadImage(ctx, gd.Ormer(), req)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
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

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	err := gd.Ormer().DelImage(img)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// -----------------/clusters handlers-----------------
func getClustersByID(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil || gd.Ormer() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	orm := gd.Ormer()
	c, err := orm.GetCluster(name)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	n, err := orm.CountNodeByCluster(c.ID)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
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

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	orm := gd.Ormer()

	list, err := orm.ListClusters()
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	out := make([]structs.GetClusterResponse, len(list))
	for i := range list {
		n, err := orm.CountNodeByCluster(list[i].ID)
		if err != nil {
			httpJSONError(w, err, http.StatusInternalServerError)
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
		httpJSONError(w, err, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	c := database.Cluster{
		ID:         utils.Generate32UUID(),
		MaxNode:    req.Max,
		UsageLimit: req.UsageLimit,
	}

	err = gd.Ormer().InsertCluster(c)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
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
		httpJSONError(w, err, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	c := database.Cluster{
		ID:         name,
		MaxNode:    req.Max,
		UsageLimit: req.UsageLimit,
	}

	err = gd.Ormer().SetClusterParams(c)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func deleteCluster(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil || gd.Cluster == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	master := resource.NewMaster(gd.Ormer(), gd.Cluster)
	err := master.RemoveCluster(name)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// -----------------/hosts handlers-----------------
func getNodeInfo(n database.Node, e *cluster.Engine) structs.NodeInfo {
	info := structs.NodeInfo{
		ID:           n.ID,
		Cluster:      n.ClusterID,
		Room:         n.Room,
		Seat:         n.Room,
		MaxContainer: n.MaxContainer,
		Enabled:      n.Enabled,
		RegisterAt:   utils.TimeToString(n.RegisterAt),
	}

	info.SetByEngine(e)

	if info.Engine.IP == "" {
		info.Engine.IP = n.Addr
	}

	return info
}

func getNode(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil || gd.Cluster == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	n, err := gd.Ormer().GetNode(name)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	e := gd.Cluster.Engine(n.EngineID)

	info := getNodeInfo(n, e)

	writeJSON(w, info, http.StatusOK)
}

func getAllNodes(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil || gd.Cluster == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	engines := gd.Cluster.ListEngines()

	nodes, err := gd.Ormer().ListNodes()
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
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

		out = append(out, getNodeInfo(nodes[i], engine))
	}

	writeJSON(w, out, http.StatusOK)
}

func postNodes(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	list := structs.PostNodesRequest{}
	err := json.NewDecoder(r.Body).Decode(&list)
	if err != nil {
		httpJSONError(w, err, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil ||
		gd.KVClient() == nil {
		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	orm := gd.Ormer()
	clusters, err := orm.ListClusters()
	if n := len(clusters); err != nil || n == 0 {
		if n == 0 {
			err = errors.New("clusters is nil")
		}
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	for i := range list {
		if list[i].Cluster == "" {
			httpJSONError(w, fmt.Errorf("host:%s ClusterID is required", list[i].Addr), http.StatusInternalServerError)
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
			httpJSONError(w, fmt.Errorf("host:%s unknown ClusterID:%s", list[i].Addr, list[i].Cluster), http.StatusInternalServerError)
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
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	master := resource.NewMaster(orm, gd.Cluster)
	err = master.InstallNodes(ctx, horus, nodes, gd.KVClient())
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
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

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	err := gd.Ormer().SetNodeEnable(name, true)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func putNodeDisable(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	err := gd.Ormer().SetNodeEnable(name, false)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
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
		httpJSONError(w, err, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	err = gd.Ormer().SetNodeParam(name, max.N)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
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
		httpJSONError(w, err, http.StatusBadRequest)
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

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	horus, err := gd.KVClient().GetHorusAddr()
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	m := resource.NewMaster(gd.Ormer(), gd.Cluster)

	err = m.RemoveNode(ctx, horus, node, username, password, force, gd.KVClient())
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
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
		httpJSONError(w, err, http.StatusBadRequest)
		return
	}

	if req.Prefix < 0 || req.Prefix > 32 {
		httpJSONError(w, fmt.Errorf("illegal Prefix:%d not in 1~32", req.Prefix), http.StatusBadRequest)
		return
	}

	if ip := net.ParseIP(req.Start); ip == nil {
		httpJSONError(w, fmt.Errorf("illegal IP:'%s' error", req.Start), http.StatusBadRequest)
		return
	}
	if ip := net.ParseIP(req.End); ip == nil {
		httpJSONError(w, fmt.Errorf("illegal IP:'%s' error", req.End), http.StatusBadRequest)
		return
	}
	if ip := net.ParseIP(req.Gateway); ip == nil {
		httpJSONError(w, fmt.Errorf("illegal Gateway:'%s' error", req.Gateway), http.StatusBadRequest)
		return
	}
	if name == "" && req.Networking != "" {
		name = req.Networking
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	nw := resource.NewNetworks(gd.Ormer())
	n, err := nw.AddNetworking(req.Start, req.End, req.Gateway, name, req.VLAN, req.Prefix)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%d}", "num", n)
}

func putNetworkingEnable(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpJSONError(w, err, http.StatusBadRequest)
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
			logrus.Warnf("JSON Decode: %s", err)
		}
	}
	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	orm := gd.Ormer()
	filters := make([]uint32, 0, len(body))

	if !all && len(body) > 0 {
		list, err := orm.ListIPByNetworking(name)
		if err != nil {
			httpJSONError(w, err, http.StatusInternalServerError)
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
				httpJSONError(w, fmt.Errorf("IP %s is not in networking %s", body[i], name), http.StatusInternalServerError)
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
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func putNetworkingDisable(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpJSONError(w, err, http.StatusBadRequest)
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
			logrus.Warnf("JSON Decode: %s", err)
		}
	}
	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	orm := gd.Ormer()
	filters := make([]uint32, 0, len(body))

	if !all && len(body) > 0 {
		list, err := orm.ListIPByNetworking(name)
		if err != nil {
			httpJSONError(w, err, http.StatusInternalServerError)
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
				httpJSONError(w, fmt.Errorf("IP %s is not in networking %s", body[i], name), http.StatusInternalServerError)
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
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func deleteNetworking(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil || gd.Ormer() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	err := gd.Ormer().DelNetworking(name)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// -----------------/services handlers-----------------
func postService(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpJSONError(w, err, http.StatusBadRequest)
		return
	}

	timeout := intValueOrZero(r, "timeout")
	if timeout > 0 {
		ctx, _ = goctx.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	}

	spec := structs.ServiceSpec{}
	err := json.NewDecoder(r.Body).Decode(&spec)
	if err != nil {
		httpJSONError(w, err, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil ||
		gd.KVClient() == nil ||
		gd.PluginClient() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	d := deploy.New(gd)

	out, err := d.Deploy(ctx, spec)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	writeJSON(w, out, http.StatusCreated)
}

func postServiceScaled(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	arch := structs.Arch{}
	err := json.NewDecoder(r.Body).Decode(&arch)
	if err != nil {
		httpJSONError(w, err, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil ||
		gd.KVClient() == nil ||
		gd.PluginClient() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	d := deploy.New(gd)

	id, err := d.ServiceScale(ctx, name, arch)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
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
		httpJSONError(w, err, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil ||
		gd.KVClient() == nil ||
		gd.PluginClient() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	d := deploy.New(gd)

	// task ID
	id, err := d.Link(ctx, links)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "task_id", id)
}

func postServiceVersionUpdate(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpJSONError(w, err, http.StatusBadRequest)
		return
	}

	name := mux.Vars(r)["name"]
	version := r.FormValue("image")

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil || gd.PluginClient() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	d := deploy.New(gd)

	id, err := d.ServiceUpdateImage(ctx, name, version)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
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
		httpJSONError(w, err, http.StatusBadRequest)
		return
	}

	if update.Require.CPU == 0 && update.Require.Memory == 0 && len(update.Volumes) == 0 {
		httpJSONError(w, fmt.Errorf("no updateConfig required"), http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil || gd.Cluster == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	d := deploy.New(gd)

	id, err := d.ServiceUpdate(ctx, name, update)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "task_id", id)

}

func putServiceStart(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil || gd.KVClient() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	svc, err := gd.GetService(name)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	err = svc.InitStart(ctx, gd.KVClient(), nil, nil)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func putServiceUpdateConfigs(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	var req = struct {
		Configs structs.ConfigsMap
		Args    map[string]interface{}
	}{}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		httpJSONError(w, err, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	svc, err := gd.GetService(name)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	err = svc.UpdateUnitsConfigs(ctx, req.Configs, req.Args)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func putServiceStop(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	svc, err := gd.GetService(name)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	err = svc.Stop(ctx, false)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func deleteService(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil || gd.KVClient() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	svc, err := gd.GetService(name)
	if err != nil {
		if errors.Cause(err) == sql.ErrNoRows {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	err = svc.Remove(ctx, gd.KVClient())
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
