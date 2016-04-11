package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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

// POST /cluster
func postCluster(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	var (
		req    = structs.PostClusterRequest{}
		stores = make([]store.Store, 0, 2)
	)

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, errUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	if req.StorageType != "local" && req.StorageID != "" {
		store, err := gd.GetStore(req.StorageID)
		if err == nil && store != nil {
			stores = append(stores, store)
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

	err := gd.AddDatacenter(cluster, stores)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "Id", cluster.ID)
}

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

}

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

// Post /task/backup/callback
func postBackupCallback(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.BackupTaskCallback{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	task := &database.Task{ID: req.TaskID}

	if req.Error() != nil {
		err := database.UpdateTaskStatus(task, structs.TaskFailed, time.Now(), req.Error().Error())
		if err != nil {
			httpError(w, err.Error(), http.StatusInternalServerError)
		}

		return
	}

	rent, err := database.BackupTaskValidate(req.TaskID, req.StrategyID, req.UnitID)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	backupFile := database.BackupFile{
		ID:         utils.Generate64UUID(),
		TaskID:     req.TaskID,
		StrategyID: req.StrategyID,
		UnitID:     req.UnitID,
		Type:       req.Type,
		Path:       req.Path,
		SizeByte:   req.Size,
		Status:     req.Status,
		CreatedAt:  time.Now(),
	}

	if rent > 0 {
		backupFile.Retention = backupFile.CreatedAt.Add(time.Duration(rent))
	}

	err = database.TxBackupTaskDone(task, structs.TaskDone, backupFile)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
