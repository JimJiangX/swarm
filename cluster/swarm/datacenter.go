package swarm

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/storage"
	"github.com/docker/swarm/scplib"
	"github.com/docker/swarm/utils"
	"github.com/pkg/errors"
)

// Datacenter containers database.Cluster,remote Store,[]*Node
type Datacenter struct {
	sync.RWMutex

	*database.Cluster

	store storage.Store

	nodes []*Node
}

// AddNewCluster returns a new database.Cluster
func AddNewCluster(req structs.PostClusterRequest) (database.Cluster, error) {
	if storage.IsLocalStore(req.StorageType) && req.StorageID != "" {
		req.StorageID = ""
	}
	if req.Type != _ProxyType && req.NetworkingID != "" {
		req.NetworkingID = ""
	}

	cluster := database.Cluster{
		ID:           utils.Generate64UUID(),
		Name:         req.Name,
		Type:         req.Type,
		StorageType:  req.StorageType,
		StorageID:    req.StorageID,
		NetworkingID: req.NetworkingID,
		Enabled:      true,
		MaxNode:      req.MaxNode,
		UsageLimit:   req.UsageLimit,
	}

	err := database.InsertCluster(cluster)
	if err != nil {
		return database.Cluster{}, err
	}

	return cluster, nil
}

// Node correspond a computer host
type Node struct {
	*database.Node
	task       *database.Task
	engine     *cluster.Engine
	localStore *storage.LocalStore
	hdd        []string
	ssd        []string
	user       string // os user
	password   string // os password
	port       int    // ssh port
}

// NewNodeWitTask returns *Node with *database.Task
func NewNodeWitTask(addr, name, cluster, user, password, room, seat string, hdd, ssd []string, port, num int) *Node {
	node := &database.Node{
		ID:        utils.Generate32UUID(),
		Name:      name,
		ClusterID: cluster,
		Addr:      addr,
		Room:      room,
		Seat:      seat,

		MaxContainer: num,
		Status:       statusNodeImport,
	}

	task := database.NewTask(node.Name, nodeInstallTask, node.ID, "import node", nil, 0)

	return &Node{
		Node:     node,
		task:     &task,
		user:     user,
		password: password,
		port:     port,
		hdd:      hdd,
		ssd:      ssd,
	}
}

// Task returns Node task
func (node *Node) Task() *database.Task {
	return node.task
}

// SaveMultiNodesToDB saves slcie of Node into database
func SaveMultiNodesToDB(nodes []*Node) error {
	list := make([]*database.Node, len(nodes))
	tasks := make([]*database.Task, len(nodes))

	for i := range nodes {
		list[i] = nodes[i].Node
		tasks[i] = nodes[i].task
	}

	return database.TxInsertMultiNodeAndTask(list, tasks)
}

// Datacenter returns Datacenter store in Gardener,if not found try in database and rebuild a Datacenter
func (gd *Gardener) Datacenter(nameOrID string) (*Datacenter, error) {
	gd.RLock()
	for i := range gd.datacenters {
		if gd.datacenters[i].ID == nameOrID || gd.datacenters[i].Name == nameOrID {
			gd.RUnlock()

			return gd.datacenters[i], nil
		}
	}

	gd.RUnlock()

	// if not found
	dc, err := gd.rebuildDatacenter(nameOrID)
	if err != nil {
		return nil, err
	}

	gd.Lock()
	gd.datacenters = append(gd.datacenters, dc)
	gd.Unlock()

	return dc, nil
}

// AddDatacenter add a Datacenter with Store
func (gd *Gardener) AddDatacenter(cl database.Cluster, store storage.Store) {
	dc := &Datacenter{
		RWMutex: sync.RWMutex{},
		Cluster: &cl,
		store:   store,
		nodes:   make([]*Node, 0, 100),
	}

	gd.Lock()
	gd.datacenters = append(gd.datacenters, dc)
	gd.Unlock()

	logrus.WithFields(logrus.Fields{
		"DC": cl.Name,
	}).Info("Datacenter Initializied")
}

// UpdateDatacenterParams update Datacenter settings
func (gd *Gardener) UpdateDatacenterParams(nameOrID string, max int, limit float32) error {
	dc, err := gd.Datacenter(nameOrID)
	if err != nil {
		logrus.WithError(err).WithField("DC", nameOrID).Error("get Datacenter")
		return err
	}

	modify := false
	dc.Lock()
	lately := *dc.Cluster

	if max > 0 && lately.MaxNode != max {
		lately.MaxNode = max
		modify = true
	}
	if limit > 0 && lately.UsageLimit != limit {
		lately.UsageLimit = limit
		modify = true
	}

	if modify {
		err := database.UpdateClusterParams(lately)
		if err != nil {
			dc.Unlock()
			logrus.WithError(err).WithField("DC", dc.Name).Error("update Datacenter params")

			return err
		}

		dc.Cluster = &lately
	}
	dc.Unlock()

	return nil
}

// SetStatus update Datacenter.Enabled
func (dc *Datacenter) SetStatus(enable bool) error {
	dc.Lock()
	err := database.UpdateClusterStatus(dc.Cluster, enable)
	dc.Unlock()

	return err
}

func (dc *Datacenter) isNodeExist(nameOrID string) bool {
	dc.RLock()

	for i := range dc.nodes {

		if dc.nodes[i].ID == nameOrID || dc.nodes[i].Name == nameOrID {
			dc.Unlock()
			return true
		}
	}

	dc.Unlock()

	return false
}

// GetNode get the assigned node in Datacenter
func (dc *Datacenter) GetNode(nameOrID string) (*Node, error) {
	if len(nameOrID) == 0 {
		return nil, errors.New("Node nameOrID is null")
	}

	dc.RLock()
	node := dc.getNode(nameOrID)
	dc.RUnlock()

	if node != nil {
		return node, nil
	}

	return nil, errors.New("not found node:" + nameOrID)
}

func (dc *Datacenter) getNode(nameOrID string) *Node {
	for i := range dc.nodes {
		if dc.nodes[i].ID == nameOrID ||
			dc.nodes[i].Name == nameOrID ||
			dc.nodes[i].EngineID == nameOrID {

			return dc.nodes[i]
		}
	}

	return nil
}

func (dc *Datacenter) listNodeID() []string {

	dc.RLock()
	out := make([]string, 0, len(dc.nodes))

	for i := range dc.nodes {
		out[i] = dc.nodes[i].ID
	}

	dc.RUnlock()

	if len(out) == 0 {

		var err error
		nodes, err := database.ListNodeByCluster(dc.ID)
		if err != nil {
			return nil
		}

		for i := range nodes {
			out = append(out, nodes[i].ID)
		}

	}

	return out
}

func (gd *Gardener) getNode(nameOrID string) (*Datacenter, *Node, error) {
	dc, err := gd.datacenterByNode(nameOrID)
	if dc != nil && err == nil {

		node, err := dc.GetNode(nameOrID)
		if node != nil {
			return dc, node, err
		}
	}

	n, err := database.GetNode(nameOrID)
	if err != nil {
		return dc, nil, err
	}

	node, err := gd.rebuildNode(n)
	if err != nil {
		return dc, node, err
	}

	if dc != nil {
		dc.Lock()
		dc.nodes = append(dc.nodes, node)
		dc.Unlock()
	}

	return dc, node, nil
}

func (gd *Gardener) datacenterByNode(nameOrID string) (*Datacenter, error) {
	node, err := database.GetNode(nameOrID)
	if err != nil {
		return nil, err
	}

	return gd.Datacenter(node.ClusterID)
}

func (gd *Gardener) datacenterByEngine(nameOrID string) (*Datacenter, error) {
	node, err := database.GetNode(nameOrID)
	if err != nil {
		return nil, err
	}

	return gd.Datacenter(node.ClusterID)
}

// SetNodeStatus update assigned Node status
func (gd *Gardener) SetNodeStatus(name string, state int64) error {
	_, node, err := gd.getNode(name)
	if err != nil {
		logrus.WithField("Node", name).Error(err)
	}
	if node == nil {
		return errors.New("not found Node:" + name)
	}

	if node.Status != statusNodeDisable &&
		node.Status != statusNodeEnable &&
		node.Status != statusNodeDeregisted {

		return errors.Errorf("Node %s status:%d,forbidding changing status to %d", name, node.Status, state)
	}

	return node.UpdateStatus(state)
}

// SetNodeParams update Node params
func (gd *Gardener) SetNodeParams(name string, max int) error {
	_, node, err := gd.getNode(name)
	if err != nil {
		return err
	}

	return node.UpdateParams(max)
}

func (dc *Datacenter) removeNode(nameOrID string) error {
	dc.Lock()
	for i := range dc.nodes {
		if dc.nodes[i].ID == nameOrID ||
			dc.nodes[i].Name == nameOrID ||
			dc.nodes[i].EngineID == nameOrID {

			dc.nodes = append(dc.nodes[:i], dc.nodes[i+1:]...)
			break
		}
	}

	dc.Unlock()

	return nil
}

// GetEngine returns the assigned Engine of Gardener
func (gd *Gardener) GetEngine(nameOrID string) (*cluster.Engine, error) {
	gd.RLock()
	eng, ok := gd.engines[nameOrID]

	if !ok {
		for _, engine := range gd.engines {
			if engine.ID == nameOrID ||
				engine.Name == nameOrID {
				eng = engine
				ok = true
				break
			}
		}
	}

	if !ok {
		for _, engine := range gd.pendingEngines {
			if engine.ID == nameOrID ||
				engine.Name == nameOrID {
				eng = engine
				ok = true
				break
			}
		}
	}
	gd.RUnlock()

	if eng != nil && ok {
		return eng, nil
	}

	return nil, errors.Errorf("not found engine '%s'", nameOrID)
}

// RemoveNode remove the assigned Node from the Gardener
func (gd *Gardener) RemoveNode(nameOrID, user, password string) (int, error) {
	node, err := database.GetNode(nameOrID)
	if err != nil {

		if errors.Cause(err) == database.ErrNoRowsFound {
			return 0, nil
		}

		return 500, err
	}

	eng, err := gd.GetEngine(node.EngineID)
	if err != nil {
		logrus.Warn(err)
	}

	if eng != nil {
		eng.RefreshContainers(false)
		if num := len(eng.Containers()); num != 0 {
			return 412, errors.Errorf("%d containers has created on Node %s", num, nameOrID)
		}

		gd.scheduler.Lock()
		for _, pending := range gd.pendingContainers {
			if pending.Engine.ID == node.EngineID {
				gd.scheduler.Unlock()

				return 412, errors.Errorf("containers has created on Node %s", nameOrID)
			}
		}
		gd.scheduler.Unlock()
	}

	count, err := database.CountUnitByNode(node.ID)
	if err != nil || count > 0 {
		return 412, errors.Errorf("count Unit by Node,%v,count:%d", err, count)
	}

	err = deregisterToHorus(false, node.ID)
	if err != nil {
		logrus.WithField("Endpoints", node.ID).Errorf("deregister Node to Horus:%+v", err)

		err = deregisterToHorus(true, node.ID)
		if err != nil {
			logrus.WithField("Endpoints", node.ID).Errorf("deregister Node to Horus,force=true,%s", err)
			return 503, err
		}
	}

	err = database.DeleteNode(nameOrID)
	if err != nil {
		return 500, err
	}

	gd.Lock()
	for i := range gd.datacenters {
		if gd.datacenters[i].ID == node.ClusterID {
			gd.datacenters[i].removeNode(nameOrID)
			break
		}
	}
	gd.Unlock()

	// ssh exec clean script
	err = nodeClean(node.ID, node.Addr, user, password)
	if err != nil {
		logrus.Errorf("clean script exec error:%s", err)
		return 510, err
	}

	return 0, nil
}

// RemoveDatacenter remove the assigned Datacenter from Gardener
func (gd *Gardener) RemoveDatacenter(nameOrID string) error {
	cl, err := database.GetCluster(nameOrID)
	if err != nil {
		return err
	}

	count, err := database.CountNodeByCluster(cl.ID)
	if err != nil {
		return err
	}

	if count > 0 {
		return errors.Errorf("%d Nodes in Cluster %s", count, nameOrID)
	}

	err = database.DeleteCluster(nameOrID)
	if err != nil {
		return err
	}

	gd.Lock()
	for i := range gd.datacenters {
		if gd.datacenters[i].ID == cl.ID {
			gd.datacenters = append(gd.datacenters[:i], gd.datacenters[i+1:]...)
			break
		}
	}
	gd.Unlock()

	return nil
}

func (dc *Datacenter) isIdleStoreEnough(num, size int) bool {
	dc.RLock()
	store := dc.store
	if store == nil {
		dc.RUnlock()
		return false
	}
	dc.RUnlock()

	idles, err := store.IdleSize()
	if err != nil {
		return false
	}

	enough := 0
	for i := range idles {
		enough += idles[i] / size
	}

	return enough > num
}

func (gd *Gardener) rebuildDatacenters() error {
	logrus.Debug("rebuild Datacenters")

	list, err := database.ListClusters()
	if err != nil {
		return err
	}
	gd.Lock()
	gd.datacenters = make([]*Datacenter, 0, len(list))
	gd.Unlock()

	for i := range list {
		dc, err := gd.rebuildDatacenter(list[i].ID)
		if err != nil {
			continue
		}

		gd.Lock()
		gd.datacenters = append(gd.datacenters, dc)
		gd.Unlock()
	}

	return nil
}

func (gd *Gardener) rebuildDatacenter(nameOrID string) (*Datacenter, error) {
	logrus.WithField("DC", nameOrID).Debug("rebuild Datacenter")

	cl, err := database.GetCluster(nameOrID)
	if err != nil {
		return nil, err
	}

	var store storage.Store
	if !storage.IsLocalStore(cl.StorageType) && cl.StorageID != "" {
		store, err = storage.GetStore(cl.StorageID)
		if err != nil {
			return nil, err
		}
	}

	dc := &Datacenter{
		RWMutex: sync.RWMutex{},
		Cluster: &cl,
		store:   store,
		nodes:   make([]*Node, 0, 100),
	}

	nodes, err := database.ListNodeByCluster(cl.ID)
	if err != nil {
		return dc, err
	}

	out := make([]*Node, 0, len(nodes))
	for n := range nodes {
		node, err := gd.rebuildNode(*nodes[n])
		if err != nil {
			continue
		}
		out = append(out, node)
	}

	dc.Lock()
	dc.nodes = append(dc.nodes, out...)
	dc.Unlock()

	return dc, nil
}

func (gd *Gardener) rebuildNode(n database.Node) (*Node, error) {
	entry := logrus.WithFields(logrus.Fields{
		"Name": n.Name,
		"addr": n.Addr,
	})

	eng, err := gd.GetEngine(n.EngineID)
	node := &Node{
		Node:   &n,
		engine: eng,
	}
	if err != nil {
		entry.WithError(err).Error("rebuild Node")

		return node, err
	}

	pluginAddr := fmt.Sprintf("%s:%d", eng.IP, pluginPort)
	node.localStore, err = storage.NewLocalDisk(pluginAddr, node.Node, 0)
	if err != nil {
		entry.WithError(err).Warn("rebuild Node with local Store error")
	}

	return node, nil
}

func getVGname(engine *cluster.Engine, _type string) (string, error) {
	if engine == nil || engine.Labels == nil {
		return "", errors.Wrap(errEngineIsNil, "get VG name")
	}

	parts := strings.SplitN(_type, ":", 2)
	if len(parts) == 1 {
		parts = append(parts, _HDD)
	}

	label := ""

	switch {
	case parts[1] == _HDD:
		label = _HDD_VG_Label
	case parts[1] == _SSD:
		label = _SSD_VG_Label
	case strings.ToUpper(parts[1]) == _HDD:
		label = _HDD_VG_Label
	case strings.ToUpper(parts[1]) == _SSD:
		label = _SSD_VG_Label

	default:
		return "", errors.Errorf("unable get VG Type '%s' VG_Label in Engine %s", parts[1], engine.Addr)
	}

	engine.RLock()
	vgName, ok := engine.Labels[label]
	engine.RUnlock()

	if !ok {
		return "", errors.Errorf("not found VG_Name '%s' of Node:'%s'", _type, engine.Addr)
	}

	return vgName, nil
}

type vgUsage struct {
	Name  string
	Total int
	Used  int
}

// GetLocalVGUsage returns the Engine local volumes infomation
func GetLocalVGUsage(engine *cluster.Engine) map[string]vgUsage {
	if engine == nil || engine.Labels == nil {
		return map[string]vgUsage{}
	}

	out := make(map[string]vgUsage, 2)

	// HDD
	engine.RLock()
	hdd, ok := engine.Labels[_HDD_VG_Size_Label]
	engine.RUnlock()

	if ok {
		total, err := strconv.Atoi(hdd)
		if err == nil {
			engine.RLock()
			vgName, ok := engine.Labels[_HDD_VG_Label]
			engine.RUnlock()

			if ok {
				used, err := storage.GetVGUsedSize(vgName)
				if err == nil {
					out[_HDD] = vgUsage{
						Name:  vgName,
						Total: total,
						Used:  used,
					}
				}
			}
		}
	}

	// SSD
	engine.RLock()
	ssd, ok := engine.Labels[_SSD_VG_Size_Label]
	engine.RUnlock()

	if ok {
		total, err := strconv.Atoi(ssd)
		if err == nil {
			engine.RLock()
			vgName, ok := engine.Labels[_SSD_VG_Label]
			engine.RUnlock()

			if ok {
				used, err := storage.GetVGUsedSize(vgName)
				if err == nil {
					out[_SSD] = vgUsage{
						Name:  vgName,
						Total: total,
						Used:  used,
					}
				}
			}
		}
	}

	return out
}

func (gd *Gardener) resourceFilter(list []database.Node, module structs.Module, num int) ([]database.Node, error) {
	logrus.Debugf("resourceFilter:nodes=%d,Type='%s',num=%d", len(list), module.Type, num)

	ncpu, err := parseCpuset(module.HostConfig.CpusetCpus)
	if err != nil {
		return nil, err
	}

	gd.RLock()
	length := len(gd.datacenters)
	gd.RUnlock()

	if length == 0 {
		err := gd.rebuildDatacenters()
		if err != nil {
			return nil, err
		}
	}

	out := make([]database.Node, 0, 100)

loop:
	for i := range list {
		dc, node, err := gd.getNode(list[i].ID)
		if err != nil || dc == nil || node == nil {
			logrus.Warn("not found Node by ID:" + list[i].ID)

			continue loop
		}

		dc.Lock()

		if !dc.Enabled {
			logrus.Debug(dc.Name + ":DC disabled")

			dc.Unlock()

			continue loop
		}

		if node.engine == nil || !node.engine.IsHealthy() {
			dc.Unlock()

			continue loop
		}

		if containers := node.engine.Containers(); len(containers) >= node.MaxContainer ||
			node.engine.TotalCpus()-node.engine.UsedCpus() < int64(ncpu) ||
			node.engine.TotalMemory()-node.engine.UsedMemory() < module.HostConfig.Memory {

			dc.Unlock()
			continue loop

		} else {
			list := make([]string, 0, len(gd.pendingContainers))
			usedMemory := int64(0)

			for _, pending := range gd.pendingContainers {
				if pending.Engine.ID == node.engine.ID {
					list = append(list, pending.Config.HostConfig.CpusetCpus)
					usedMemory += pending.Config.HostConfig.Memory
				}
			}

			usedCPUs := parseUintList(list)

			if node.engine.TotalCpus()-node.engine.UsedCpus()-int64(len(usedCPUs)) < int64(ncpu) ||
				node.engine.TotalMemory()-node.engine.UsedMemory()-usedMemory < module.HostConfig.Memory {

				dc.Unlock()
				continue loop
			}
		}

		dc.Unlock()

		for _, v := range module.Stores {

			if storage.IsLocalStore(v.Type) {

				if !node.isIdleStoreEnough(v.Type, v.Size) {
					logrus.Debugf("%s local store shortage:%d", node.Name, v.Size)

					continue loop
				}

			} else if v.Type == storage.SANStore {
				// when storage is HITACHI or HUAWEI
				if !dc.isIdleStoreEnough(num/2, v.Size) {
					logrus.Debugf("%s san store shortage:%d", dc.Name, v.Size)

					continue loop
				}
			}
		}

		out = append(out, list[i])
	}

	return out, nil
}

func (node *Node) isIdleStoreEnough(_type string, size int) bool {
	if !storage.IsLocalStore(_type) {

		return false
	}

	if node.localStore == nil {
		logrus.Debugf("%s Local Store is nil", node.Name)

		return false
	}

	idle, err := node.localStore.IdleSize()
	if err != nil {
		logrus.Debugf("%s Local Store error:%s", node.Name, err)

		return false
	}

	vgName, err := getVGname(node.engine, _type)
	if err != nil {
		logrus.Debugf("%s get VG_Name error:%s", node.Name, err)

		return false
	}

	if idle[vgName] < size {
		logrus.Debugf("%s VG %s shortage:%v %d", node.Name, vgName, idle[vgName], size)

		return false
	}

	return true
}

// DeregisterNode deregister the assigned Node
func (dc *Datacenter) DeregisterNode(nameOrID string) error {
	dc.RLock()

	node := dc.getNode(nameOrID)

	dc.RUnlock()

	if node == nil {
		node = &Node{
			Node: &database.Node{
				ID: nameOrID,
			},
		}
	}

	dc.Lock()

	err := node.UpdateStatus(statusNodeDeregisted)

	if node.engine != nil {
		node.engine.Disconnect()
	}

	dc.Unlock()

	logrus.Infof("Deregister Node:%s of Cluster:%s", nameOrID, dc.Name)

	return err
}

// DistributeNode distribute,install and start agents on the remote host
func (dc *Datacenter) DistributeNode(node *Node) error {
	entry := logrus.WithFields(logrus.Fields{
		"Node":    node.Name,
		"addr":    node.Addr,
		"Cluster": dc.Cluster.ID,
	})
	err := database.TxUpdateNodeStatus(node.Node, node.task,
		statusNodeInstalling, statusTaskRunning, "")
	if err != nil {
		entry.Error(err)

		return err
	}

	entry.Info("Adding new Node")

	if err := node.distribute(); err != nil {
		entry.WithError(err).Error("SSH UploadDir")

		return err
	}

	dc.Lock()

	dc.nodes = append(dc.nodes, node)

	dc.Unlock()

	entry.Info("Node initialized")

	return err
}

// CA,script,error
func (node Node) modifyProfile() (*database.Configurations, string, error) {
	config, err := database.GetSystemConfig()
	if err != nil {
		return nil, "", err
	}

	horus, err := getHorusFromConsul()
	if err != nil {
		return nil, "", err
	}

	horusIP, horusPort, err := net.SplitHostPort(horus)
	if err != nil {
		return nil, "", errors.Wrap(err, "Horus addr:"+horus)
	}

	sourceDir, err := utils.GetAbsolutePath(true, config.SourceDir)
	if err != nil {
		return nil, "", errors.Wrap(err, "get sourceDir:"+config.SourceDir)
	}

	config.SourceDir = sourceDir
	path, caFile, _ := config.DestPath()

	buf, err := json.Marshal(config.GetConsulAddrs())
	if err != nil {
		return nil, "", errors.Wrap(err, "JSON marshal consul addrs")
	}
	/*
		#!/bin/bash
		swarm_key=$1
		adm_ip=$2
		cs_datacenter=$3
		cs_list=$4
		registry_domain=$5
		registry_ip=$6
		registry_port=$7
		registry_username=$8
		registry_passwd=$9
		regstry_ca_file=${10}
		docker_port=${11}
		hdd_dev=${12}
		ssd_dev=${13}
		horus_agent_port=${14}
		consul_port=${15}
		node_id=${16}
		horus_server_ip=${17}
		horus_server_port=${18}
		docker_plugin_port=${19}
		nfs_ip=${20}
		nfs_dir=${21}
		nfs_mount_dir=${22}
		nfs_mount_opts=${23}
		cur_dir=`dirname $0`

		hdd_vgname=${HOSTNAME}_HDD_VG
		ssd_vgname=${HOSTNAME}_SSD_VG

		adm_nic=bond0
		int_nic=bond1
		ext_nic=bond2
	*/
	hdd, ssd := "null", "null"
	if len(node.hdd) > 0 {
		hdd = strings.Join(node.hdd, ",")
	}
	if len(node.ssd) > 0 {
		ssd = strings.Join(node.ssd, ",")
	}

	script := fmt.Sprintf("chmod 755 %s && %s %s %s %s '%s' %s %s %d %s %s %s %d %s %s %d %d %s %s %s %d %s %s %s %s",
		path, path, dockerNodesKVPath, node.Addr, config.ConsulDatacenter, string(buf),
		config.Registry.Domain, config.Registry.Address, config.Registry.Port,
		config.Registry.Username, config.Registry.Password, caFile,
		config.DockerPort, hdd, ssd, config.HorusAgentPort, config.ConsulPort,
		node.ID, horusIP, horusPort, config.PluginPort,
		config.NFSOption.Addr, config.NFSOption.Dir, config.MountDir, config.MountOptions)

	return config, script, nil
}

func (node *Node) distribute() (err error) {
	entry := logrus.WithFields(logrus.Fields{
		"Node": node.Name,
		"host": node.Addr,
		"user": node.user,
	})

	nodeState, taskState := int64(statusNodeInstalling), int64(statusTaskRunning)

	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("Recover from Panic:%v", r)
		}

		var msg string
		if err == nil {
			nodeState = statusNodeInstalled
		} else {
			if nodeState == statusNodeInstalling {
				nodeState = statusNodeInstallFailed
			}
			taskState = statusTaskFailed
			msg = err.Error()
		}

		r := database.TxUpdateNodeStatus(node.Node, node.task,
			nodeState, taskState, msg)
		if r != nil || err != nil {
			entry.Error(msg, r)
		}
	}()

	config, script, err := node.modifyProfile()
	if err != nil {
		entry.WithError(err).Error("modify profile")

		return err
	}

	entry = entry.WithFields(logrus.Fields{
		"source":      config.SourceDir,
		"destination": config.Destination,
	})

	c, err := scplib.NewClient(node.Addr, node.user, node.password)
	if err != nil {
		nodeState = statusNodeSSHLoginFailed
		return err
	}
	defer c.Close()

	if err := c.UploadDir(config.Destination, config.SourceDir); err != nil {
		entry.WithError(err).Error("SSH upload dir:" + config.SourceDir)

		if err := c.UploadDir(config.Destination, config.SourceDir); err != nil {
			entry.WithError(err).Error("SSH upload dir twice:" + config.SourceDir)

			nodeState = statusNodeSCPFailed
			return err
		}
	}

	logrus.Infof("Registry.CA_CRT:%d %s", len(config.Registry.CA_CRT), config.Registry.CA_CRT)

	_, filename, _ := config.DestPath()

	if err := c.Upload(config.Registry.CA_CRT, filename, 0644); err != nil {
		entry.WithError(err).Error("SSH upload file:" + filename)

		if err := c.Upload(config.Registry.CA_CRT, filename, 0644); err != nil {
			entry.WithError(err).Error("SSH upload file twice:" + filename)

			nodeState = statusNodeSCPFailed
			return err
		}
	}

	out, err := c.Exec(script)
	if err != nil {
		entry.WithError(err).Errorf("exec remote command:'%s',output:%s", script, out)

		if out, err = c.Exec(script); err != nil {
			entry.WithError(err).Errorf("exec remote command twice:'%s',output:%s", script, out)

			nodeState = statusNodeSSHExecFailed
			return err
		}
	}

	entry.Infof("SSH remote PKG install successed! output:\n%s", out)

	return nil
}

// RegisterNodes register Nodes
func (gd *Gardener) RegisterNodes(name string, nodes []*Node, timeout time.Duration) error {
	entry := logrus.WithField("DC", name)

	dc, err := gd.Datacenter(name)
	if err != nil || dc == nil {
		entry.WithError(err).Error("not found Dataceneter")

		return err
	}

	config, err := gd.systemConfig()
	if err != nil {
		entry.WithError(err).Error("Gardener systemConfig")

		return err
	}

	deadline := time.Now().Add(timeout)

	for {
		if time.Now().After(deadline) {
			err := dealWithTimeout(nodes)

			return errors.Errorf("Node register timeout %ds,%v", timeout, err)
		}

		time.Sleep(30 * time.Second)

		for i := range nodes {
			_entry := entry.WithFields(logrus.Fields{
				"Node": nodes[i].Name,
				"addr": nodes[i].Addr,
			})
			if nodes[i].Status != statusNodeInstalled {
				_entry.Warnf("status not match,%d!=%d", nodes[i].Status, statusNodeInstalled)
				continue
			}

			eng, err := gd.updateNodeEngine(nodes[i], config.DockerPort)
			if err != nil || eng == nil {
				_entry.Error(err)

				continue
			}

			err = initNodeStores(dc, nodes[i], eng)
			if err != nil {
				_entry.Error(err)
				continue
			}

			err = database.TxUpdateNodeRegister(nodes[i].Node, nodes[i].task, statusNodeEnable, statusTaskDone, eng.ID, "")
			if err != nil {
				_entry.WithError(err).Error("Node register")

				continue
			}

			nodes[i].engine = eng
			nodes[i].EngineID = eng.ID
		}
	}
}

func dealWithTimeout(nodes []*Node) error {
	for i := range nodes {
		if nodes[i].Status >= statusNodeEnable {
			continue
		}

		if nodes[i].Status != statusNodeInstalled {
			nodes[i].Status = statusNodeInstallFailed
		}

		err := database.TxUpdateNodeRegister(nodes[i].Node, nodes[i].task, nodes[i].Status, statusTaskTimeout, "", "Node Register Timeout")
		if err != nil {
			logrus.WithField("Node", nodes[i].Name).WithError(err).Error("Node register timeout")
		}
	}

	return nil
}

func (gd *Gardener) updateNodeEngine(node *Node, dockerPort int) (*cluster.Engine, error) {
	addr := fmt.Sprintf("%s:%d", node.Addr, dockerPort)
	eng := gd.getEngineByAddr(addr)

	if status := ""; eng == nil || !eng.IsHealthy() {
		if eng != nil {
			status = eng.Status()
		} else {
			status = "engine is nil"
		}

		return nil, errors.Errorf("engine %s status:%s", addr, status)
	}

	return eng, nil
}

func initNodeStores(dc *Datacenter, node *Node, eng *cluster.Engine) error {
	pluginAddr := fmt.Sprintf("%s:%d", eng.IP, pluginPort)

	localStore, err := storage.NewLocalDisk(pluginAddr, node.Node, 0)
	if err != nil {
		logrus.WithField("Node", node.Name).WithError(err).Warn("init node local store")
	}

	node.localStore = localStore

	wwn := eng.Labels[_SAN_HBA_WWN_Lable]
	if strings.TrimSpace(wwn) != "" {

		if dc.store == nil || dc.store.Driver() != storage.SANStoreDriver {
			return nil
		}

		list := strings.Split(wwn, ",")

		err := dc.store.AddHost(node.ID, list...)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"Node":   node.Name,
				"Store":  dc.store.ID(),
				"Vendor": dc.store.Vendor(),
			}).WithError(err).Warn("add node to store")
		}
	}

	return err
}

func nodeClean(node, addr, user, password string) error {
	config, err := database.GetSystemConfig()
	if err != nil {
		return err
	}

	horus, err := getHorusFromConsul()
	if err != nil {
		return err
	}

	horusIP, horusPort, err := net.SplitHostPort(horus)
	if err != nil {
		return errors.Wrap(err, "check Horus Addr:"+horus)
	}

	_, _, destName := config.DestPath()

	srcFile, err := utils.GetAbsolutePath(false, config.SourceDir, config.CleanScriptName)
	if err != nil {
		logrus.Errorf("%s %s", srcFile, err)

		return errors.Wrap(err, "get absolute path")
	}

	entry := logrus.WithFields(logrus.Fields{
		"host":        addr,
		"user":        user,
		"source":      srcFile,
		"destination": destName,
	})

	c, err := scplib.NewClient(addr, user, password)
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.UploadFile(destName, srcFile); err != nil {
		entry.Errorf("SSH UploadFile %s Error,%s", srcFile, err)

		if err := c.UploadFile(destName, srcFile); err != nil {
			entry.Errorf("SSH UploadFile %s Error Twice,%s", srcFile, err)

			return err
		}
	}

	/*
		adm_ip=$1
		consul_port=${2}
		node_id=${3}
		horus_server_ip=${4}
		horus_server_port=${5}
		backup_dir = ${6}
	*/

	script := fmt.Sprintf("chmod 755 %s && %s %s %d %s %s %s %s",
		destName, destName, addr, config.ConsulPort, node,
		horusIP, horusPort, config.NFSOption.MountDir)

	out, err := c.Exec(script)
	if err != nil {
		entry.Errorf("exec remote command: %s,%v,Output:%s", script, err, out)

		out, err := c.Exec(script)
		if err != nil {
			entry.Errorf("exec remote command twice: %s,%v,Output:%s", script, err, out)
			return err
		}
	}

	entry.Infof("SSH Remote Exec Successed! Output:\n%s", out)

	return nil
}
