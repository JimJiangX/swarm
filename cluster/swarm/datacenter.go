package swarm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
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
	"github.com/pkg/errors"
)

type Datacenter struct {
	sync.RWMutex

	*database.Cluster

	storage store.Store

	nodes []*Node
}

func AddNewCluster(req structs.PostClusterRequest) (database.Cluster, error) {
	if store.IsLocalStore(req.StorageType) && req.StorageID != "" {
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
	localStore *store.LocalStore
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
		Status:       statusNodeImport,
	}

	task := database.NewTask(node.Name, _Node_Install_Task, node.ID, "import node", nil, 0)

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

func SaveMultiNodesToDB(nodes []*Node) error {
	list := make([]*database.Node, len(nodes))
	tasks := make([]*database.Task, len(nodes))

	for i := range nodes {
		list[i] = nodes[i].Node
		tasks[i] = nodes[i].task
	}

	return database.TxInsertMultiNodeAndTask(list, tasks)
}

func (gd *Gardener) Datacenter(nameOrID string) (*Datacenter, error) {
	gd.RLock()
	for i := range gd.datacenters {
		if gd.datacenters[i].ID == nameOrID || gd.datacenters[i].Name == nameOrID {
			gd.RUnlock()

			return gd.datacenters[i], nil
		}
	}

	gd.RUnlock()

	//If Not Found
	dc, err := gd.rebuildDatacenter(nameOrID)
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

func (gd *Gardener) UpdateDatacenterParams(nameOrID string, max int, limit float32) error {
	dc, err := gd.Datacenter(nameOrID)
	if err != nil {
		logrus.Error(err)
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
		err := lately.UpdateParams()
		if err != nil {
			dc.Unlock()

			logrus.Errorf("DC %s,%s", dc.Name, err)
			return err
		}
		dc.Cluster = &lately
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

func (dc *Datacenter) GetNode(nameOrID string) (*Node, error) {
	if len(nameOrID) == 0 {
		return nil, errors.New("Node Name Or ID is null")
	}

	dc.RLock()
	node := dc.getNode(nameOrID)
	dc.RUnlock()

	if node != nil {

		return node, nil
	}

	return nil, errors.New("Not Found Node")
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

func (gd *Gardener) GetNode(nameOrID string) (*Datacenter, *Node, error) {
	dc, err := gd.DatacenterByNode(nameOrID)
	if dc != nil && err == nil {

		node, err := dc.GetNode(nameOrID)
		if node != nil && err == nil {
			return dc, node, nil
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

func (gd *Gardener) DatacenterByNode(nameOrID string) (*Datacenter, error) {
	node, err := database.GetNode(nameOrID)
	if err != nil {
		return nil, err
	}

	return gd.Datacenter(node.ClusterID)
}

func (gd *Gardener) DatacenterByEngine(nameOrID string) (*Datacenter, error) {
	node, err := database.GetNode(nameOrID)
	if err != nil {
		return nil, err
	}

	return gd.Datacenter(node.ClusterID)
}

func (gd *Gardener) SetNodeStatus(name string, state int64) error {
	_, node, err := gd.GetNode(name)
	if err != nil {
		logrus.Error(err)
	}
	if node == nil {
		return fmt.Errorf("Not Found Node %s", name)
	}

	if node.Status != statusNodeDisable &&
		node.Status != statusNodeEnable &&
		node.Status != statusNodeDeregisted {

		return fmt.Errorf("Node %s Status:%d,Forbidding Changing Status to %d", name, node.Status, state)
	}

	return node.UpdateStatus(state)
}

func (gd *Gardener) SetNodeParams(name string, max int) error {
	_, node, err := gd.GetNode(name)
	if err != nil {
		return err
	}

	return node.UpdateParams(max)
}

func (dc *Datacenter) RemoveNode(nameOrID string) error {
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

	return nil, errors.Errorf("Not Found Engine %s", nameOrID)
}

func (gd *Gardener) RemoveNode(nameOrID, user, password string) error {
	node, err := database.GetNode(nameOrID)
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
			return fmt.Errorf("%d Containers Has Created On Node %s", num, nameOrID)
		}

		gd.scheduler.Lock()
		for _, pending := range gd.pendingContainers {
			if pending.Engine.ID == node.EngineID {
				gd.scheduler.Unlock()

				return fmt.Errorf("Containers Has Created On Node %s", nameOrID)
			}
		}
		gd.scheduler.Unlock()
	}

	count, err := database.CountUnitByNode(node.ID)
	if err != nil || count != 0 {
		return fmt.Errorf("Count Unit ByNode,%v,count:%d", err, count)
	}

	sys, err := gd.SystemConfig()
	if err != nil {
		logrus.Errorf("SystemConfig error:%s", err)
		return err
	}
	horus := fmt.Sprintf("%s:%d", sys.HorusServerIP, sys.HorusServerPort)
	endpoint := deregisterService{Endpoint: node.ID}
	err = deregisterToHorus(horus, []deregisterService{endpoint})
	if err != nil {
		logrus.Errorf("Node %s:%s deregisterToHorus error:%s", node.Name, node.Addr, err)
		return err
	}

	err = database.DeleteNode(nameOrID)
	if err != nil {
		return err
	}

	gd.Lock()
	for i := range gd.datacenters {
		if gd.datacenters[i].ID == node.ClusterID {
			gd.datacenters[i].RemoveNode(nameOrID)
			break
		}
	}
	gd.Unlock()

	// ssh exec clean script
	err = nodeClean(node.ID, node.Addr, user, password)
	if err != nil {
		logrus.Error("clean script exec error:%s", err)
	}

	return err
}

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
		return fmt.Errorf("%d Nodes In Cluster %s", count, nameOrID)
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
	store := dc.storage
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
	logrus.WithField("nameOrID", nameOrID).Debug("rebuild Datacenter")

	cl, err := database.GetCluster(nameOrID)
	if err != nil {
		return nil, err
	}

	var storage store.Store
	if !store.IsLocalStore(cl.StorageType) && cl.StorageID != "" {
		storage, err = gd.GetStore(cl.StorageID)
		if err != nil {
			return nil, err
		}
	}

	dc := &Datacenter{
		RWMutex: sync.RWMutex{},
		Cluster: &cl,
		storage: storage,
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
			logrus.WithError(err).Error("rebuild Node:")
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
	eng, err := gd.GetEngine(n.EngineID)

	node := &Node{
		Node:   &n,
		engine: eng,
	}

	entry := logrus.WithFields(logrus.Fields{
		"Name": n.Name,
		"Addr": n.Addr,
	})
	entry.Debug("rebuild Node")

	if err != nil {
		entry.Error(err)

		return node, err
	}

	pluginAddr := fmt.Sprintf("%s:%d", eng.IP, pluginPort)
	node.localStore = store.NewLocalDisk(pluginAddr, node.Node)

	return node, nil
}

func (node *Node) getVGname(_type string) (string, error) {
	if node.engine == nil || node.engine.Labels == nil {
		return "", errEngineIsNil
	}

	parts := strings.SplitN(_type, ":", 2)
	if len(parts) == 1 {
		parts = append(parts, "HDD")
	}

	vgName, ok := node.engine.Labels[parts[1]+"_VG"]
	if !ok {
		return "", errors.Errorf("Not Found VG_Name '%s' of Node:'%s'", _type, node.Name)
	}

	return vgName, nil
}

func (gd *Gardener) shortIdleStoreFilter(list []database.Node, volumes []structs.DiskStorage, _type string, num int) []database.Node {
	logrus.Debugf("shortIdleStoreFilter:nodes=%d,Type='%s',num=%d", len(list), _type, num)

	gd.RLock()
	length := len(gd.datacenters)
	gd.RUnlock()

	if length == 0 {
		err := gd.rebuildDatacenters()
		if err != nil {
			return nil
		}
	}

	out := make([]database.Node, 0, 100)

loop:
	for i := range list {
		dc, node, err := gd.GetNode(list[i].ID)
		if err != nil || dc == nil || node == nil {
			logrus.Warningf("Not Found Node By ID:%s", list[i].ID)

			continue loop
		}

		dc.Lock()

		if !dc.Enabled {
			logrus.Debug("DC Disabled ", dc.Name)

			dc.Unlock()

			continue loop
		}

		if node.engine == nil ||
			(node.engine != nil && (!node.engine.IsHealthy() ||
				len(node.engine.Containers()) >= node.MaxContainer)) {
			logrus.Debugf("%s Engine Unmatch,%s %s", node.Name, node.ID, node.EngineID)

			dc.Unlock()

			continue loop
		}

		dc.Unlock()

		for _, v := range volumes {
			if store.IsLocalStore(v.Type) {
				if !node.isIdleStoreEnough(v.Type, v.Size) {
					logrus.Debugf("%s local store shortage:%d", node.Name, v.Size)

					continue loop
				}
			} else if v.Type == store.SANStore {
				// when storage is HITACHI or HUAWEI
				if !dc.isIdleStoreEnough(num/2, v.Size) {
					logrus.Debugf("%s san store shortage:%d", dc.Name, v.Size)

					continue loop
				}
			}
		}

		out = append(out, list[i])
	}

	return out
}

func (node *Node) isIdleStoreEnough(_type string, size int) bool {
	if !store.IsLocalStore(_type) {

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

	vgName, err := node.getVGname(_type)
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

func (dc *Datacenter) DistributeNode(node *Node) error {
	entry := logrus.WithFields(logrus.Fields{
		"name":    node.Name,
		"addr":    node.Addr,
		"cluster": dc.Cluster.ID,
	})
	err := database.TxUpdateNodeStatus(node.Node, node.task,
		statusNodeInstalling, statusTaskRunning, "")
	if err != nil {
		entry.Error(err)
		return err
	}

	entry.Info("Adding new Node")

	if err := node.Distribute(); err != nil {
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
func (node Node) modifyProfile() (*database.Configurations, string, error) {
	config, err := database.GetSystemConfig()
	if err != nil {
		return nil, "", err
	}

	config.SourceDir, err = utils.GetAbsolutePath(true, config.SourceDir)
	if err != nil {
		return nil, "", err
	}

	path, caFile, _ := config.DestPath()

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

	script := fmt.Sprintf("chmod 755 %s && %s %s %s %s '%s' %s %s %d %s %s %s %d %s %s %d %d %s %s %d %d %s %s %s %s",
		path, path, DockerNodesKVPath, node.Addr, config.ConsulDatacenter, string(buf),
		config.Registry.Domain, config.Registry.Address, config.Registry.Port,
		config.Registry.Username, config.Registry.Password, caFile,
		config.DockerPort, hdd, ssd, config.HorusAgentPort, config.ConsulPort,
		node.ID, config.HorusServerIP, config.HorusServerPort, config.PluginPort,
		config.NFSOption.Addr, config.NFSOption.Dir, config.MountDir, config.MountOptions)

	return config, script, nil
}

func (node *Node) Distribute() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Recover From Panic:%v", r)
		}

		nodeState, taskState, msg := int64(0), int64(0), ""
		if err == nil {
			nodeState = statusNodeInstalled
		} else {
			nodeState = statusNodeInstallFailed
			taskState = statusTaskFailed
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

	config, script, err := node.modifyProfile()
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
		entry.Errorf("communicator connection error: %s", err)

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
	_, filename, _ := config.DestPath()

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
		entry.Errorf("Executing Remote Command: %s,Exited:%d,%v,Output:%s", cmd.Command, cmd.ExitStatus, err, buffer.String())

		cp := remote.Cmd{
			Command: script,
			Stdout:  buffer,
			Stderr:  buffer,
		}
		err = c.Start(&cp)
		cp.Wait()
		if err != nil || cp.ExitStatus != 0 {
			err = fmt.Errorf("Twice Executing Remote Command: %s,Exited:%d,%v,Output:%s", cp.Command, cp.ExitStatus, err, buffer.String())
			entry.Error(err)

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
	config, err := gd.SystemConfig()
	if err != nil {
		logrus.Error(err)
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
			if nodes[i].Status != statusNodeInstalled {
				logrus.Warnf("Node Status Not Match,%s:%d!=%d", nodes[i].Addr, nodes[i].Status, statusNodeInstalled)
				continue
			}
			eng, err := gd.updateNodeEngine(nodes[i], config.DockerPort)
			if err != nil || eng == nil {
				logrus.Warn(err)
				continue
			}

			err = initNodeStores(dc, nodes[i], eng)
			if err != nil {
				logrus.Error(err)
				continue
			}

			err = database.TxUpdateNodeRegister(nodes[i].Node, nodes[i].task, statusNodeEnable, statusTaskDone, eng.ID, "")
			if err != nil {
				logrus.WithField("Host", nodes[i].Name).Errorf("Node Registed,Error:%s", err)

				continue
			}

			nodes[i].engine = eng
			nodes[i].EngineID = eng.ID
		}
	}
}

func dealWithTimeout(nodes []*Node) error {
	logrus.Warnf("RegisterNodes Timeout")
	for i := range nodes {
		if nodes[i].Status >= statusNodeEnable {
			continue
		}
		if nodes[i].Status != statusNodeInstalled {
			nodes[i].Status = statusNodeInstallFailed
		}
		err := database.TxUpdateNodeRegister(nodes[i].Node, nodes[i].task, nodes[i].Status, statusTaskTimeout, "", "Node Register Timeout")
		if err != nil {
			logrus.Error(nodes[i].Name, "TxUpdateNodeRegister", err)
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

		return nil, errors.Errorf("Engine %s Status:%s", addr, status)
	}

	return eng, nil
}

func initNodeStores(dc *Datacenter, node *Node, eng *cluster.Engine) error {
	pluginAddr := fmt.Sprintf("%s:%d", eng.IP, pluginPort)
	node.localStore = store.NewLocalDisk(pluginAddr, node.Node)

	wwn := eng.Labels[_SAN_HBA_WWN_Lable]
	if strings.TrimSpace(wwn) != "" {
		list := strings.Split(wwn, ",")
		if dc.storage == nil || dc.storage.Driver() != store.SANStoreDriver {
			return nil
		}

		err := dc.storage.AddHost(node.ID, list...)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"Host":    node.Name,
				"Storage": dc.storage.ID(),
				"Vendor":  dc.storage.Vendor(),
			}).Errorf("Add Host To Storage Error:%s", err)
		}
	}

	return nil
}

func nodeClean(node, addr, user, password string) error {
	config, err := database.GetSystemConfig()
	if err != nil {
		return err
	}
	_, _, destName := config.DestPath()

	srcFile, err := utils.GetAbsolutePath(false, config.SourceDir, config.CleanScriptName)
	if err != nil {
		logrus.Errorf("%s %s", srcFile, err)
		return err
	}

	file, err := os.Open(srcFile)
	if err != nil {
		logrus.Error(err)
		return err
	}

	r := &terraform.InstanceState{
		Ephemeral: terraform.EphemeralState{
			ConnInfo: map[string]string{
				"type":     "ssh",
				"user":     user,
				"password": password,
				"host":     addr,
			},
		},
	}
	entry := logrus.WithFields(logrus.Fields{
		"host":        addr,
		"user":        user,
		"source":      srcFile,
		"destination": destName,
	})
	c, err := ssh.New(r)
	if err != nil {
		entry.Errorf("error creating communicator: %s", err)

		return err
	}
	err = c.Connect(nil)
	if err != nil {
		entry.Errorf("communicator connection error: %s", err)

		return err
	}
	defer c.Disconnect()

	if err := c.Upload(destName, file); err != nil {
		entry.Errorf("SSH UploadFile %s Error,%s", destName, err)

		if err := c.Upload(destName, file); err != nil {
			entry.Errorf("SSH UploadFile %s Error Twice,%s", destName, err)

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

	script := fmt.Sprintf("chmod 755 %s && %s %s %d %s %s %d %s",
		destName, destName, addr, config.ConsulPort, node,
		config.HorusServerIP, config.HorusServerPort, config.NFSOption.MountDir)

	buffer := new(bytes.Buffer)
	cmd := remote.Cmd{
		Command: script,
		Stdout:  buffer,
		Stderr:  buffer,
	}

	err = c.Start(&cmd)
	cmd.Wait()
	if err != nil || cmd.ExitStatus != 0 {
		entry.Errorf("Executing Remote Command: %s,Exited:%d,%v,Output:%s", cmd.Command, cmd.ExitStatus, err, buffer.String())

		cp := remote.Cmd{
			Command: script,
			Stdout:  buffer,
			Stderr:  buffer,
		}
		err = c.Start(&cp)
		cp.Wait()
		if err != nil || cp.ExitStatus != 0 {
			err = fmt.Errorf("Twice Executing Remote Command: %s,Exited:%d,%v,Output:%s", cp.Command, cp.ExitStatus, err, buffer.String())
			entry.Error(err)

			return err
		}
	}

	entry.Info("SSH Remote Exec Successed! Output:\n", buffer.String())

	return nil
}
