package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"sort"
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
	consulapi "github.com/hashicorp/consul/api"
	goctx "golang.org/x/net/context"
)

var ErrUnsupportGardener = errors.New("Unsupported Gardener")

func getNodeInspect(gd *swarm.Gardener, node database.Node) structs.NodeInspect {
	var (
		totalCPUs    int
		usedCPUs     int
		totalMemory  int
		usedMemory   int
		dockerStatus = "Disconnected"
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

	return structs.NodeInspect{
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

// GET /nodes/{name:.*}
func getNode(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	node, err := database.GetNode(name)
	if err != nil {
		httpError(w, fmt.Sprintf("Not Found Node by NameOrID:%s,Error:%s", name, err), http.StatusInternalServerError)
		return
	}
	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	resp := getNodeInspect(gd, node)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// GET /nodes
func getAllNodes(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	nodes, err := database.GetAllNodes()
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	resp := make([]structs.NodeInspect, len(nodes))
	for i := range nodes {
		resp[i] = getNodeInspect(gd, nodes[i])
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

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
	for i := range nodes {
		list[i] = getNodeInspect(gd, *nodes[i])
	}

	resp := structs.PerClusterInfoResponse{
		ClusterInfoResponse: structs.ClusterInfoResponse{
			ID:           cl.ID,
			Name:         cl.Name,
			Type:         cl.Type,
			StorageType:  cl.StorageType,
			StorageID:    cl.StorageID,
			NetworkingID: cl.NetworkingID,
			Datacenter:   swarm.DatacenterID,
			Enabled:      cl.Enabled,
			MaxNode:      cl.MaxNode,
			UsageLimit:   cl.UsageLimit,
			NodeNum:      len(nodes),
		},
		Nodes: list,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// GET /clusters
func getClusters(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	clusters, err := database.ListClusters()
	if err != nil {
		logrus.Error("List Cluster", err)
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	lists := make([]structs.ClusterInfoResponse, len(clusters))

	for i := range clusters {
		num, err := database.CountNodeByCluster(clusters[i].ID)
		if err != nil {
			logrus.Error("Count Node By Cluster", err)
			httpError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		lists[i] = structs.ClusterInfoResponse{
			ID:           clusters[i].ID,
			Name:         clusters[i].Name,
			Type:         clusters[i].Type,
			StorageType:  clusters[i].StorageType,
			StorageID:    clusters[i].StorageID,
			NetworkingID: clusters[i].NetworkingID,
			Datacenter:   swarm.DatacenterID,
			Enabled:      clusters[i].Enabled,
			MaxNode:      clusters[i].MaxNode,
			NodeNum:      num,
			UsageLimit:   clusters[i].UsageLimit,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(lists)
}

// GET /resources
func getClustersResource(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	clusters, err := database.ListClusters()
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
		if i < len(nodesDetail) {
			nodesDetail[i] = structs.NodeResource{
				ID:       nodes[i].ID,
				Name:     nodes[i].Name,
				EngineID: nodes[i].EngineID,
				Addr:     nodes[i].Addr,
				Status:   swarm.ParseNodeStatus(nodes[i].Status),
			}
		}

		if nodes[i].EngineID != "" {
			eng, err := gd.GetEngine(nodes[i].EngineID)
			if err != nil || eng == nil {
				logrus.Warnf("Engine %s Not Found,%v", nodes[i].EngineID, err)
				continue
			}

			totalCPUs += eng.Cpus
			totalMemory += eng.Memory
			_CPUs := eng.UsedCpus()
			_Memory := eng.UsedMemory()
			usedCPUs += _CPUs
			usedMemory += _Memory

			if i < len(nodesDetail) {
				nodesDetail[i] = structs.NodeResource{
					ID:       nodes[i].ID,
					Name:     nodes[i].Name,
					EngineID: eng.ID,
					Addr:     eng.IP,
					Status:   eng.Status(),
					Labels:   eng.Labels,
					Resource: structs.Resource{
						TotalCPUs:   int(eng.Cpus),
						UsedCPUs:    int(_CPUs),
						TotalMemory: int(eng.Memory),
						UsedMemory:  int(_Memory),
					},
					Containers: containerWithResource(eng.Containers()),
				}
			}
		}
	}

	return structs.ClusterResource{
		ID:     cl.ID,
		Name:   cl.Name,
		Type:   cl.Type,
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

func containerWithResource(containers cluster.Containers) []structs.ContainerWithResource {
	if len(containers) == 0 {
		return nil
	}

	out := make([]structs.ContainerWithResource, len(containers))
	for i, c := range containers {
		ncpu, err := utils.GetCPUNum(c.Info.HostConfig.CpusetCpus)
		if err != nil {
			ncpu = c.Info.HostConfig.CPUShares
		}

		out[i] = structs.ContainerWithResource{
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

	return out
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
			UnitName:  ports[i].UnitName,
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

	kvs, err := iniParse(config.Content, config.KeySets)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
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
			ID:    config.ID,    // string `json:"config_id"`
			Mount: config.Mount, // string `json:"config_mount_path"`
			KVs:   kvs,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// GET /services?from=DBAAS
func getServices(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	from := r.FormValue("from")

	services, err := database.ListServices()
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	containers := gd.Containers()
	consulClient, err := gd.ConsulAPIClient()
	if err != nil {
		logrus.Error(err)
	}

	var response io.Reader
	switch strings.ToUpper(from) {
	case "DBAAS":
		logrus.Debugf("From %s", from)

		response = listServiceFromDBAAS(services, containers, consulClient)
	default:
		logrus.Debugf("From %s", "default")

		list := make([]structs.ServiceResponse, len(services))
		for i := range services {
			list[i] = getServiceResponse(services[i], containers, consulClient)
		}

		buffer := bytes.NewBuffer(nil)
		json.NewEncoder(buffer).Encode(list)
		response = buffer
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, response)
}

const (
	servicePassing  = "passing"
	serviceUnknown  = "unknown"
	serviceWarning  = "warning"
	serviceCritical = "critical"
)

func getServiceRunningStatus(serviceID string, units []database.Unit,
	containers cluster.Containers, checks map[string]consulapi.HealthCheck) string {
	if len(units) == 0 && serviceID != "" {
		var err error
		units, err = database.ListUnitByServiceID(serviceID)
		if err != nil {
			logrus.Errorf("List Unit By ServiceID Error:%s,serviceID=%s", err, serviceID)
			return serviceUnknown
		}
	}

	if len(units) == 0 {
		return serviceCritical
	}

	total := len(units)
	m := make(map[string][]int, total)
	for i := range units {
		list, ok := m[units[i].Type]
		if !ok {
			list = make([]int, 0, total)
		}

		list = append(list, i)
		m[units[i].Type] = list
	}

	state := servicePassing

	for _type, list := range m {
		count := 0
		for _, index := range list {
			u := units[index]
			c := containers.Get(u.ContainerID)

			if c == nil {
				count++

				logrus.Warnf(" Unit '%s',Not Found Container By '%s'", u.Name, u.ContainerID)
				continue
			}
			if c.Engine == nil {
				count++

				logrus.Warnf("Unit '%s',Engine is nil", u.Name)
				continue
			}
			if c.State != "running" {
				count++

				logrus.Warnf("Unit '%s',Container State=%s Status=%s", u.Name, c.State, c.Status)
				continue
			}
			if status := c.Engine.Status(); status != "Healthy" {
				count++

				logrus.Warnf("Unit '%s',Engine Status=%s", u.Name, status)
				continue
			}
			if val, ok := checks[u.ID]; !ok || val.Status != servicePassing {
				count++

				if !ok {
					logrus.Warnf("Unit '%s',Not Found Status In Consul", u.Name)
				} else {
					logrus.Warnf("Unit '%s',Status In Consul:'%s'", u.Name, val.Status)
				}
				continue
			}
		}

		if count > 0 && state == servicePassing {
			state = serviceWarning
		}

		if len(list)-count == 0 && _type != "switch_manager" {
			state = serviceCritical
		}
	}

	return state
}

func listServiceFromDBAAS(services []database.Service,
	containers cluster.Containers, client *consulapi.Client) io.Reader {
	type response struct {
		ID           string //"id": "??",
		Name         string //"name": "test01",
		BusinessCode string `json:"business_code"` // "business_code": "??",

		Version string // "upsql_version": "??",
		Arch    string // "upsql_arch": "??",

		CpusetCpus string // "upsql_cpusetCpus": "??",
		Memory     int64  // "upsql_memory": "??",

		ManageStatus  int64  `json:"manage_status"`  //"manage_status": "??",
		RunningStatus string `json:"running_status"` //"running_status": "??",
		CreatedAt     string `json:"created_at"`     // "created_at": "??"
	}

	checks, err := swarm.HealthChecksFromConsul(client, "any", nil)
	if err != nil {
		logrus.Error(err)
		checks = make(map[string]consulapi.HealthCheck, 0)
	}

	out := make([]response, len(services))
	for i := range services {
		desc := structs.PostServiceRequest{}
		err := json.NewDecoder(bytes.NewBufferString(services[i].Description)).Decode(&desc)
		if err != nil {
			logrus.Warningf("JSON Decode Serivce.Description %s:%s,%s", services[i].Name, err, services[i].Description)
		}
		sql := structs.Module{}
		for _, m := range desc.Modules {
			if m.Type == "upsql" {
				sql = m
				break
			}
		}
		out[i] = response{
			Name:          services[i].Name,
			ID:            services[i].ID,
			BusinessCode:  services[i].BusinessCode,
			Version:       sql.Version,
			Arch:          sql.Arch,
			Memory:        sql.HostConfig.Memory,
			CpusetCpus:    sql.HostConfig.CpusetCpus,
			ManageStatus:  services[i].Status,
			RunningStatus: getServiceRunningStatus(services[i].ID, nil, containers, checks),
			CreatedAt:     utils.TimeToString(services[i].CreatedAt),
		}
	}

	rw := bytes.NewBuffer(nil)
	json.NewEncoder(rw).Encode(out)

	return rw
}

// GET /services/{name}
func getServicesByNameOrID(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	service, err := database.GetService(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	consulClient, err := gd.ConsulAPIClient()
	if err != nil {
		logrus.Error(err)
	}

	resp := getServiceResponse(service, gd.Containers(), consulClient)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func getServiceResponse(service database.Service, containers cluster.Containers,
	client *consulapi.Client) structs.ServiceResponse {
	desc := structs.PostServiceRequest{}
	err := json.NewDecoder(bytes.NewBufferString(service.Description)).Decode(&desc)
	if err != nil {
		logrus.Warnf("JSON Decode Serivce.Description %s:%s,%s", service.Name, err, service.Description)
	}

	_, files, err := database.ListBackupFilesByService(service.ID)
	if err != nil {
		logrus.Error(err)
	}

	usedSpaceByte := 0
	for i := range files {
		usedSpaceByte += files[i].SizeByte
	}

	units, err := database.ListUnitByServiceID(service.ID)
	if err != nil {
		logrus.Error("ListUnitByServiceID", err)
	}

	roles := make(map[string]string)
	for i := range units {
		if units[i].Type == "switch_manager" {
			roles, err = swarm.GetUnitRoleFromConsul(client, service.ID+"/"+units[i].Name)
			if err == nil {
				break
			} else {
				logrus.Error(err)
			}
		}
	}

	checks, err := swarm.HealthChecksFromConsul(client, "any", nil)
	if err != nil {
		logrus.Error(err)
		checks = make(map[string]consulapi.HealthCheck, 0)
	}

	names := make([]string, len(units))
	for i := range units {
		names[i] = units[i].EngineID
	}
	nodes, err := database.ListNodesByEngines(names)
	if err != nil {
		logrus.Error(err, names)
	}

	list := make([]structs.UnitInfo, len(units))
	for i := range units {
		node := database.Node{}
		for n := range nodes {
			if nodes[n].EngineID == units[i].EngineID {
				node = nodes[n]
				break
			}
		}

		networkings, ports := getUnitNetworking(units[i].ID)

		list[i] = structs.UnitInfo{
			ID:          units[i].ID,
			Name:        units[i].Name,
			Type:        units[i].Type,
			NodeID:      node.ID,
			NodeAddr:    node.Addr,
			ClusterID:   node.ClusterID,
			Networkings: networkings,
			Ports:       ports,
			Role:        roles[units[i].Name],
			Status:      checks[units[i].ID].Status,
			CreatedAt:   utils.TimeToString(units[i].CreatedAt),
		}

		if list[i].Role == "" && list[i].Type == "upsql" {
			list[i].Role = "unknown"
		}

		container := containers.Get(units[i].ContainerID)
		if container != nil {
			list[i].Info = container.Info
			list[i].CpusetCpus = container.Info.HostConfig.CpusetCpus
			list[i].Memory = container.Info.HostConfig.Memory
			list[i].State = container.Status
		} else {
			list[i].Status = serviceCritical
		}
	}

	return structs.ServiceResponse{
		ID:                   service.ID,
		Name:                 service.Name,
		Architecture:         service.Architecture,
		Description:          desc,
		HighAvailable:        service.HighAvailable,
		Status:               service.Status,
		BackupMaxSizeByte:    service.BackupMaxSizeByte,
		BackupUsedSizeByte:   usedSpaceByte,
		BackupFilesRetention: service.BackupFilesRetention,
		RunningStatus:        getServiceRunningStatus(service.ID, units, containers, checks),
		CreatedAt:            utils.TimeToString(service.CreatedAt),
		FinishedAt:           utils.TimeToString(service.FinishedAt),
		Containers:           list,
	}
}

func getUnitNetworking(id string) ([]struct {
	Type string
	Addr string
}, []struct {
	Name string
	Port int
}) {
	ips, err := database.ListIPByUnitID(id)
	if err != nil {
		logrus.Error("%s List IP error %s", id, err)
	}

	networkings := make([]struct {
		Type string
		Addr string
	}, len(ips))

	for i := range ips {
		networking, _, err := database.GetNetworkingByID(ips[i].NetworkingID)
		if err != nil {
			logrus.Error("Get Networking By ID", err, ips[i].NetworkingID)
		}

		ip := utils.Uint32ToIP(ips[i].IPAddr)

		networkings[i].Type = networking.Type
		networkings[i].Addr = fmt.Sprintf("%s/%d", ip.String(), ips[i].Prefix)
	}

	out, err := database.ListPortsByUnit(id)
	if err != nil {
		logrus.Error("%s List Port error %s", id, err)
	}

	ports := make([]struct {
		Name string
		Port int
	}, len(out))

	for i := range out {
		ports[i].Name = out[i].Name
		ports[i].Port = out[i].Port
	}

	return networkings, ports
}

// GET /services/{name}/users
func getServiceUsers(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	if err := r.ParseForm(); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	_type := "proxy"
	if key, ok := r.Form["filter"]; ok {
		if len(key) == 0 {
			httpError(w, r.URL.String(), http.StatusBadRequest)
			return
		}
		_type = key[0]
	}

	users, err := database.ListUsersByService(name, _type)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(users)
}

// GET /services/{name}/topology
func hijackTopology(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	service, err := gd.GetService(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	addr, err := service.GetSwitchManagerAddr()
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	r.URL.Path = "/topology"

	logrus.Debugf("hijack to %s,URL:%s", addr, r.URL.String())

	err = hijack(nil, addr, w, r)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// GET /services/{name}/proxys
func hijackProxys(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	service, err := gd.GetService(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	addr, err := service.GetSwitchManagerAddr()
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	r.URL.Path = "/proxys"

	logrus.Debugf("hijack to %s,URL:%s", addr, r.URL.String())

	err = hijack(nil, addr, w, r)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// GET /services/{name}/service_config
func getServiceServiceConfig(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	service, err := database.GetService(name)
	if err != nil {
		httpError(w, fmt.Sprintf("Not Found Service '%s',Error:%s", name, err), http.StatusInternalServerError)
		return
	}

	configs, err := database.ListUnitConfigByService(service.ID)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := make([]structs.UnitConfigResponse, len(configs))
	for i := range configs {
		keysetsMap := converteToKeysetParams(configs[i].Config.KeySets)

		resp[i] = structs.UnitConfigResponse{
			ID:        configs[i].Unit.ID,
			Name:      configs[i].Unit.Name,
			Type:      configs[i].Unit.Type,
			Config:    make([]structs.ValueAndKeyset, 0, len(keysetsMap)),
			CreatedAt: utils.TimeToString(configs[i].Config.CreatedAt),
		}

		parser, _, err := swarm.Factory(resp[i].Type)
		if err != nil {
			logrus.Error(err)

			continue
		}
		configer, err := parser.ParseData([]byte(configs[i].Config.Content))
		if err != nil {
			logrus.Error("ParseData error", err, configs[i].Config.Content)
			continue
		}

		for key, val := range keysetsMap {
			value := configer.String(key)
			resp[i].Config = append(resp[i].Config, structs.ValueAndKeyset{
				Value:        value,
				KeysetParams: val,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// GET /services/{name}/backup_strategy
func getServiceBackupStrategy(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	service, err := database.GetService(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	list, err := database.ListBackupStrategyByServiceID(service.ID)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	out := make([]structs.BackupStrategy, len(list))

	for i := range list {
		out[i] = structs.BackupStrategy{
			ID:        list[i].ID,
			Name:      list[i].Name,
			Type:      list[i].Type,
			Spec:      list[i].Spec,
			Valid:     utils.TimeToString(list[i].Valid),
			CreatedAt: utils.TimeToString(list[i].CreatedAt),
			Enable:    list[i].Enabled,
			BackupDir: list[i].BackupDir,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(out)
}

// GET /services/{name}/backup_files
func getServiceBackupFiles(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	_, files, err := database.ListBackupFilesByService(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	list := make([]structs.BackupFile, len(files))
	for i := range files {
		list[i] = structs.BackupFile{
			ID:         files[i].ID,
			Name:       path.Base(files[i].Path),
			TaskID:     files[i].TaskID,
			StrategyID: files[i].StrategyID,
			UnitID:     files[i].UnitID,
			Type:       files[i].Type,
			Path:       files[i].Path,
			SizeByte:   files[i].SizeByte,
			Retention:  utils.TimeToString(files[i].Retention),
			CreatedAt:  utils.TimeToString(files[i].CreatedAt),
			FinishedAt: utils.TimeToString(files[i].FinishedAt),
			Created:    files[i].CreatedAt,
		}
	}

	sortByTime := structs.BackupFiles(list)
	sort.Sort(sortByTime)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(sortByTime)
}

// GET /storage/san
func getSANStoragesInfo(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	names, err := database.ListStorageID()
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := make([]structs.SANStorageResponse, len(names))
	for i := range names {
		resp[i], err = getSanStoreInfo(names[i])
		if err != nil {
			httpError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// GET /storage/san/{name:.*}
func getSANStorageInfo(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	resp, err := getSanStoreInfo(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func getSanStoreInfo(id string) (structs.SANStorageResponse, error) {
	store, err := store.GetStoreByID(id)
	if err != nil {
		return structs.SANStorageResponse{}, err
	}

	info, err := store.Info()
	if err != nil {
		return structs.SANStorageResponse{}, err
	}

	i := 0
	spaces := make([]structs.Space, len(info.List))
	for _, val := range info.List {
		spaces[i] = structs.Space{
			Enable: val.Enable,
			ID:     val.ID,
			Total:  val.Total,
			Free:   val.Free,
			LunNum: val.LunNum,
			State:  val.State,
		}
		i++
	}

	return structs.SANStorageResponse{
		ID:     info.ID,
		Vendor: info.Vendor,
		Driver: info.Driver,
		Total:  info.Total,
		Free:   info.Free,
		Used:   info.Total - info.Free,
		Spaces: spaces,
	}, nil
}

// GET /tasks
func getTasks(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	var (
		withCondition = false
		tasks         []database.Task
		begin, end    time.Time
	)
	if v, ok := r.Form["status"]; ok {
		if len(v) == 0 {
			httpError(w, r.URL.String(), http.StatusBadRequest)
			return
		}
		status, err := strconv.Atoi(v[0])
		if err != nil {
			msg := fmt.Sprintf("parse status:'%s' error:%s", v, err)
			httpError(w, msg, http.StatusInternalServerError)
			return
		}
		tasks, err = database.ListTaskByStatus(status)
		withCondition = true
		logrus.Debugf("List Task By Status:%d", status)
	}

	if key, ok := r.Form["key"]; ok {
		if len(key) == 0 {
			httpError(w, r.URL.String(), http.StatusBadRequest)
			return
		}
		tasks, err = database.ListTaskByRelated(key[0])
		withCondition = true
		logrus.Debugf("List Task By Related:%s", key[0])
	}

	if key, ok := r.Form["begin"]; ok {
		if len(key) == 0 {
			httpError(w, r.URL.String(), http.StatusBadRequest)
			return
		}
		// "2016-07-19T10:41:05+08:00"
		// make sure time with location
		if !strings.HasSuffix(key[0], "+08:00") {
			key[0] += "+08:00"
		}
		begin, err = time.Parse(time.RFC3339, key[0])
		if err != nil {
			httpError(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	if key, ok := r.Form["end"]; ok {
		if len(key) == 0 {
			httpError(w, r.URL.String(), http.StatusBadRequest)
			return
		}
		// "2016-07-19T10:41:05+08:00"
		// make sure time with location
		if !strings.HasSuffix(key[0], "+08:00") {
			key[0] += "+08:00"
		}
		end, err = time.Parse(time.RFC3339, key[0])
		if err != nil {
			httpError(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	if !begin.IsZero() || !end.IsZero() {
		tasks, err = database.ListTaskByTimestamp(begin, end)
		withCondition = true
		logrus.Debugf("List Task By Timestamp,begin=%s,end=%s", begin, end)
	}

	if !withCondition {
		tasks, err = database.ListTask()
		logrus.Debug("List Tasks")
	}

	if err != nil {
		logrus.Errorf("getTasks error:%+v", err)

		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := make([]structs.TaskResponse, len(tasks))
	for i := range tasks {
		resp[i] = getTaskResponse(tasks[i])
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func getTaskResponse(task database.Task) structs.TaskResponse {
	var (
		err     error
		unit    database.Unit
		node    database.Node
		service database.Service
	)
	/*
		_Node_Install_Task   = "node_install"
		_Image_Load_Task     = "image_load"
		_Unit_Migrate_Task   = "unit_migrate"
		_Unit_Rebuild_Task   = "unit_rebuild"
		_Unit_Restore_Task   = "unit_restore"
		_Service_Create_Task = "service_create"
		_Backup_Auto_Task    = "backup_auto"
		_Backup_Manual_Task  = "backup_manual"
	*/
	switch task.Related {
	case "backup_auto":
		strategy, err := database.GetBackupStrategy(task.Linkto)
		if err == nil {
			service, err = database.GetService(strategy.ServiceID)
			if err != nil {
				logrus.Warning(err)
			}
		}

	case "node_install":
		node, err = database.GetNode(task.Linkto)
		if err != nil {
			logrus.Warning(err)
		}

	case "service_create":
		service, err = database.GetService(task.Linkto)
		if err != nil {
			logrus.Warning(err)
		}

	case "unit_migrate", "unit_rebuild", "unit_restore", "backup_manual":
		unit, err = database.GetUnit(task.Linkto)
		if err == nil {
			node, err = database.GetNode(unit.EngineID)
			if err != nil {
				logrus.Error(err)
			}

			service, err = database.GetService(unit.ServiceID)
			if err != nil {
				logrus.Error(err)
			}
		}

	default:
		logrus.Debugf("%v", task)
	}

	return structs.TaskResponse{
		ID:          task.ID,
		Related:     task.Related,
		Linkto:      task.Linkto,
		ServiceID:   service.ID,
		ServiceName: service.Name,
		NodeID:      node.ID,
		NodeName:    node.Name,
		UnitID:      unit.ID,
		UnitName:    unit.Name,
		Description: task.Description,
		Labels:      task.Labels,
		Errors:      task.Errors,
		Timeout:     task.Timeout,
		Status:      int(task.Status),
		Timestamp:   task.Timestamp,
		CreatedAt:   utils.TimeToString(task.CreatedAt),
		FinishedAt:  utils.TimeToString(task.FinishedAt),
	}
}

// GET /tasks/{name:.*}
func getTask(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	task, err := database.GetTask(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := getTaskResponse(*task)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// POST /datacenter
func postDatacenter(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.RegisterDatacenter{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}
	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err = swarm.RegisterDatacenter(gd, req)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

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

	if req.Type == "proxy" && req.NetworkingID != "" {
		_, _, err := database.GetNetworkingByID(req.NetworkingID)
		if err != nil {
			httpError(w, fmt.Sprintf("Not Found Networking By ID:%s,error:%s", req.NetworkingID, err), http.StatusInternalServerError)
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
		go dc.DistributeNode(nodes[i])
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

	svc, strategyID, taskID, err := gd.CreateService(req)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := structs.PostServiceResponse{
		ID:               svc.ID,
		BackupStrategyID: strategyID,
		TaskID:           taskID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// POST /services/{name:.*}/users
func postServiceUsers(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	users := []structs.User{}

	if err := json.NewDecoder(r.Body).Decode(&users); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	svc, err := gd.GetService(name)
	if err != nil {
		httpError(w, fmt.Sprintf("Not Found Service %s,Error:%s", name, err), http.StatusInternalServerError)
		return
	}

	code, err := svc.AddServiceUsers(users)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(code)
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
	service := mux.Vars(r)["name"]

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	taskID, err := gd.TemporaryServiceBackupTask(service, "")
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "{%q:%q}", "task_id", taskID)
}

// POST /services/{name:.*}/scale
func postServiceScaled(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	req := structs.PostServiceScaledRequest{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err := gd.ServiceScaleTask(name, req)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// POST /services/{name:.*}/slowlog
func postServiceSlowlog(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	req := structs.PostSlowlogRequest{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	svc, err := gd.GetService(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = svc.Slowlog(req.Enable, req.NotUsingIndexes, req.LongQueryTime)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// POST /services/{name:.*}/service-config/update
func postServiceConfig(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	req := structs.UpdateServiceConfigRequest{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	service, err := gd.GetService(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = service.UpdateUnitConfig(req.Type, req.Pairs)
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
	sys, err := gd.SystemConfig()
	if err != nil {
		logrus.Error(err)
	}

	req.BackupDir = sys.BackupDir

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
	sys, err := gd.SystemConfig()
	if err != nil {
		logrus.Error(err)
	}

	req.BackupDir = sys.BackupDir

	err = gd.UpdateServiceBackupStrategy(name, req)
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
		httpError(w, err.Error(), http.StatusBadRequest)
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
func postUnitBackup(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	unit := mux.Vars(r)["name"]
	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	taskID, err := gd.TemporaryServiceBackupTask("", unit)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "{%q:%q}", "task_id", taskID)
}

// POST /units/{name:.*}/restore
func postUnitRestore(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := mux.Vars(r)["name"]
	from := r.FormValue("from")

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	file, err := database.GetBackupFile(from)
	if err != nil {
		httpError(w, fmt.Sprintf("Not Found Backup File by ID:%s,Error:%s", from, err), http.StatusInternalServerError)
		return
	}

	taskID, err := gd.RestoreUnit(name, file.Path)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "{%q:%q}", "task_id", taskID)
}

// POST /units/{name:.*}/migrate
func postUnitMigrate(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	req := structs.PostMigrateUnit{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logrus.Warningf("postUnitRebuild,JSON Decode error:%s", err)
	}
	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	taskID, err := gd.UnitMigrate(name, req.Candidates, req.HostConfig)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "{%q:%q}", "task_id", taskID)
}

// POST /units/{name:.*}/rebuild
func postUnitRebuild(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	req := structs.PostRebuildUnit{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logrus.Warningf("postUnitRebuild,JSON Decode error:%s", err)
	}
	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	taskID, err := gd.UnitRebuild(name, req.Candidates, req.HostConfig)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "{%q:%q}", "task_id", taskID)
}

// POST /units/{name:.*}/isolate
func postUnitIsolate(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err := gd.UnitIsolate(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// POST /units/{name:.*}/switchback
func postUnitSwitchback(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err := gd.UnitSwitchBack(name)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// POST /tasks/backup/callback
func postBackupCallback(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	req := structs.BackupTaskCallback{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := swarm.BackupTaskCallback(req)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
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

	err := swarm.ValidateIPAddress(req.Prefix, req.Start, req.End, req.Gateway)
	if err != nil {
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

	imageID, taskID, err := swarm.LoadImage(req)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := structs.LoadImageResponse{
		ImageID: imageID,
		TaskID:  taskID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
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

	kvs := req.ConfigKVs
	if len(req.ConfigKVs) == 0 {
		kvs, err = iniParse(config.Content, config.KeySets)
		if err != nil {
			httpError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	resp := structs.ImageConfigResponse{
		ID:    config.ID,    // string `json:"config_id"`
		Mount: config.Mount, // string `json:"config_mount_path"`
		KVs:   kvs,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func converteToKeysetParams(from map[string]database.KeysetParams) map[string]structs.KeysetParams {
	if len(from) == 0 {
		return nil
	}

	keysets := make(map[string]structs.KeysetParams, len(from))
	for key, val := range from {
		keysets[key] = structs.KeysetParams{
			Key:         val.Key,
			CanSet:      val.CanSet,
			MustRestart: val.MustRestart,
			Description: val.Description,
		}
	}

	return keysets
}

func iniParse(content string, keysetsMap map[string]database.KeysetParams) ([]structs.ValueAndKeyset, error) {
	var (
		delimiter = "::"
		prefix    = "default"
		kvs       = make([]structs.ValueAndKeyset, 0, 100)
	)

	lenContent := len(content)
	lenKV := 0
	buf := bytes.NewBufferString(content)
	for {
		if lenKV == lenContent {
			break
		}
		s, err := buf.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}
		lenKV += len(s)
		s = strings.TrimSpace(s)
		if strings.Index(s, "[") == 0 {
			prefix = s[1 : len(s)-1]
			continue
		}
		if strings.Index(s, "#") == 0 {
			continue
		}
		index := strings.Index(s, "=")
		if index < 0 {
			continue
		}
		key := strings.TrimSpace(s[:index])
		if len(key) == 0 {
			continue
		}
		val := strings.TrimSpace(s[index+1:])
		if len(val) == 0 {
			continue
		}
		index = strings.Index(val, "#")
		if index > 0 {
			val = strings.TrimSpace(val[:index])
		}

		key = prefix + delimiter + key
		v := keysetsMap[key]
		kvs = append(kvs, structs.ValueAndKeyset{
			Value: val,
			KeysetParams: structs.KeysetParams{
				Key:         key,
				CanSet:      v.CanSet,
				MustRestart: v.MustRestart,
				Description: v.Description,
			},
		})
	}

	return kvs, nil
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

	store, err := store.RegisterStore(req.Vendor, req.Addr,
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
	req := structs.PostRaidGroupRequest{}
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

	err = gd.UpdateStoreSpaceStatus(san, rg, false)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// DELETE /services/{name}
// TODO:Not Done Yet
func deleteService(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
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

// DELETE /services/{name}/users
func deleteServiceUsers(ctx goctx.Context, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	name := mux.Vars(r)["name"]
	all := boolValue(r, "all")
	usernames := r.FormValue("usernames")
	users := strings.Split(usernames, ",")

	logrus.Debugf("%s %s %v", usernames, users, all)

	if len(users) == 0 && all == false {
		httpError(w, fmt.Sprintf("URL without {usernames}:'%s'", usernames), http.StatusBadRequest)
	}

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	svc, err := gd.GetService(name)
	if err != nil {
		httpError(w, fmt.Sprintf("Not Found Service '%s' Error:%s", name, err), http.StatusInternalServerError)
		return
	}

	err = svc.DeleteServiceUsers(users, all)
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
	if err := r.ParseForm(); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	node := mux.Vars(r)["node"]
	username := r.FormValue("username")
	password := r.FormValue("password")

	ok, _, gd := fromContext(ctx, _Gardener)
	if !ok && gd == nil {
		httpError(w, ErrUnsupportGardener.Error(), http.StatusInternalServerError)
		return
	}

	err := gd.RemoveNode(node, username, password)
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
