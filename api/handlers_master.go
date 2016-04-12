package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	goctx "golang.org/x/net/context"

	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster/swarm"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/store"
	"github.com/docker/swarm/utils"
	"github.com/gorilla/mux"
)

const (
	StatusUnprocessableEntity = 422
)

var errUnsupportGardener = errors.New("Unsupported Gardener")

// 创建集群
// POST /cluster
func postCluster(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	var (
		req   = structs.PostClusterRequest{}
		store store.Store
	)

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, errUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	if req.StorageType != "local" && req.StorageID != "" {
		store, err = gd.GetStore(req.StorageID)
		if err != nil {
			httpError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	cluster := database.Cluster{
		ID:          utils.Generate64UUID(),
		Name:        req.Name,
		Type:        req.Type,
		StorageType: req.StorageType,
		StorageID:   req.StorageID,
		Datacenter:  req.Datacenter,
		Enabled:     true,
		MaxNode:     req.MaxNode,
		UsageLimit:  req.UsageLimit,
	}

	err = gd.AddDatacenter(cluster, store)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "ID", cluster.ID)
}

// 集群物理机入库
// Post /cluster/{name:.*}/nodes
func postNodes(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, errUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	dc, err := gd.Datacenter(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	list := structs.PostNodesRequest{}

	if err := json.NewDecoder(r.Body).Decode(&list); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	nodes := make([]*swarm.Node, len(list))
	response := make([]structs.PostNodeResponse, len(list))

	for i := range list {
		nodes[i] = swarm.NewNode(list[i].Address, list[i].Name, dc.ID,
			list[i].Username, list[i].Password, list[i].Port, list[i].MaxContainer)

		response[i] = structs.PostNodeResponse{
			ID:     nodes[i].ID,
			Name:   nodes[i].Name,
			TaskID: nodes[i].Task().ID,
		}

	}

	err = swarm.SaveMultiNodesToDB(nodes)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)

	for i := range nodes {
		go dc.DistributeNode(nodes[i], gd.KVPath())
	}

	go gd.RegisterNodes(name, nodes, time.Second*600)

}

// 创建服务
// Post /service
func postService(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.PostServiceRequest{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if warnings := swarm.Validate(req); len(warnings) > 0 {
		httpError(w, strings.Join(warnings, ","), StatusUnprocessableEntity)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, errUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	svc, err := gd.CreateService(req)
	if err != nil {
		httpError(w, errUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	response := structs.PostServiceResponse{
		ID:     svc.ID,
		TaskID: svc.Task().ID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// 备份任务完成结果回调处理
// Post /task/backup/callback
func postBackupCallback(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.BackupTaskCallback{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := swarm.BackupTaskCallback(req)
	if err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// 网络规划
// Post /networking
func postNetworking(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.PostNetworkingRequest{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, errUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	net, err := gd.AddNetworking(req.IP, req.Type, req.Gateway, req.Prefix, req.Num)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "ID", net.ID)
}

// Post /networking/ports/import
func postImportPort(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.PostImportPortRequest{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	num, err := database.TxImportPort(req.Start, req.End, req.Filters...)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "num", num)
}

// Load Image
// Post /image/load
func postImageLoad(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.PostLoadImageRequest{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, errUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	id, err := gd.LoadImage(req)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "ID", id)

}

// SAN存储系统入库
// Post /storage/san
func postSanStorage(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.PostSANStoreRequest{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, errUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	id := utils.Generate64UUID()
	store, err := store.RegisterStore(id, req.Vendor, req.Addr,
		req.Username, req.Password, req.Admin,
		req.LunStart, req.LunEnd, req.HostLunStart, req.HostLunEnd)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = gd.AddStore(store)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "ID", store.ID())
}

// Post /storage/{name:.*}/raidgroup/add
func postRGToSanStorage(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	if err := r.ParseForm(); err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rg, err := strconv.Atoi(r.Form.Get("rg"))
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, errUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	store, err := gd.GetStore(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	size, err := store.AddSpace(rg)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "size", size)
}

// NAS系统登记
// Post /storage/nas
func postNasStorage(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
}
