package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/store"
	"github.com/docker/swarm/utils"
	"github.com/gorilla/mux"
	goctx "golang.org/x/net/context"
)

var ErrUnsupportGardener = errors.New("Unsupported Gardener")

// GET /clusters/{name:.*}
func getClustersByNameOrID(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	cl, err := database.GetCluster(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	nodes, err := database.ListNodeByCluster(cl.ID)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	list := make([]structs.NodeInspect, len(nodes))
	for i, node := range nodes {
		var (
			totalCPUs    int
			usedCPUs     int
			totalMemory  int
			usedMemory   int
			dockerStatus string
		)

		if node.EngineID != "" {
			eng, err := gd.GetEngine(node.EngineID)
			if err == nil && eng != nil {
				totalCPUs = int(eng.Cpus)
				usedCPUs = int(eng.UsedCpus())
				totalMemory = int(eng.Memory)
				usedMemory = int(eng.UsedMemory())
				dockerStatus = eng.Status()
			}
		}

		list[i] = structs.NodeInspect{
			ID:           node.ID,
			Name:         node.Name,
			ClusterID:    node.ClusterID,
			Addr:         node.Addr,
			EngineID:     node.EngineID,
			DockerStatus: dockerStatus,
			Room:         node.Room,
			Seat:         node.Seat,
			MaxContainer: node.MaxContainer,
			Status:       node.Status,
			RegisterAt:   utils.TimeToString(node.RegisterAt),
			Resource: structs.Resource{
				TotalCPUs:   totalCPUs,
				UsedCPUs:    usedCPUs,
				TotalMemory: totalMemory,
				UsedMemory:  usedMemory,
			},
		}
	}

	resp := structs.PerClusterInfoResponse{
		ID:          cl.ID,
		Name:        cl.Name,
		Type:        cl.Type,
		StorageType: cl.StorageType,
		StorageID:   cl.StorageID,
		Datacenter:  cl.Datacenter,
		Enabled:     cl.Enabled,
		MaxNode:     cl.MaxNode,
		UsageLimit:  cl.UsageLimit,
		Nodes:       list,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// GET /clusters
func getClusters(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	clusters, err := database.ListCluster()
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	lists := make([]structs.ClusterInfoResponse, len(clusters))

	for i := range clusters {
		num, err := database.CountNodeByCluster(clusters[i].ID)
		if err != nil {
			httpError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		lists[i] = structs.ClusterInfoResponse{
			ID:          clusters[i].ID,
			Name:        clusters[i].Name,
			Type:        clusters[i].Type,
			StorageType: clusters[i].StorageType,
			StorageID:   clusters[i].StorageID,
			Datacenter:  clusters[i].Datacenter,
			Enabled:     clusters[i].Enabled,
			MaxNode:     clusters[i].MaxNode,
			NodeNum:     num,
			UsageLimit:  clusters[i].UsageLimit,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(lists)
}

// GET /resources
func getClustersResource(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	clusters, err := database.ListCluster()
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	resp := make([]structs.ClusterResource, len(clusters))
	for i := range clusters {
		resp[i], err = getClusterResource(gd, clusters[i], false)
		if err != nil {
			httpError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// GET /resources/{cluster:.*}
func getNodesResourceByCluster(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["cluster"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	cluster, err := database.GetCluster(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp, err := getClusterResource(gd, cluster, true)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func getClusterResource(gd *swarm.Gardener, cl database.Cluster, detail bool) (structs.ClusterResource, error) {
	nodes, err := database.ListNodeByCluster(cl.ID)
	if err != nil {
		return structs.ClusterResource{}, err
	}

	var nodesDetail []structs.NodeResource = nil
	var totalCPUs, usedCPUs, totalMemory, usedMemory int64
	if detail {
		nodesDetail = make([]structs.NodeResource, len(nodes))
	}

	for i := range nodes {
		if nodes[i].EngineID != "" {
			eng, err := gd.GetEngine(nodes[i].EngineID)
			if err != nil || eng == nil {
				logrus.Warnf("Engine %s Not Found,%v", nodes[i].EngineID, err)
				continue
			}
			_CPUs := eng.UsedCpus()
			_Memory := eng.UsedMemory()
			totalCPUs += eng.Cpus
			totalMemory += eng.Memory
			usedCPUs += _CPUs
			usedMemory += _Memory

			if detail {
				containers := eng.Containers()
				crs := make([]structs.ContainerWithResource, len(containers))
				for index, c := range containers {
					ncpu, err := utils.GetCPUNum(c.Info.HostConfig.CpusetCpus)
					if err != nil {
						ncpu = c.Info.HostConfig.CPUShares
					}

					crs[index] = structs.ContainerWithResource{
						ID:             c.ID,
						Name:           c.Info.Name,
						Image:          c.Image,
						Driver:         c.Info.Driver,
						NetworkMode:    c.HostConfig.NetworkMode,
						Created:        c.Info.Created,
						State:          cluster.StateString(c.Info.State),
						Labels:         c.Labels,
						Env:            c.Info.Config.Env,
						Mounts:         c.Mounts,
						CpusetCpus:     c.Info.HostConfig.CpusetCpus,
						CPUs:           ncpu,
						Memory:         c.Info.HostConfig.Memory,
						MemorySwap:     c.Info.HostConfig.MemorySwap,
						OomKillDisable: *c.Info.HostConfig.OomKillDisable,
					}
				}

				nodesDetail[i] = structs.NodeResource{
					ID:       nodes[i].ID,
					Name:     nodes[i].Name,
					EngineID: eng.ID,
					Addr:     eng.IP,
					Status:   eng.Status(),
					Resource: structs.Resource{
						TotalCPUs:   int(eng.Cpus),
						UsedCPUs:    int(_CPUs),
						TotalMemory: int(eng.Memory),
						UsedMemory:  int(_Memory),
					},
					Containers: crs,
				}
			}
		}
	}

	return structs.ClusterResource{
		ID:     cl.ID,
		Name:   cl.Name,
		Enable: cl.Enabled,
		Entire: structs.Resource{
			TotalCPUs:   int(totalCPUs),
			UsedCPUs:    int(usedCPUs),
			TotalMemory: int(totalMemory),
			UsedMemory:  int(usedMemory),
		},
		Nodes: nodesDetail,
	}, nil
}

// GET /ports
func getPorts(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	start := intValueOrZero(r, "start")
	end := intValueOrZero(r, "end")
	limit := intValueOrZero(r, "limit")

	ports, err := swarm.ListPorts(start, end, limit)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := make([]structs.PortResponse, len(ports))
	for i := range ports {
		resp[i] = structs.PortResponse{
			Port:      ports[i].Port,
			Name:      ports[i].Name,
			UnitID:    ports[i].UnitID,
			Proto:     ports[i].Proto,
			Allocated: ports[i].Allocated,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// GET /networkings
func getNetworkings(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	resp, err := swarm.ListNetworkings()
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// GET /image/{name:.*}
func getImage(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	image, config, err := database.GetImageAndUnitConfig(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var labels map[string]string
	err = json.NewDecoder(strings.NewReader(image.Labels)).Decode(&labels)
	if err != nil {
		labels = nil
	}

	var keysets map[string]structs.KeysetParams
	if len(config.KeySets) > 0 {
		keysets = make(map[string]structs.KeysetParams, len(config.KeySets))
		for key, val := range config.KeySets {
			keysets[key] = structs.KeysetParams{
				Key:         val.Key,
				CanSet:      val.CanSet,
				MustRestart: val.MustRestart,
				Description: val.Description,
			}
		}
	}

	resp := structs.GetImageResponse{
		ID:       image.ID,
		Name:     image.Name,
		Version:  image.Version,
		ImageID:  image.ImageID,
		Labels:   labels,
		Enabled:  image.Enabled,
		Size:     image.Size,
		UploadAt: utils.TimeToString(image.UploadAt),
		TemplateConfig: structs.ImageConfigResponse{
			ID:      config.ID,      // string `json:"config_id"`
			Mount:   config.Mount,   // string `json:"config_mount_path"`
			Content: config.Content, // string `json:"config_content"`
			KeySet:  keysets,        // map[string]KeysetParams,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
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

func floatValueOrZero(r *http.Request, k string) float64 {
	val, err := strconv.ParseFloat(r.FormValue(k), 64)
	if err != nil {
		return 0
	}
	return val
}

// POST /clusters/{name}/update
func postUpdateClusterParams(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.UpdateClusterParamsRequest{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err = gd.UpdateDatacenterParams(name, req.MaxNode, req.UsageLimit)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// POST /clusters/{name}/enable
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

// POST /clusters/{name}/disable
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

// POST /clusters/{name}/nodes
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

	for i, l := range list {
		nodes[i] = swarm.NewNode(l.Address, l.Name, dc.ID,
			l.Username, l.Password, l.Room, l.Seat, l.HDD, l.SSD,
			l.Port, l.MaxContainer)

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

//POST /clusters/nodes/{node}/enable
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

//POST /clusters/nodes/{node}/disable
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

// POST /clusters/nodes/{node}/update
func updateNode(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["node"]
	req := structs.UpdateNodeSetting{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err := gd.SetNodeParams(name, req.MaxContainer)
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

	svc, strategy, err := gd.CreateService(req)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := structs.PostServiceResponse{
		ID:               svc.ID,
		BackupStrategyID: strategy.ID,
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

	svc, err := gd.GetService(name)
	if err != nil {
		httpError(w, fmt.Sprintf("Not Found Service %s,Error:%s", name, err.Error()), http.StatusInternalServerError)
		return
	}

	err = svc.StartService()
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// POST /services/{name:.*}/stop
func postServiceStop(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}
	svc, err := gd.GetService(name)
	if err != nil {
		httpError(w, fmt.Sprintf("Not Found Service %s,Error:%s", name, err.Error()), http.StatusInternalServerError)
		return
	}

	err = svc.StopService()
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
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

// POST /services/{name:.*}/recreate
func postServiceRecreate(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}
	err := gd.RecreateAndStartService(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
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

	err := gd.UpdateServiceBackupStrategy(name, req)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

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

// POST /units/{name:.*}/start
func postUnitStart(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err := gd.StartUnitService(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// POST /units/{name:.*}/stop
func postUnitStop(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := mux.Vars(r)["name"]
	timeout := intValueOrZero(r, "timeout")

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err := gd.StopUnitService(name, timeout)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// POST /units/{name:.*}/backup
func postUnitBackup(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {}

// POST /units/{name:.*}/recover
func postUnitRecover(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {}

// POST /units/{name:.*}/migrate
func postUnitMigrate(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {}

// POST /units/{name:.*}/rebuild
func postUnitRebuild(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {}

// POST /units/{name:.*}/isolate
func postUnitIsolate(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {}

// POST /units/{name:.*}/switchback
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

// POST /ports
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

// Load Image
// POST /image/load
func postImageLoad(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.PostLoadImageRequest{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	id, err := swarm.LoadImage(req)
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

	err := swarm.UpdateImageStatus(image, true)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// POST 	/image/{image:.*}/disable
func postDisableImage(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	image := mux.Vars(r)["image"]

	err := swarm.UpdateImageStatus(image, false)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// POST /image/{image:.*}/template
func updateImageTemplateConfig(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	image := mux.Vars(r)["image"]

	req := structs.UpdateUnitConfigRequest{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	config, err := swarm.UpdateImageTemplateConfig(image, req)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var keysets map[string]structs.KeysetParams
	if len(config.KeySets) > 0 {
		keysets = make(map[string]structs.KeysetParams, len(config.KeySets))
		for key, val := range config.KeySets {
			keysets[key] = structs.KeysetParams{
				Key:         val.Key,
				CanSet:      val.CanSet,
				MustRestart: val.MustRestart,
				Description: val.Description,
			}
		}
	}

	resp := structs.ImageConfigResponse{
		ID:      config.ID,      // string `json:"config_id"`
		Mount:   config.Mount,   // string `json:"config_mount_path"`
		Content: config.Content, // string `json:"config_content"`
		KeySet:  keysets,        // map[string]KeysetParams,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
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

// POST /storage/san/{name}/raidgroup
func postRGToSanStorage(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := struct{ ID int }{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := mux.Vars(r)["name"]

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

	size, err := store.AddSpace(req.ID)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "{%q:%d}", "Size", size)
}

// POST /storage/san/{name}/raid_group/{rg:[0-9]+}/enable
func postEnableRaidGroup(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	san := mux.Vars(r)["name"]
	rg, err := strconv.Atoi(mux.Vars(r)["rg"])
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

// POST /storage/san/{name}/raid_group/{rg:[0-9]+}/disable
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

// DELETE /services/{name}
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

	err := gd.RemoveService(name, force, volumes, timeout)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DELETE /services/backup_strategy/{name:.*}
func deleteBackupStrategy(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	err := swarm.DeleteServiceBackupStrategy(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DELETE /clusters/{name}
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

// DELETE /ports/{port:[0-9]+}
func deletePort(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	port, err := strconv.Atoi(mux.Vars(r)["port"])
	if err != nil {
		msg := fmt.Sprintf("Parse error:%s,port must in range 1~65535", err)
		httpError(w, msg, http.StatusBadRequest)
		return
	}
	if port <= 0 || port > 65535 {
		httpError(w, "port must in range 1~65535", http.StatusBadRequest)
		return
	}

	err = database.DeletePort(port, false)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DELETE /storage/san/{name}
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

// DELETE /storage/san/{name}/raid_group/{rg:[0-9]+}
func deleteRaidGroup(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	san := mux.Vars(r)["name"]

	rg, err := strconv.Atoi(mux.Vars(r)["rg"])
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
