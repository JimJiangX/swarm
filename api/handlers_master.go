package api

import (
	"encoding/json"
	stderr "errors"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/utils"
	"github.com/gorilla/mux"
	goctx "golang.org/x/net/context"
)

var errUnsupportGarden = stderr.New("unsupport Garden yet")

// Emit an HTTP error and log it.
func httpError2(w http.ResponseWriter, err error, status int) {
	if err != nil {
		json.NewEncoder(w).Encode(structs.ResponseHead{
			Result:  false,
			Code:    status,
			Message: err.Error(),
		})
	}

	w.WriteHeader(status)

	logrus.WithField("status", status).Errorf("HTTP error: %+v", err)
}

func httpSucceed(w http.ResponseWriter, obj interface{}, status int) {
	resp := structs.CommandResponse{
		ResponseHead: structs.ResponseHead{
			Result: true,
			Code:   status,
		},
		Object: obj,
	}

	err := json.NewEncoder(w).Encode(resp)
	if err != nil {
		logrus.WithField("status", status).Errorf("JSON Encode error: %+v", err)
	}

	w.WriteHeader(status)
}

func postRegisterDC(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.RegisterDC{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		httpError2(w, err, http.StatusBadRequest)
		return
	}
	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil {
		httpError2(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	err = gd.Register(req)
	if err != nil {
		httpError2(w, err, http.StatusInternalServerError)
		return
	}

	httpSucceed(w, nil, http.StatusCreated)
}

func postImageLoad(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.PostLoadImageRequest{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		httpError2(w, err, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil ||
		gd.PluginClient() == nil {

		httpError2(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	if req.Timeout > 0 {
		var cancel goctx.CancelFunc
		ctx, cancel = goctx.WithTimeout(ctx, time.Duration(req.Timeout)*time.Second)
		defer cancel()
	}

	id, err := resource.LoadImage(ctx, gd.Ormer(), gd.PluginClient(), req)
	if err != nil {
		httpError2(w, err, http.StatusInternalServerError)
		return
	}

	httpSucceed(w, id, http.StatusCreated)
}

func postCluster(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.PostClusterRequest{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		httpError2(w, err, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpError2(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	c := database.Cluster{
		ID:         utils.Generate32UUID(),
		Name:       req.Name,
		Type:       req.Type,
		Enabled:    true,
		MaxNode:    req.MaxNode,
		UsageLimit: req.UsageLimit,
	}

	err = gd.Ormer().InsertCluster(c)
	if err != nil {
		httpError2(w, err, http.StatusInternalServerError)
		return
	}

	httpSucceed(w, c, http.StatusCreated)
}

func postNodes(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	list := structs.PostNodesRequest{}
	err := json.NewDecoder(r.Body).Decode(&list)
	if err != nil {
		httpError2(w, err, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil ||
		gd.KVClient() == nil {

		httpError2(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	ormer := gd.Ormer()
	c, err := ormer.GetCluster(name)
	if err != nil {
		httpError2(w, err, http.StatusInternalServerError)
		return
	}
	horus, err := gd.KVClient().GetHorusAddr()
	if err != nil {
		httpError2(w, err, http.StatusInternalServerError)
		return
	}

	nodes := resource.NewNodeWithTaskList(len(list))
	for i, n := range list {
		nodes[i], err = resource.NewNodeWithTask(n.Username, n.Password,
			n.HDD, n.SSD,
			database.Node{
				ID:           utils.Generate32UUID(),
				Name:         n.Name,
				ClusterID:    c.ID,
				Addr:         n.Address,
				EngineID:     "",
				Room:         n.Room,
				Seat:         n.Seat,
				MaxContainer: n.MaxContainer,
				Status:       0,
			})
		if err != nil {
			httpError2(w, err, http.StatusInternalServerError)
			return
		}
	}

	master := resource.NewNodes(ormer, gd.Cluster)
	err = master.InstallNodes(ctx, horus, nodes)
	if err != nil {
		httpError2(w, err, http.StatusInternalServerError)
		return
	}

	response := make([]structs.PostNodeResponse, len(list))

	for i := range nodes {
		response[i] = structs.PostNodeResponse{
			ID:     nodes[i].Node.ID,
			Name:   nodes[i].Node.Name,
			TaskID: nodes[i].Task.ID,
		}
	}

	httpSucceed(w, response, http.StatusCreated)
}
