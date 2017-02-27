package api

import (
	"encoding/json"
	stderr "errors"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource"
	"github.com/docker/swarm/garden/stack"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/utils"
	"github.com/gorilla/mux"
	goctx "golang.org/x/net/context"
)

var errUnsupportGarden = stderr.New("unsupport Garden yet")

// Emit an HTTP error and log it.
func httpError2(w http.ResponseWriter, err error, status int) {
	field := logrus.WithField("status", status)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")

		_err := json.NewEncoder(w).Encode(structs.ResponseHead{
			Result:  false,
			Code:    status,
			Message: err.Error(),
		})

		if _err != nil {
			field.Errorf("JSON Encode error: %+v", err)
		}
	}

	w.WriteHeader(status)

	field.Errorf("HTTP error: %+v", err)
}

func httpSucceed(w http.ResponseWriter, obj interface{}, status int) {
	resp := structs.CommonResponse{
		ResponseHead: structs.ResponseHead{
			Result: true,
			Code:   status,
		},
		Object: obj,
	}

	w.Header().Set("Content-Type", "application/json")

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
		Type:       req.Type,
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
				ClusterID:    c.ID,
				Addr:         n.Address,
				EngineID:     "",
				Room:         n.Room,
				Seat:         n.Seat,
				MaxContainer: n.MaxContainer,
				Status:       0,
				Enabled:      false,
			})
		if err != nil {
			httpError2(w, err, http.StatusInternalServerError)
			return
		}
	}

	master := resource.NewMaster(ormer, gd.Cluster)
	err = master.InstallNodes(ctx, horus, nodes)
	if err != nil {
		httpError2(w, err, http.StatusInternalServerError)
		return
	}

	response := make([]structs.PostNodeResponse, len(list))

	for i := range nodes {
		response[i] = structs.PostNodeResponse{
			ID:     nodes[i].Node.ID,
			Addr:   nodes[i].Node.Addr,
			TaskID: nodes[i].Task.ID,
		}
	}

	httpSucceed(w, response, http.StatusCreated)
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
		httpError2(w, err, http.StatusBadRequest)
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

		httpError2(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	horus, err := gd.KVClient().GetHorusAddr()
	if err != nil {
		httpError2(w, err, http.StatusInternalServerError)
		return
	}

	m := resource.NewMaster(gd.Ormer(), gd.Cluster)

	err = m.RemoveNode(ctx, horus, node, username, password, force)
	if err != nil {
		httpError2(w, err, http.StatusInternalServerError)
		return
	}

	httpSucceed(w, nil, http.StatusNoContent)
}

func postService(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpError2(w, err, http.StatusBadRequest)
		return
	}

	timeout := intValueOrZero(r, "timeout")
	if timeout > 0 {
		ctx, _ = goctx.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	}

	services := []structs.ServiceSpec{}
	err := json.NewDecoder(r.Body).Decode(&services)
	if err != nil {
		httpError2(w, err, http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil ||
		gd.KVClient() == nil ||
		gd.PluginClient() == nil {

		httpError2(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	stack := stack.New(gd, services)
	list, err := stack.DeployServices(ctx)
	// TODO:convert to structs.PostServiceResponse

	out := make([]database.Service, 0, len(list))

	for _, l := range list {
		out = append(out, database.Service(l.Spec().Service))
	}

	resp := structs.CommonResponse{
		ResponseHead: structs.ResponseHead{
			Result: true,
			Code:   http.StatusCreated,
		},
		Object: out,
	}

	if err != nil {
		resp.ResponseHead = structs.ResponseHead{
			Result:  false,
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		}
	}

	w.Header().Set("Content-Type", "application/json")

	err = json.NewEncoder(w).Encode(resp)
	if err != nil {
		logrus.WithField("status", resp.Code).Errorf("JSON Encode error: %+v", err)
	}

	w.WriteHeader(resp.Code)
}

func putServicesLink(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
}
