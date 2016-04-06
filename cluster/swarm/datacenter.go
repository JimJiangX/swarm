package swarm

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"

	log "github.com/Sirupsen/logrus"
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

	nodes []*database.Node
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
		nodes:   make([]*database.Node, 0, 100),
	}

	gd.Lock()
	gd.datacenters = append(gd.datacenters, dc)
	gd.Unlock()

	log.Infof("Datacenter Initialized:%s", dc.Name)

	return nil
}

func (dc *Datacenter) ListNode() []*database.Node {
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

		return *node, nil
	}

	return database.GetNode(IDOrName)
}

func (dc *Datacenter) getNode(IDOrName string) *database.Node {
	for i := range dc.nodes {
		if dc.nodes[i].ID == IDOrName || dc.nodes[i].Name == IDOrName {

			return dc.nodes[i]
		}
	}

	return nil
}

func (dc *Datacenter) listNodeID() []string {
	out := make([]string, 0, len(dc.nodes))

	dc.RLock()
	nodes := dc.nodes

	if len(nodes) == 0 {

		var err error
		nodes, err = database.ListNodeByClusterType(dc.Type)
		if err != nil {
			dc.RUnlock()
			return nil
		}

	}

	dc.RUnlock()

	for i := range nodes {
		out = append(out, nodes[i].ID)
	}

	return out
}

func (gd *Gardener) DatacenterByNode(IDOrName string) (*Datacenter, error) {
	gd.RLock()

	for i := range gd.datacenters {

		if gd.datacenters[i].isNodeExist(IDOrName) {
			gd.RUnlock()

			return gd.datacenters[i], nil
		}
	}

	gd.RUnlock()

	node, err := database.GetNode(IDOrName)
	if err != nil {
		return nil, err
	}

	gd.RLock()
	for i := range gd.datacenters {
		if gd.datacenters[i].ID == node.ClusterID {
			gd.RUnlock()

			return gd.datacenters[i], nil
		}
	}

	gd.RUnlock()

	return nil, errors.New("Datacenter Not Found")
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
		node = &database.Node{
			ID: IDOrName,
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
		node = &database.Node{
			ID: IDOrName,
		}
	}

	dc.Lock()

	err := node.UpdateStatus(2)

	dc.Unlock()

	log.Infof("Deregister Node:%s of Cluster:%s", IDOrName, dc.Name)

	return err
}

func (dc *Datacenter) AddNode(addr, name, os_user, os_pwd string, num int) error {
	node := &database.Node{
		ID:           utils.Generate64UUID(),
		Name:         name,
		ClusterID:    dc.Cluster.ID,
		Addr:         addr,
		MaxContainer: num,
	}

	m := map[string]string{
		"host":     addr,
		"user":     os_user,
		"password": os_pwd,
	}

	log.WithFields(log.Fields{
		"name":    name,
		"addr":    addr,
		"cluster": dc.Cluster.ID,
	}).Info("Adding new Node")

	dir, script, err := modifyProfile("")
	if err != nil {
		log.WithFields(log.Fields{
			"name":    name,
			"addr":    addr,
			"cluster": dc.Cluster.ID,
			"source":  "",
		}).Errorf("Modify shell script Error,%s", err)

		return err
	}

	if err := Distribute("", dir, script, m); err != nil {
		log.WithFields(log.Fields{
			"name":        name,
			"addr":        addr,
			"cluster":     dc.Cluster.ID,
			"source":      dir,
			"destination": "",
		}).Errorf("SSH UploadDir Error,%s", err)

		return err
	}

	if err := node.Insert(); err != nil {
		log.Errorf("Node:%d Insert INTO DB Error,%s", name, err)

		return err
	}

	dc.Lock()
	dc.nodes = append(dc.nodes, node)
	dc.Unlock()

	log.WithFields(log.Fields{
		"name":    name,
		"addr":    addr,
		"cluster": dc.Cluster.ID,
	}).Info("Added Node")

	return nil
}

func modifyProfile(path string) (string, string, error) {
	return "", "", nil
}

func Distribute(dst, src string, script string, m map[string]string) error {

	if m["port"] == "0" || m["port"] == "" {
		m["port"] = "22"
	}

	r := &terraform.InstanceState{
		Ephemeral: terraform.EphemeralState{
			ConnInfo: map[string]string{
				"type":     "ssh",
				"user":     m["user"],
				"password": m["password"],
				"host":     m["host"],
				"port":     m["port"],
				"timeout":  "30s",
			},
		},
	}

	entry := log.WithFields(log.Fields{
		"user":        m["user"],
		"host":        m["host"],
		"port":        m["port"],
		"source":      src,
		"destination": dst,
	})

	c, err := ssh.New(r)
	if err != nil {
		entry.Errorf("error creating communicator: %s", err)

		return err
	}
	defer c.Disconnect()

	if err := c.UploadDir(dst, src); err != nil {
		entry.Errorf("SSH UploadDir Error,%s", err)

		if err := c.UploadDir(dst, src); err != nil {
			entry.Errorf("SSH UploadDir Error Twice,%s", err)

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
