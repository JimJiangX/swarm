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

	log "github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/store"
	"github.com/docker/swarm/utils"
	"github.com/hashicorp/terraform/communicator/remote"
	"github.com/hashicorp/terraform/communicator/ssh"
	"github.com/hashicorp/terraform/terraform"
)

const (
	LocalDiskStore = "localDiskStore"
)

type Datacenter struct {
	sync.RWMutex

	*database.Cluster

	storage store.Store

	nodes []*Node
}

type Node struct {
	*database.Node
	task       *database.Task
	engine     *cluster.Engine
	localStore store.Store
	hdd        string
	ssd        string
	user       string
	password   string
	port       int
}

func NewNode(addr, name, cluster, user, password, hdd, ssd string, port, num int) *Node {
	node := &database.Node{
		ID:        utils.Generate64UUID(),
		Name:      name,
		ClusterID: cluster,
		Addr:      addr,

		MaxContainer: num,
		Status:       _StatusNodeImport,
	}

	task := database.NewTask("node", node.ID, "import node", nil, 0)

	return &Node{
		Node:     node,
		task:     task,
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
	err := database.TxInsertNodeAndTask(node.Node, node.task)
	if err != nil {
		log.Errorf("Node:%s Insert INTO DB Error,%s", node.Name, err)
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

	cl, err := database.GetCluster(IDOrName)
	if err != nil || cl == nil {
		return nil, fmt.Errorf("Not Found %s,Error %s", IDOrName, err)
	}

	dc := &Datacenter{
		Cluster: cl,
		nodes:   make([]*Node, 0, 100),
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

	log.WithFields(log.Fields{
		"dc": cl.Name,
	}).Info("Datacenter Initializing")

	err := cl.Insert()
	if err != nil {
		log.Errorf("DB Error,%s", err)
		return err
	}

	dc := &Datacenter{
		RWMutex: sync.RWMutex{},
		Cluster: &cl,
		storage: storage,
		nodes:   make([]*Node, 0, 100),
	}

	gd.Lock()
	gd.datacenters = append(gd.datacenters, dc)
	gd.Unlock()

	log.Infof("Datacenter Initialized:%s", dc.Name)

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

func (dc *Datacenter) GetNode(IDOrName string) (database.Node, error) {

	if len(IDOrName) == 0 {
		return database.Node{}, errors.New("Not Found Node")
	}

	dc.RLock()

	node := dc.getNode(IDOrName)

	dc.RUnlock()

	if node != nil {

		return *node.Node, nil
	}

	return database.GetNode(IDOrName)
}

func (dc *Datacenter) getNode(IDOrName string) *Node {
	for i := range dc.nodes {
		if dc.nodes[i].ID == IDOrName || dc.nodes[i].Name == IDOrName {

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

func (gd *Gardener) DatacenterByNode(IDOrName string) (*Datacenter, error) {
	node, err := database.GetNode(IDOrName)
	if err != nil {
		return nil, err
	}

	return gd.Datacenter(node.ClusterID)
}

func (gd *Gardener) SetNodeStatus(cluster, name string, state int) error {
	gd.RLock()
	dc, err := gd.Datacenter(cluster)
	gd.RLock()
	if err != nil {
		return err
	}

	node, err := dc.GetNode(name)
	if err != nil {
		return err
	}

	return node.UpdateStatus(state)
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

func (dc *Datacenter) AllocStore(host, IDOrType string, size int64) error {
	dc.Lock()
	defer dc.Unlock()

	store := dc.getStore(IDOrType)
	if store == nil {
		return fmt.Errorf("Not Enough Storage %s,%d for Host %s", IDOrType, size, host)
	}

	_, _, err := store.Alloc("", int(size))

	return err
}

func (gd *Gardener) listShortIdleStore(IDOrType string, num, size int) []string {
	if IDOrType == LocalDiskStore {
		return nil
	}

	out := make([]string, 0, 100)
	gd.RLock()
	defer gd.RUnlock()

	for i := range gd.datacenters {
		if !gd.datacenters[i].isIdleStoreEnough(IDOrType, num, size) {
			out = append(out, gd.datacenters[i].ID)
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

	log.Infof("Deregister Node:%s of Cluster:%s", IDOrName, dc.Name)

	return err
}

func (dc *Datacenter) DistributeNode(node *Node, kvpath string) error {
	entry := log.WithFields(log.Fields{
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
		init.sh input
		swarm_key=$1
		adm_ip=$2
		cs_datacenter=$3
		cs_list=$4
		registry_domain=$5
		registry_ip=$6
		registry_port=$7
		registry_username=$8
		registry_passwd=$9
		regstry_ca_file=$10
		DOCKER_PORT=$11
	*/

	script := fmt.Sprintf("%s %s %s %s '%s' %s %s %d %s %s %s %d %s %s",
		path, kvpath, node.Addr, config.ConsulDatacenter, string(buf),
		config.Registry.Domain, config.Registry.Address, config.Registry.Port,
		config.Registry.Username, config.Registry.Password, caFile,
		config.DockerPort, node.hdd, node.ssd)

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
			log.Error(err, r)
		}
	}()

	if node.port == 0 {
		node.port = 22
	}

	r := &terraform.InstanceState{
		Ephemeral: terraform.EphemeralState{
			ConnInfo: map[string]string{
				"type":     "ssh",
				"user":     node.user,
				"password": node.password,
				"host":     node.Addr,
				"port":     strconv.Itoa(node.port),
				"timeout":  "30s",
			},
		},
	}

	config, script, err := node.modifyProfile(kvpath)
	if err != nil {
		log.Error(err)

		return err
	}

	entry := log.WithFields(log.Fields{
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
	defer c.Disconnect()

	if err := c.UploadDir(config.Destination, config.SourceDir); err != nil {
		entry.Errorf("SSH UploadDir Error,%s", err)

		if err := c.UploadDir(config.Destination, config.SourceDir); err != nil {
			entry.Errorf("SSH UploadDir Error Twice,%s", err)

			return err
		}
	}

	log.Info("Registry.CA_CRT", len(config.Registry.CA_CRT), config.Registry.CA_CRT)

	caBuf := bytes.NewBufferString(config.Registry.CA_CRT)
	_, scriptName, filename := config.DestPath()

	if err := c.Upload(filename, caBuf); err != nil {
		entry.Errorf("SSH UploadFile %s Error,%s", filename, err)

		if err := c.Upload(filename, caBuf); err != nil {
			entry.Errorf("SSH UploadFile %s Error Twice,%s", filename, err)

			return err
		}
	}

	buffer := new(bytes.Buffer)
	// chmod script file
	chmod := remote.Cmd{
		Command: "chmod 755 " + scriptName,
		Stdout:  buffer,
		Stderr:  buffer,
	}

	err = c.Start(&chmod)
	time.Sleep(time.Second * 3)
	if err != nil || chmod.ExitStatus != 0 {
		log.Errorf("Executing Remote Command: %s,Exited:%d,%s,Output:%s", chmod.Command, chmod.ExitStatus, err, buffer.String())

		cp := remote.Cmd{
			Command: "chmod 755 " + scriptName,
			Stdout:  buffer,
			Stderr:  buffer,
		}
		if err := c.Start(&cp); err != nil || cp.ExitStatus != 0 {
			err = fmt.Errorf("Executing Remote Command Twice: %s,Exited:%d,%s,Output:%s", cp.Command, cp.ExitStatus, err, buffer.String())
			log.Error(err)

			return err
		}
	}

	cmd := remote.Cmd{
		Command: script,
		Stdout:  buffer,
		Stderr:  buffer,
	}

	err = c.Start(&cmd)
	time.Sleep(time.Second)
	if err != nil || cmd.ExitStatus != 0 {
		log.Errorf("Executing Remote Command: %s,Exited:%d,%s,Output:%s", cmd.Command, cmd.ExitStatus, err, buffer.String())

		cp := remote.Cmd{
			Command: script,
			Stdout:  buffer,
			Stderr:  buffer,
		}
		if err := c.Start(&cp); err != nil || cp.ExitStatus != 0 {
			err = fmt.Errorf("Executing Remote Command Twice: %s,Exited:%d,%s,Output:%s", cp.Command, cp.ExitStatus, err, buffer.String())
			log.Error(err)

			return err
		}
	}

	entry.Info("SSH Remote PKG Install Successed! Output:\n", buffer.String())

	return nil
}

func SSHCommand(host, port, user, password, shell string, output io.Writer) error {
	if port == "0" || port == "" {
		port = "22"
	}

	r := &terraform.InstanceState{
		Ephemeral: terraform.EphemeralState{
			ConnInfo: map[string]string{
				"type":     "ssh",
				"user":     user,
				"password": password,
				"host":     host,
				"port":     port,
				"timeout":  "30s",
			},
		},
	}

	c, err := ssh.New(r)
	if err != nil {
		log.Errorf("error creating communicator: %s", err)

		return err
	}
	defer c.Disconnect()

	cmd := remote.Cmd{
		Command: shell,
		Stdout:  output,
		Stderr:  output,
	}

	err = c.Start(&cmd)
	time.Sleep(time.Second)
	if err != nil || cmd.ExitStatus != 0 {
		log.Errorf("Executing Remote Command: %s,Exited:%d,%s", cmd.Command, cmd.ExitStatus, err)

		cp := remote.Cmd{
			Command: shell,
			Stdout:  output,
			Stderr:  output,
		}
		if err := c.Start(&cp); err != nil || cp.ExitStatus != 0 {
			err = fmt.Errorf("Executing Remote Command Twice: %s,Exited:%d,%s", cp.Command, cp.ExitStatus, err)
			log.Error(err)

			return err
		}
	}

	log.Info("SSH Remote Execute Successed!")

	return nil
}

func (gd *Gardener) RegisterNodes(name string, nodes []*Node, timeout time.Duration) error {
	dc, err := gd.Datacenter(name)
	if err != nil || dc == nil {
		log.Error("%s Not Found,%s", name, err)
		return err
	}
	config, err := database.GetSystemConfig()
	if err != nil {
		return err
	}

	deadline := time.Now().Add(timeout)

	// TODO: set timeout

	for {
		if time.Now().After(deadline) {
			log.Error("RegisterNodes Timeout:%d", timeout)
			return fmt.Errorf("Timeout %ds", timeout)
		}
		time.Sleep(10 * time.Second)

		for i := range nodes {
			if nodes[i].Status != _StatusNodeInstalled {
				log.Warnf("Node Status Not Match,%s:%d!=%d", nodes[i].Addr, nodes[i].Status, _StatusNodeInstalled)
				continue
			}

			addr := fmt.Sprintf("%s:%d", nodes[i].Addr, config.DockerPort)
			eng := gd.getEngineByAddr(addr)

			if status := ""; eng == nil || !strings.EqualFold(eng.Status(), "Healthy") {
				if eng != nil {
					status = eng.Status()
				} else {
					status = "engine is nil"
				}

				log.Warnf("Engine %s Status:%s", addr, status)
				continue
			}
			nodes[i].engine = eng

			err = database.TxUpdateNodeRegister(nodes[i].Node, nodes[i].task, _StatusNodeEnable, _StatusTaskDone, eng.ID, "")
			if err != nil {
				log.Error(eng.Addr, "TxUpdateNodeRegister", err)
				continue
			}

			nodes[i].localStore = store.NewLocalDisk("", eng.Labels["vg"], nodes[i].Node)

			continue

			wwwn := eng.Labels["wwwn"]
			if strings.TrimSpace(wwwn) == "" {
				continue
			}

			list := strings.Split(wwwn, ",")

			if dc.storage == nil || dc.storage.Driver() != "lvm" {
				continue
			}

			err := dc.storage.AddHost(nodes[i].ID, list...)
			if err != nil {
				continue
			}

			err = database.TxUpdateNodeRegister(nodes[i].Node, nodes[i].task, _StatusNodeEnable, _StatusTaskDone, eng.ID, "")

			// servcie register
			// TODO:create container test

		}
	}
}
