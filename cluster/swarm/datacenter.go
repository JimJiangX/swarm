package swarm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/store"
	"github.com/docker/swarm/utils"
	"github.com/hashicorp/terraform/communicator/remote"
	"github.com/hashicorp/terraform/communicator/ssh"
	"github.com/hashicorp/terraform/terraform"
)

type Datacenter struct {
	sync.RWMutex

	*database.Cluster

	storage store.Store

	nodes []*Node
}

func ValidDatacenter(req structs.PostClusterRequest) string {
	warnings := make([]string, 0, 5)
	if req.Name == "" {
		warnings = append(warnings, "'name' is null")
	}

	if !isStringExist(req.StorageType, supportedStoreTypes) {
		warnings = append(warnings, fmt.Sprintf("Unsupported '%s' Yet", req.StorageType))
	}

	if req.StorageType != store.LocalDiskStore && req.StorageID == "" {
		warnings = append(warnings, "missing 'StorageID' when 'StorageType' isnot 'local'")
	}

	if req.Datacenter == "" {
		warnings = append(warnings, "'dc' is null")
	}

	if len(warnings) == 0 {
		return ""
	}

	return strings.Join(warnings, ",")
}

func AddNewCluster(req structs.PostClusterRequest) (database.Cluster, error) {
	if req.StorageType == store.LocalDiskStore {
		req.StorageID = ""
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

	err := cluster.Insert()
	if err != nil {
		return database.Cluster{}, err
	}

	return cluster, nil
}

type Node struct {
	*database.Node
	task       *database.Task
	engine     *cluster.Engine
	localStore store.Store
	hdd        []string
	ssd        []string
	user       string // os user
	password   string // os password
	port       int    // ssh port
}

func NewNode(addr, name, cluster, user, password, room, seat string, hdd, ssd []string, port, num int) *Node {
	node := &database.Node{
		ID:        utils.Generate64UUID(),
		Name:      name,
		ClusterID: cluster,
		Addr:      addr,
		Room:      room,
		Seat:      seat,

		MaxContainer: num,
		Status:       _StatusNodeImport,
	}

	task := database.NewTask("node", node.ID, "import node", nil, 0)

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

func (node *Node) Task() *database.Task {
	return node.task
}

func (node *Node) Insert() error {
	err := database.TxInsertNodeAndTask(*node.Node, *node.task)
	if err != nil {
		logrus.Errorf("Node:%s Insert INTO DB Error,%s", node.Name, err)
	}

	return err
}

func SaveMultiNodesToDB(nodes []*Node) error {
	list := make([]*database.Node, len(nodes))
	tasks := make([]*database.Task, len(nodes))

	for i := range nodes {
		list[i] = nodes[i].Node
		tasks[i] = nodes[i].task
	}

	return database.TxInsertMultiNodeAndTask(list, tasks)
}

func (gd *Gardener) Datacenter(IDOrName string) (*Datacenter, error) {
	gd.RLock()
	for i := range gd.datacenters {
		if gd.datacenters[i].ID == IDOrName || gd.datacenters[i].Name == IDOrName {
			gd.RUnlock()

			return gd.datacenters[i], nil
		}
	}

	gd.RUnlock()

	//If Not Found
	dc, err := gd.rebuildDatacenter(IDOrName)
	if err != nil {
		return nil, err
	}

	gd.Lock()
	gd.datacenters = append(gd.datacenters, dc)
	gd.Unlock()

	return dc, nil
}

func (gd *Gardener) AddDatacenter(cl database.Cluster, storage store.Store) error {
	if cl.ID == "" {
		cl.ID = gd.generateUniqueID()
	}

	logrus.WithFields(logrus.Fields{
		"dc": cl.Name,
	}).Info("Datacenter Initializing")

	dc := &Datacenter{
		RWMutex: sync.RWMutex{},
		Cluster: &cl,
		storage: storage,
		nodes:   make([]*Node, 0, 100),
	}

	gd.Lock()
	gd.datacenters = append(gd.datacenters, dc)
	gd.Unlock()

	logrus.Infof("Datacenter Initialized:%s", dc.Name)

	return nil
}

func (gd *Gardener) UpdateDatacenterParams(NameOrID string, max int, limit float32) error {
	dc, err := gd.Datacenter(NameOrID)
	if err != nil {
		return err
	}
	modify := false
	dc.Lock()
	old := *dc.Cluster

	if max > 0 && old.MaxNode != max {
		old.MaxNode = max
		modify = true
	}
	if limit > 0 && old.UsageLimit != limit {
		old.UsageLimit = limit
		modify = true
	}

	if modify {
		err := old.UpdateParams()
		if err != nil {
			dc.Unlock()
			return err
		}

		dc.Cluster = &old
	}
	dc.Unlock()

	return nil
}

func (dc *Datacenter) SetStatus(enable bool) error {
	dc.Lock()
	err := dc.UpdateStatus(enable)
	dc.Unlock()

	return err
}

func (dc *Datacenter) ListNode() []*Node {
	dc.RLock()
	nodes := dc.nodes
	dc.RUnlock()

	return nodes
}

func (dc *Datacenter) isNodeExist(IDOrName string) bool {
	dc.RLock()

	for i := range dc.nodes {

		if dc.nodes[i].ID == IDOrName || dc.nodes[i].Name == IDOrName {
			dc.Unlock()
			return true
		}
	}

	dc.Unlock()

	return false
}

func (dc *Datacenter) GetNode(IDOrName string) (*Node, error) {
	if len(IDOrName) == 0 {
		return nil, errors.New("Not Found Node")
	}

	dc.RLock()

	node := dc.getNode(IDOrName)

	dc.RUnlock()

	if node != nil {

		return node, nil
	}

	return nil, errors.New("Not Found Node")
}

func (dc *Datacenter) getNode(NameOrID string) *Node {
	for i := range dc.nodes {
		if dc.nodes[i].ID == NameOrID ||
			dc.nodes[i].Name == NameOrID ||
			dc.nodes[i].EngineID == NameOrID {

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

func (gd *Gardener) GetNode(NameOrID string) (*Node, error) {
	dc, err := gd.DatacenterByNode(NameOrID)
	if dc != nil && err == nil {

		node, err := dc.GetNode(NameOrID)
		if node != nil && err == nil {
			return node, nil
		}
	}

	n, err := database.GetNode(NameOrID)
	if err != nil {
		return nil, err
	}

	node, err := gd.rebuildNode(n)
	if err != nil {
		return nil, err
	}

	if dc != nil {
		dc.Lock()
		dc.nodes = append(dc.nodes, node)
		dc.Unlock()
	}

	return node, nil
}

func (gd *Gardener) DatacenterByNode(IDOrName string) (*Datacenter, error) {
	node, err := database.GetNode(IDOrName)
	if err != nil {
		return nil, err
	}

	return gd.Datacenter(node.ClusterID)
}

func (gd *Gardener) DatacenterByEngine(IDOrName string) (*Datacenter, error) {
	node, err := database.GetNode(IDOrName)
	if err != nil {
		return nil, err
	}

	return gd.Datacenter(node.ClusterID)
}

func (gd *Gardener) SetNodeStatus(name string, state int) error {
	node, err := gd.GetNode(name)
	if err != nil {
		return err
	}

	if node.Status != _StatusNodeDisable &&
		node.Status != _StatusNodeEnable &&
		node.Status != _StatusNodeDeregisted {

		return fmt.Errorf("Node %s Status:%d,Forbidding Changing Status to %d", name, node.Status, state)
	}

	return node.UpdateStatus(state)
}

func (gd *Gardener) SetNodeParams(name string, max int) error {
	node, err := gd.GetNode(name)
	if err != nil {
		return err
	}

	return node.UpdateParams(max)
}

func (dc *Datacenter) RemoveNode(NameOrID string) error {
	dc.Lock()
	for i := range dc.nodes {
		if dc.nodes[i].ID == NameOrID ||
			dc.nodes[i].Name == NameOrID ||
			dc.nodes[i].EngineID == NameOrID {

			dc.nodes = append(dc.nodes[:i], dc.nodes[i+1:]...)
			break
		}
	}

	dc.Unlock()

	return nil
}

func (gd *Gardener) GetEngine(NameOrID string) (*cluster.Engine, error) {
	gd.RLock()
	eng, ok := gd.engines[NameOrID]

	if !ok {
		for _, engine := range gd.engines {
			if engine.ID == NameOrID ||
				engine.Name == NameOrID {
				eng = engine
				ok = true
				break
			}
		}
	}

	if !ok {
		for _, engine := range gd.pendingEngines {
			if engine.ID == NameOrID ||
				engine.Name == NameOrID {
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

	return nil, fmt.Errorf("Not Found Engine %s", NameOrID)
}

func (gd *Gardener) RemoveNode(NameOrID string) error {
	node, err := database.GetNode(NameOrID)
	if err != nil {
		return err
	}

	eng, err := gd.GetEngine(node.EngineID)
	if err != nil {
		logrus.Warn(err)
	}
	if eng != nil {
		eng.RefreshContainers(false)
		if num := len(eng.Containers()); num != 0 {
			return fmt.Errorf("%d Containers Has Created On Node %s", num, NameOrID)
		}

		gd.scheduler.Lock()
		for _, pending := range gd.pendingContainers {
			if pending.Engine.ID == node.EngineID {
				gd.scheduler.Unlock()

				return fmt.Errorf("Containers Has Created On Node %s", NameOrID)
			}
		}
		gd.scheduler.Unlock()
	}

	count, err := database.CountUnitByNode(node.ID)
	if err != nil || count != 0 {
		return fmt.Errorf("Count Unit ByNode,%v,count:%d", err, count)
	}

	dc, err := gd.DatacenterByNode(NameOrID)
	if err != nil {
		return err
	}

	err = database.DeleteNode(NameOrID)
	if err != nil {
		return err
	}

	return dc.RemoveNode(NameOrID)
}

func (gd *Gardener) RemoveDatacenter(NameOrID string) error {
	cl, err := database.GetCluster(NameOrID)
	if err != nil {
		return err
	}
	count, err := database.CountNodeByCluster(cl.ID)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%d Nodes In Cluster %s", count, NameOrID)
	}

	err = database.DeleteCluster(NameOrID)
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

func (dc *Datacenter) isIdleStoreEnough(IDOrType string, num, size int) bool {
	dc.RLock()

	store := dc.getStore(IDOrType)
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

	return enough >= num
}

func (dc *Datacenter) getStore(IDOrType string) store.Store {
	if IDOrType == "" {
		return dc.storage
	}

	if dc.storage != nil {
		if IDOrType == dc.storage.Vendor() ||
			IDOrType == dc.storage.ID() ||
			IDOrType == dc.storage.Driver() {

			return dc.storage
		}
	}

	return nil
}

func (gd *Gardener) rebuildDatacenters() error {
	list, err := database.ListCluster()
	if err != nil {
		return err
	}

	for i := range list {
		dc, err := gd.rebuildDatacenter(list[i].ID)
		if err != nil {
			continue
		}
		nodes, err := database.ListNodeByCluster(list[i].ID)
		if err != nil {
			continue
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

		gd.Lock()
		gd.datacenters = append(gd.datacenters, dc)
		gd.Unlock()
	}

	return nil
}

func (gd *Gardener) rebuildDatacenter(NameOrID string) (*Datacenter, error) {
	cl, err := database.GetCluster(NameOrID)
	if err != nil || cl == nil {
		return nil, fmt.Errorf("Not Found %s,Error %s", NameOrID, err)
	}

	var storage store.Store
	if cl.StorageType != store.LocalDiskStore && cl.StorageID != "" {
		storage, err = gd.GetStore(cl.StorageID)
		if err != nil {
			return nil, err
		}
	}

	dc := &Datacenter{
		RWMutex: sync.RWMutex{},
		Cluster: cl,
		storage: storage,
		nodes:   make([]*Node, 0, 100),
	}

	return dc, nil
}

func (gd *Gardener) rebuildNode(n database.Node) (*Node, error) {
	eng, err := gd.GetEngine(n.EngineID)
	if err != nil {
		return nil, err
	}

	node := &Node{
		Node:   &n,
		engine: eng,
	}

	vgs := make([]store.VG, 0, 2)
	//SSD
	if ssd := eng.Labels[_SSD_VG_Label]; ssd != "" {
		vgs = append(vgs, store.VG{
			Vendor: _SSD,
			Name:   ssd,
		})
	}
	// HDD
	if hdd := eng.Labels[_HDD_VG_Label]; hdd != "" {
		vgs = append(vgs, store.VG{
			Vendor: _HDD,
			Name:   hdd,
		})
	}

	pluginAddr := fmt.Sprintf("%s:%d", eng.IP, pluginPort)
	node.localStore = store.NewLocalDisk(pluginAddr, node.Node, vgs)

	return node, nil
}

func (gd *Gardener) listShortIdleStore(volumes []structs.DiskStorage, IDOrType string, num int) []string {
	gd.RLock()
	length := len(gd.datacenters)
	gd.RUnlock()

	if length == 0 {
		err := gd.rebuildDatacenters()
		if err != nil {
			return nil
		}
	}

	gd.RLock()
	defer gd.RUnlock()
	out := make([]string, 0, 100)
	for _, dc := range gd.datacenters {
		// check dc
		if dc == nil {
			continue
		}

		if !dc.Enabled {
			out = append(out, dc.ID)
			continue
		}

		if IDOrType != "" && !(dc.Type == IDOrType || dc.ID == IDOrType) {
			out = append(out, dc.ID)
			continue
		}

		for _, node := range dc.nodes {
			if node.engine == nil ||
				(node.engine != nil &&
					(!node.engine.IsHealthy() ||
						len(node.engine.Containers()) >= node.MaxContainer)) {

				out = append(out, node.ID)
				continue
			}
		}

		for _, v := range volumes {
			// when storage is HITACHI or HUAWEI
			if v.Type == store.SANStore {
				if !dc.isIdleStoreEnough("", num/2, v.Size) {
					out = append(out, dc.ID)
				}
			}

			if !strings.Contains(v.Type, store.LocalDiskStore) {
				continue
			}
			for _, node := range dc.nodes {
				if node.localStore == nil {
					out = append(out, node.ID)
					continue
				}

				idle, err := node.localStore.IdleSize()
				if err != nil {
					out = append(out, node.ID)
					continue
				}

				if idle[v.Type] < v.Size {
					out = append(out, node.ID)
					continue
				}
			}
		}
	}

	return out
}

func (dc *Datacenter) DeregisterNode(IDOrName string) error {
	dc.RLock()

	node := dc.getNode(IDOrName)

	dc.RUnlock()

	if node == nil {
		node = &Node{
			Node: &database.Node{
				ID: IDOrName,
			},
		}
	}

	dc.Lock()

	err := node.UpdateStatus(_StatusNodeDeregisted)

	if node.engine != nil {
		node.engine.Disconnect()
	}

	dc.Unlock()

	logrus.Infof("Deregister Node:%s of Cluster:%s", IDOrName, dc.Name)

	return err
}

func (dc *Datacenter) DistributeNode(node *Node, kvpath string) error {
	entry := logrus.WithFields(logrus.Fields{
		"name":    node.Name,
		"addr":    node.Addr,
		"cluster": dc.Cluster.ID,
	})
	err := database.TxUpdateNodeStatus(node.Node, node.task,
		_StatusNodeInstalling, _StatusTaskRunning, "")
	if err != nil {
		entry.Error(err)
		return err
	}

	entry.Info("Adding new Node")

	if err := node.Distribute(kvpath); err != nil {
		entry.Errorf("SSH UploadDir Error,%s", err)

		return err
	}

	dc.Lock()

	dc.nodes = append(dc.nodes, node)

	dc.Unlock()

	entry.Info("Init Node done")

	return err
}

// CA,script,error
func (node Node) modifyProfile(kvpath string) (*database.Configurations, string, error) {
	config, err := database.GetSystemConfig()
	if err != nil {
		return nil, "", err
	}

	config.SourceDir, err = utils.GetAbsolutePath(true, config.SourceDir)
	if err != nil {
		return nil, "", err
	}

	_, path, caFile := config.DestPath()

	buf, err := json.Marshal(config.GetConsulAddrs())
	if err != nil {
		return nil, "", err
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

	script := fmt.Sprintf("chmod 755 %s && %s %s %s %s '%s' %s %s %d %s %s %s %d %s %s %d %d %s %s %d",
		path, path, kvpath, node.Addr, config.ConsulDatacenter, string(buf),
		config.Registry.Domain, config.Registry.Address, config.Registry.Port,
		config.Registry.Username, config.Registry.Password, caFile,
		config.DockerPort, hdd, ssd,
		config.HorusAgentPort, config.ConsulPort, node.ID, config.HorusServerIP, config.HorusServerPort)

	return config, script, nil
}

func (node *Node) Distribute(kvpath string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Recover From Panic:%v", r)
		}

		nodeState, taskState, msg := 0, 0, ""
		if err == nil {
			nodeState = _StatusNodeInstalled
		} else {
			nodeState = _StatusNodeInstallFailed
			taskState = _StatusTaskFailed
			msg = err.Error()
		}

		r := database.TxUpdateNodeStatus(node.Node, node.task,
			nodeState, taskState, msg)
		if r != nil || err != nil {
			logrus.Error(err, r)
		}
	}()

	r := &terraform.InstanceState{
		Ephemeral: terraform.EphemeralState{
			ConnInfo: map[string]string{
				"type":     "ssh",
				"user":     node.user,
				"password": node.password,
				"host":     node.Addr,
				"port":     strconv.Itoa(node.port),
			},
		},
	}

	config, script, err := node.modifyProfile(kvpath)
	if err != nil {
		logrus.Error(err)

		return err
	}

	entry := logrus.WithFields(logrus.Fields{
		"host":        node.Addr,
		"user":        node.user,
		"source":      config.SourceDir,
		"destination": config.Destination,
	})

	c, err := ssh.New(r)
	if err != nil {
		entry.Errorf("error creating communicator: %s", err)

		return err
	}
	err = c.Connect(nil)
	if err != nil {
		logrus.Errorf("communicator connection error: %s", err)

		return err
	}
	defer c.Disconnect()

	if err := c.UploadDir(config.Destination, config.SourceDir); err != nil {
		entry.Errorf("SSH UploadDir Error,%s", err)

		if err := c.UploadDir(config.Destination, config.SourceDir); err != nil {
			entry.Errorf("SSH UploadDir Error Twice,%s", err)

			return err
		}
	}

	logrus.Info("Registry.CA_CRT", len(config.Registry.CA_CRT), config.Registry.CA_CRT)

	caBuf := bytes.NewBufferString(config.Registry.CA_CRT)
	_, _, filename := config.DestPath()

	if err := c.Upload(filename, caBuf); err != nil {
		entry.Errorf("SSH UploadFile %s Error,%s", filename, err)

		if err := c.Upload(filename, caBuf); err != nil {
			entry.Errorf("SSH UploadFile %s Error Twice,%s", filename, err)

			return err
		}
	}

	buffer := new(bytes.Buffer)
	cmd := remote.Cmd{
		Command: script,
		Stdout:  buffer,
		Stderr:  buffer,
	}

	err = c.Start(&cmd)
	cmd.Wait()
	if err != nil || cmd.ExitStatus != 0 {
		logrus.Errorf("Executing Remote Command: %s,Exited:%d,%v,Output:%s", cmd.Command, cmd.ExitStatus, err, buffer.String())

		cp := remote.Cmd{
			Command: script,
			Stdout:  buffer,
			Stderr:  buffer,
		}
		err = c.Start(&cp)
		cp.Wait()
		if err != nil || cp.ExitStatus != 0 {
			err = fmt.Errorf("Twice Executing Remote Command: %s,Exited:%d,%v,Output:%s", cp.Command, cp.ExitStatus, err, buffer.String())
			logrus.Error(err)

			return err
		}
	}

	entry.Info("SSH Remote PKG Install Successed! Output:\n", buffer.String())

	return nil
}

func SSHCommand(host, user, password, shell string, output io.Writer) error {
	r := &terraform.InstanceState{
		Ephemeral: terraform.EphemeralState{
			ConnInfo: map[string]string{
				"type":     "ssh",
				"user":     user,
				"password": password,
				"host":     host,
			},
		},
	}

	c, err := ssh.New(r)
	if err != nil {
		logrus.Errorf("error creating communicator: %s", err)

		return err
	}
	err = c.Connect(nil)
	if err != nil {
		logrus.Errorf("communicator connection error: %s", err)

		return err
	}
	defer c.Disconnect()

	cmd := remote.Cmd{
		Command: shell,
		Stdout:  output,
		Stderr:  output,
	}

	err = c.Start(&cmd)
	cmd.Wait()
	if err != nil || cmd.ExitStatus != 0 {
		logrus.Errorf("Executing Remote Command: %s,Exited:%d,%v", cmd.Command, cmd.ExitStatus, err)

		cp := remote.Cmd{
			Command: shell,
			Stdout:  output,
			Stderr:  output,
		}
		err = c.Start(&cp)
		cp.Wait()
		if err != nil || cp.ExitStatus != 0 {
			err = fmt.Errorf("Twice Executing Remote Command: %s,Exited:%d,%v", cp.Command, cp.ExitStatus, err)
			logrus.Error(err)

			return err
		}
	}

	logrus.Info("SSH Remote Execute Successed!")

	return nil
}

func (gd *Gardener) RegisterNodes(name string, nodes []*Node, timeout time.Duration) error {
	dc, err := gd.Datacenter(name)
	if err != nil || dc == nil {
		logrus.Error("%s Not Found,%s", name, err)
		return err
	}
	config, err := database.GetSystemConfig()
	if err != nil {
		logrus.Error(err.Error())
		return err
	}

	deadline := time.Now().Add(timeout)

	for {
		if time.Now().After(deadline) {
			err := dealWithTimeout(nodes)
			return fmt.Errorf("Timeout %ds,%v", timeout, err)
		}
		time.Sleep(30 * time.Second)

		for i := range nodes {
			if nodes[i].Status != _StatusNodeInstalled {
				logrus.Warnf("Node Status Not Match,%s:%d!=%d", nodes[i].Addr, nodes[i].Status, _StatusNodeInstalled)
				continue
			}
			eng, err := gd.updateNodeEngine(nodes[i], config.DockerPort)
			if err != nil || eng == nil {
				logrus.Error(err)
				continue
			}

			err = initNodeStores(dc, nodes[i], eng)
			if err != nil {
				logrus.Error(err)
				continue
			}

			err = database.TxUpdateNodeRegister(nodes[i].Node, nodes[i].task, _StatusNodeEnable, _StatusTaskDone, eng.ID, "")
			if err != nil {
				logrus.WithField("Host", nodes[i].Name).Errorf("Node Registed,Error:%s", err)

				continue
			}
		}
	}
}

func dealWithTimeout(nodes []*Node) error {
	logrus.Warnf("RegisterNodes Timeout")
	for i := range nodes {
		if nodes[i].Status >= _StatusNodeEnable {
			continue
		}
		if nodes[i].Status != _StatusNodeInstalled {
			nodes[i].Status = _StatusNodeInstallFailed
		}
		err := database.TxUpdateNodeRegister(nodes[i].Node, nodes[i].task, nodes[i].Status, _StatusTaskTimeout, "", "Node Register Timeout")
		if err != nil {
			logrus.Error(nodes[i].Name, "TxUpdateNodeRegister", err)
		}
	}
	return nil
}

func (gd *Gardener) updateNodeEngine(node *Node, dockerPort int) (*cluster.Engine, error) {
	addr := fmt.Sprintf("%s:%d", node.Addr, dockerPort)
	eng := gd.getEngineByAddr(addr)

	if status := ""; eng == nil || !strings.EqualFold(eng.Status(), "Healthy") {
		if eng != nil {
			status = eng.Status()
		} else {
			status = "engine is nil"
		}

		err := fmt.Errorf("Engine %s Status:%s", addr, status)
		return nil, err
	}

	err := database.TxUpdateNodeRegister(node.Node, node.task, _StatusNodeTesting, _StatusTaskRunning, eng.ID, "")
	if err != nil {
		logrus.Error(eng.Addr, "TxUpdateNodeRegister", err)
		return nil, err
	}
	node.engine = eng
	node.EngineID = eng.ID

	return eng, nil
}

func initNodeStores(dc *Datacenter, node *Node, eng *cluster.Engine) error {
	vgs := make([]store.VG, 0, 2)
	//SSD
	if ssd := eng.Labels[_SSD_VG_Label]; ssd != "" {
		vgs = append(vgs, store.VG{
			Vendor: _SSD,
			Name:   ssd,
		})
	}
	// HDD
	if hdd := eng.Labels[_HDD_VG_Label]; hdd != "" {
		vgs = append(vgs, store.VG{
			Vendor: _HDD,
			Name:   hdd,
		})
	}

	pluginAddr := fmt.Sprintf("%s:%d", eng.IP, pluginPort)
	node.localStore = store.NewLocalDisk(pluginAddr, node.Node, vgs)

	wwn := eng.Labels[_SAN_HBA_WWN_Lable]
	if strings.TrimSpace(wwn) != "" {
		list := strings.Split(wwn, ",")
		if dc.storage == nil || dc.storage.Driver() != store.SAN_StoreDriver {
			return nil
		}

		err := dc.storage.AddHost(node.ID, list...)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"Host":    node.Name,
				"Storage": dc.storage.ID(),
				"Vendor":  dc.storage.Vendor(),
			}).Errorf("Add Host To Storage Error:%s", err.Error())
		}
	}

	return nil
}
