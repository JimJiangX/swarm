package api

import (
	"encoding/json"
	stderr "errors"
	"fmt"
	"net"
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
func httpJSONError(w http.ResponseWriter, err error, status int) {
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

func writeJSON(w http.ResponseWriter, obj interface{}, status int) {
	if obj != nil {
		w.Header().Set("Content-Type", "application/json")

		err := json.NewEncoder(w).Encode(obj)
		if err != nil {
			logrus.WithField("status", status).Errorf("JSON Encode error: %+v", err)
		}
	}

	w.WriteHeader(status)
}

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
	id, err := resource.LoadImage(ctx, gd.Ormer(), req)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "Id", id)
}

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
		MaxNode:    req.MaxNode,
		UsageLimit: req.UsageLimit,
	}

	err = gd.Ormer().InsertCluster(c)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%q}", "Id", c.ID)
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
		MaxNode:    req.MaxNode,
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
		gd.Ormer() == nil {

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
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	for i := range list {
		if list[i].Cluster == "" {
			httpJSONError(w, fmt.Errorf("host:%s ClusterID is required", list[i].Address), http.StatusInternalServerError)
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
			httpJSONError(w, fmt.Errorf("host:%s unknown ClusterID:%s", list[i].Address, list[i].Cluster), http.StatusInternalServerError)
			return
		}
	}

	nodes := resource.NewNodeWithTaskList(len(list))
	for i, n := range list {
		nodes[i], err = resource.NewNodeWithTask(n.Username, n.Password,
			n.HDD, n.SSD,
			database.Node{
				ID:           utils.Generate32UUID(),
				ClusterID:    n.Cluster,
				Addr:         n.Address,
				EngineID:     "",
				Room:         n.Room,
				Seat:         n.Seat,
				MaxContainer: n.MaxContainer,
				Status:       0,
				Enabled:      false,
			})
		if err != nil {
			httpJSONError(w, err, http.StatusInternalServerError)
			return
		}
	}

	horus, err := gd.KVClient().GetHorusAddr()
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	master := resource.NewMaster(orm, gd.Cluster)
	err = master.InstallNodes(ctx, horus, nodes)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	out := make([]structs.PostNodeResponse, len(list))

	for i := range nodes {
		out[i] = structs.PostNodeResponse{
			ID:   nodes[i].Node.ID,
			Addr: nodes[i].Node.Addr,
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

	err = m.RemoveNode(ctx, horus, node, username, password, force)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

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
	if ip := net.ParseIP(req.Gatewary); ip == nil {
		httpJSONError(w, fmt.Errorf("illegal Gateway:'%s' error", req.Gatewary), http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	nw := resource.NewNetworks(gd.Ormer())
	n, err := nw.AddNetworking(req.Start, req.End, req.Gatewary, name, int(req.VLAN), req.Prefix)
	if err != nil {
		httpJSONError(w, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%d}", "num", n)
}

func putNetworkingEnable(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	var req structs.PutNetworkingRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		logrus.Warnf("JSON Decode: %s", err)
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	orm := gd.Ormer()
	filters := make([]uint32, 0, len(req.Filters))

	if len(req.Filters) > 0 {
		list, err := orm.ListIPByNetworking(name)
		if err != nil {
			httpJSONError(w, err, http.StatusInternalServerError)
			return
		}

		for i := range req.Filters {
			n := utils.IPToUint32(req.Filters[i])
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
				httpJSONError(w, fmt.Errorf("IP %s is not in networking %s", req.Filters[i], name), http.StatusInternalServerError)
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
	name := mux.Vars(r)["name"]

	var req structs.PutNetworkingRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		logrus.Warnf("JSON Decode: %s", err)
	}

	ok, _, gd := fromContext(ctx, _Garden)
	if !ok || gd == nil ||
		gd.Ormer() == nil {

		httpJSONError(w, errUnsupportGarden, http.StatusInternalServerError)
		return
	}

	orm := gd.Ormer()
	filters := make([]uint32, 0, len(req.Filters))

	if len(req.Filters) > 0 {
		list, err := orm.ListIPByNetworking(name)
		if err != nil {
			httpJSONError(w, err, http.StatusInternalServerError)
			return
		}

		for i := range req.Filters {
			n := utils.IPToUint32(req.Filters[i])
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
				httpJSONError(w, fmt.Errorf("IP %s is not in networking %s", req.Filters[i], name), http.StatusInternalServerError)
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

func postService(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpJSONError(w, err, http.StatusBadRequest)
		return
	}

	timeout := intValueOrZero(r, "timeout")
	if timeout > 0 {
		ctx, _ = goctx.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	}

	services := []structs.ServiceSpec{}
	err := json.NewDecoder(r.Body).Decode(&services)
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

	stack := stack.New(gd, services)
	list, err := stack.DeployServices(ctx)
	// TODO:convert to structs.PostServiceResponse

	out := make([]database.Service, 0, len(list))

	for _, l := range list {
		out = append(out, database.Service(l.Spec().Service))
	}

	writeJSON(w, out, http.StatusCreated)
}

func putServicesLink(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
}
