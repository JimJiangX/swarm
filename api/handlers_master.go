package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/store"
	"github.com/docker/swarm/utils"
)

// POST /cluster
func postCluster(ctx *mcontext, w http.ResponseWriter, r *http.Request) {
	var (
		req    = structs.PostClusterRequest{}
		stores = make([]store.Store, 0, 2)
	)

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.StorageType != "local" && req.StorageID != "" {
		store := ctx.region.GetStore(req.StorageID)
		if store != nil {
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

	err := ctx.region.AddDatacenter(cluster, stores)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "Id", cluster.ID)
}
