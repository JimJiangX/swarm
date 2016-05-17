package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster/swarm"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/store"
	"github.com/docker/swarm/utils"
	"github.com/gorilla/mux"
	goctx "golang.org/x/net/context"
)

const (
	StatusUnprocessableEntity = 422
)

var ErrUnsupportGardener = errors.New("Unsupported Gardener")

// GET /clusters/{name:.*}
func getClustersByNameOrID(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	_ = name

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}
}

// GET /clusters
func getClusters(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}
}

// GET /clusters/{name:.*}/nodes
func getNodes(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	_ = name

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}
}

// GET /clusters/{name:.*}/nodes/{node:.*}
func getNodesByNameOrID(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	cluster := mux.Vars(r)["name"]
	node := mux.Vars(r)["node"]
	_, _ = cluster, node

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}
}

// GET /tasks
func getTasks(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {}

// GET /tasks/{name:.*}
func getTask(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {}

// POST /clusters
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

	if warnings := swarm.ValidDatacenter(req); warnings != "" {
		httpError(w, warnings, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	if req.StorageType != "local" && req.StorageID != "" {
		store, err = gd.GetStore(req.StorageID)
		if err != nil {
			httpError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	cluster, err := swarm.AddNewCluster(req)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
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

// POST /clusters/{name:.*}/enable
func postEnableCluster(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	dc, err := gd.Datacenter(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = dc.SetStatus(true)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// POST /clusters/{name:.*}/disable
func postDisableCluster(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	dc, err := gd.Datacenter(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = dc.SetStatus(false)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// POST /clusters/{name:.*}/nodes
func postNodes(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	dc, err := gd.Datacenter(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
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
			list[i].Username, list[i].Password, list[i].HDD, list[i].SSD,
			list[i].Port, list[i].MaxContainer)

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

	for i := range nodes {
		go dc.DistributeNode(nodes[i], gd.KVPath())
	}

	min := 600
	if len(nodes) > 5 {
		min = len(nodes) * 120
	}
	go gd.RegisterNodes(name, nodes, time.Second*time.Duration(min))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

//POST /clusters/nodes/{node:.*}/enable
func postEnableNode(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["node"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err := gd.SetNodeStatus(name, 6)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

//POST /clusters/nodes/{node:.*}/disable
func postDisableNode(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["node"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err := gd.SetNodeStatus(name, 7)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// POST /services
func postService(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.PostServiceRequest{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if warnings := swarm.ValidService(req); len(warnings) > 0 {
		httpError(w, strings.Join(warnings, ";"), http.StatusConflict)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	svc, err := gd.CreateService(req)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	strategy := ""
	if backup := svc.BackupStrategy(); backup != nil {
		strategy = backup.ID
	}

	response := structs.PostServiceResponse{
		ID:               svc.ID,
		BackupStrategyID: strategy,
		TaskID:           svc.Task().ID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// POST /services/{name:.*}/start
func postServiceStart(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}
	_ = name
}

// POST /services/{name:.*}/stop
func postServiceStop(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}
	_ = name
}

// POST /services/{name:.*}/backup
func postServiceBackup(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}
	_ = name
}

// POST /services/{name:.*}/recover
func postServiceRecover(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}
	_ = name
}

// POST /services/{name:.*}/backup_strategy
func postStrategyToService(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	req := structs.BackupStrategy{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	strategy, err := gd.ReplaceServiceBackupStrategy(name, req)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "ID", strategy.ID)
}

// POST 	/services/backup_strategy/{name:.*}/update
func postUpdateServiceStrategy(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}
	_ = name
}

// POST 	/services/backup_strategy/{name:.*}/enable
func postEnableServiceStrategy(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err := gd.EnableServiceBackupStrategy(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// POST /services/backup_strategy/{name:.*}/disable
func postDisableServiceStrategy(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err := gd.DisableBackupStrategy(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// POST /units/{unit:.*}/start
func postUnitStart(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {}

// POST 	/units/{unit:.*}/stop
func postUnitStop(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {}

// POST /units/{unit:.*}/backup
func postUnitBackup(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {}

// POST /units/{unit:.*}/recover
func postUnitRecover(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {}

// POST /units/{unit:.*}/migrate
func postUnitMigrate(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {}

// POST /units/{unit:.*}/rebuild
func postUnitRebuild(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {}

// POST /units/{unit:.*}/isolate
func postUnitIsolate(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {}

// POST /units/{unit:.*}/switchback
func postUnitSwitchback(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {}

// POST /tasks/backup/callback
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

// POST /networkings
func postNetworking(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.PostNetworkingRequest{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	net, err := gd.AddNetworking(req.Start, req.End, req.Type, req.Gateway, req.Prefix)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "ID", net.ID)
}

// POST /networkings/{name:.*}/enable
func postEnableNetworking(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err := gd.SetNetworkingStatus(name, true)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// POST /networkings/{name:.*}/disable
func postDisableNetworking(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err := gd.SetNetworkingStatus(name, false)

	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// POST /networkings/ports/import
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
	fmt.Fprintf(w, "{%q:%d}", "num", num)
}

// POST /networkings/ports/{port:.*}/disable
func postDisablePort(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	port := intValueOrZero(r, "port")
	if port == 0 {
		httpError(w, "port must be between 1~65535", http.StatusBadRequest)
		return
	}

	err := database.SetPortAllocated(port, true)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// Load Image
// POST /image/load
func postImageLoad(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.PostLoadImageRequest{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
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

// POST /image/{image:.*}/enable
func postEnableImage(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	image := mux.Vars(r)["image"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err := gd.UpdateImageStatus(image, true)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// POST 	/image/{image:.*}/disable
func postDisableImage(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	image := mux.Vars(r)["image"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err := gd.UpdateImageStatus(image, false)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// POST /storage/san
func postSanStorage(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.PostSANStoreRequest{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
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

// POST /storage/san/{name:.*}/raidgroup
func postRGToSanStorage(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	name := mux.Vars(r)["name"]

	rg, err := strconv.Atoi(r.Form.Get("rg"))
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
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
	fmt.Fprintf(w, "{%q:%d}", "Size", size)
}

// POST /storage/san/{name:.*}/raid_group/{rg:.*}/enable
func postEnableRaidGroup(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	san := mux.Vars(r)["name"]

	rg, err := strconv.Atoi(r.Form.Get("rg"))
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err = gd.UpdateStoreSpaceStatus(san, rg, true)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// POST /storage/san/{name:.*}/raid_group/{rg:.*}/disable
func postDisableRaidGroup(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	san := mux.Vars(r)["name"]

	rg, err := strconv.Atoi(r.Form.Get("rg"))
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err = gd.UpdateStoreSpaceStatus(san, rg, false)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// POST /storage/nas
func postNasStorage(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
}

// DELETE /services/{name:.*}
// TODO:Not Done Yet
func deleteService(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	name := mux.Vars(r)["name"]
	force := boolValue(r, "force")
	volumes := boolValue(r, "v")
	timeout := intValueOrZero(r, "time")

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err := gd.DeleteService(name, force, volumes, timeout)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DELETE /services/backup_strategy/{name:.*}
func deleteBackupStrategy(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err := gd.DeleteServiceBackupStrategy(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DELETE /clusters/{name:.*}
func deleteCluster(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err := gd.RemoveDatacenter(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DELETE /clusters/nodes/{node:.*}
func deleteNode(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	node := mux.Vars(r)["node"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err := gd.RemoveNode(node)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DELETE /netwrokings/{name:.*}
func deleteNetworking(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err := gd.RemoveNetworking(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DELETE /storage/san/{name:.*}
func deleteStorage(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err := gd.RemoveStore(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DELETE /storage/san/{name:.*}/raid_group/{rg:.*}
func deleteRaidGroup(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	san := mux.Vars(r)["name"]

	rg, err := strconv.Atoi(r.Form.Get("rg"))
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err = gd.RemoveStoreSpace(san, rg)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DELETE /image/{image:.*}
func deleteImage(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	image := mux.Vars(r)["image"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err := gd.RemoveImage(image)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
