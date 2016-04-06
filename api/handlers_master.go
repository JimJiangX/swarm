package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	goctx "golang.org/x/net/context"

	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/store"
	"github.com/docker/swarm/utils"
)

var UnsupportGardener = errors.New("Unsupported Gardener")

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
		httpError(w, UnsupportGardener.Error(), http.StatusInternalServerError)
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
