package swarm

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
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

	stores []store.Store

	nodes []*Node
}

type Node struct {
	*database.Node
	task       *database.Task
	engine     *cluster.Engine
	localStore store.Store
	user       string
	password   string
	port       int
}

func NewNode(addr, name, cluster, user, password string, port, num int) *Node {
	node := &database.Node{
		ID:           utils.Generate64UUID(),
		Name:         name,
		ClusterID:    cluster,
		Addr:         addr,
		MaxContainer: num,
	}

	task := database.NewTask("node", node.ID, "", nil, 0)

	return &Node{
		Node:     node,
		task:     task,
		user:     user,
		password: password,
		port:     port,
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

	return nil, fmt.Errorf("Not Found %s", IDOrName)

}

func (gd *Gardener) AddDatacenter(cl database.Cluster, stores []store.Store) error {
	if cl.ID == "" {
		cl.ID = gd.generateUniqueID()
	}

	log.WithFields(log.Fields{
		"dc":        cl.Name,
		"storesNum": len(stores),
	}).Info("Datacenter Initializing")

	err := cl.Insert()
	if err != nil {
		log.Errorf("DB Error,%s", err)
		return err
	}

	if stores == nil {
		stores = make([]store.Store, 0, 3)
	}

	dc := &Datacenter{
		RWMutex: sync.RWMutex{},
		Cluster: &cl,
		stores:  stores,
		nodes:   make([]*Node, 0, 100),
	}

	gd.Lock()
	gd.datacenters = append(gd.datacenters, dc)
	gd.Unlock()

	log.Infof("Datacenter Initialized:%s", dc.Name)

	return nil
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
		nodes, err := database.ListNodeByClusterType(dc.Type)
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

func (dc *Datacenter) isIdleStoreEnough(IDOrType string, num, size int) bool {
	dc.RLock()

	store, err := dc.getStore(IDOrType)
	if err != nil {
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

func (dc *Datacenter) getStore(IDOrType string) (store.Store, error) {
	for i := range dc.stores {
		if IDOrType == dc.stores[i].Vendor() || IDOrType == dc.stores[i].ID() {

			return dc.stores[i], nil
		}
	}

	return nil, fmt.Errorf("Store:%s Not Found", IDOrType)
}

func (dc *Datacenter) AllocStore(host, IDOrType string, size int64) error {
	dc.Lock()
	defer dc.Unlock()

	store, err := dc.getStore(IDOrType)
	if err != nil {
		return err
	}

	_, _, err = store.Alloc("", int(size))

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

func (dc *Datacenter) RegisterNode(IDOrName string) error {
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

	err := node.UpdateStatus(1)

	dc.Unlock()

	log.Infof("Register Node:%s to Cluster:%s", IDOrName, dc.Name)

	return err
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

	err := node.UpdateStatus(2)

	dc.Unlock()

	log.Infof("Deregister Node:%s of Cluster:%s", IDOrName, dc.Name)

	return err
}

func (dc *Datacenter) DistributeNode(node *Node, kvpath string) error {
	err := database.UpdateTaskStatus(node.task, _TaskRunning, time.Time{}, "")
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"name":    node.Name,
		"addr":    node.Addr,
		"cluster": dc.Cluster.ID,
	}).Info("Adding new Node")

	if err := node.Distribute(kvpath); err != nil {
		log.WithFields(log.Fields{
			"name":    node.Name,
			"addr":    node.Addr,
			"cluster": dc.Cluster.ID,
		}).Errorf("SSH UploadDir Error,%s", err)

		return err
	}

	dc.Lock()

	dc.nodes = append(dc.nodes, node)

	dc.Unlock()

	log.WithFields(log.Fields{
		"name":    node.Name,
		"addr":    node.Addr,
		"cluster": dc.Cluster.ID,
	}).Info("Added Node")

	err = database.UpdateTaskStatus(node.task, _TaskDone, time.Now(), "")

	return err
}

// CA,script,error
func (node Node) modifyProfile(kvpath string) (*database.Configurations, *os.File, string, error) {
	config, err := database.GetSystemConfig()
	if err != nil {
		return nil, nil, "", err
	}

	// Create a temp dir in the system default temp folder
	tempDirPath, err := ioutil.TempDir("", node.Name)
	if err != nil {
		return nil, nil, "", err
	}

	// Create a file in new temp directory
	tempFile, err := ioutil.TempFile(tempDirPath, config.CA_CRT_Name)
	if err != nil {
		return nil, nil, "", err
	}

	_, err = tempFile.WriteString(config.CA_CRT)
	if err != nil {
		return nil, nil, "", err
	}

	_, path, caFile := config.DestPath()

	list := ""
	addrs := config.GetConsulAddrs()

	for i := range addrs {
		if i != 0 {
			list += ","
		}
		list += fmt.Sprintf(`"%s"`, addrs[i])
	}
	/*
	   init.sh input
	   DATACENTER:"" //swarm keyname
	   ADM_IP:
	   CS_DATACENTER:""
	   CS_LIST:""
	   registry.domain:""
	   registry.ip:""
	   registry.port:""
	   registry.ca_crt.filepath:
	*/

	script := fmt.Sprintf("%s %s %s %s %s %s %s %d %s", path, kvpath, node.Addr, config.ConsulDatacenter, list,
		config.Registry.Domain, config.Registry.Address, config.Registry.Port, caFile)

	return config, tempFile, script, nil
}

func (node *Node) Distribute(kvpath string) error {
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

	config, caFile, script, err := node.modifyProfile(kvpath)
	if err != nil {
		return err
	}

	entry := log.WithFields(log.Fields{
		"host":        node.port,
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

	err = c.Upload(filepath.Join(config.Destination, config.CA_CRT_Name), caFile)
	if err != nil {
		entry.Errorf("SSH UploadDir Error,%s", err)

		err = c.Upload(filepath.Join(config.Destination, config.CA_CRT_Name), caFile)
		if err != nil {
			entry.Errorf("SSH UploadFile Error Twice,%s", err)

			return err
		}
	}

	// Close file
	err = caFile.Close()
	if err != nil {

	}

	// Delete the resources we created
	err = os.RemoveAll(filepath.Dir(caFile.Name()))
	if err != nil {

	}

	buffer := new(bytes.Buffer)
	cmd := remote.Cmd{
		Command: script,
		Stdout:  buffer,
		Stderr:  buffer,
	}

	err = c.Start(&cmd)
	if err != nil {
		log.Errorf("error executing remote command: %s", err)
	}

	entry.Info("SSH UploadDir Success!")

	return err
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

	return c.Start(&cmd)
}

func (gd *Gardener) RegisterNodes(name string, nodes []*Node, timeout time.Duration) error {
	dc, err := gd.Datacenter(name)
	if err != nil || dc == nil {
		return err
	}

	for {
		for i := range nodes {
			if nodes[i].Status >= 5 {
				continue
			}

			eng := gd.getEngineByAddr(nodes[i].Addr)
			if eng == nil || !strings.EqualFold(eng.Status(), "Healthy") {
				continue
			}
			nodes[i].engine = eng
			
			nodes[i].localStore = store.NewLocalDisk("",eng.Labels["vg"],nodes[i].Node)

			wwwn := eng.Labels["wwwn"]
			if strings.TrimSpace(wwwn) == "" {
				continue
			}

			list := strings.Split(wwwn, ",")

			for i := range dc.stores {
				if dc.stores[i].Driver() != "lvm" {
					continue
				}

				err := dc.stores[i].AddHost(nodes[i].ID, list...)
				if err != nil {
					continue
				}
			}

			nodes[i].Status = 5

			// servcie register
			// TODO:create container test

		}
	}
}
