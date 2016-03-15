package gardener

import (
	"bytes"
	"errors"
	"io"
	"sync"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/stringid"
	"github.com/docker/swarm/cluster/gardener/database"
	"github.com/docker/swarm/cluster/gardener/store"
	"github.com/hashicorp/terraform/communicator/remote"
	"github.com/hashicorp/terraform/communicator/ssh"
	"github.com/hashicorp/terraform/terraform"
)

type Datacenter struct {
	sync.RWMutex

	*database.Cluster

	stores []*store.Store

	nodes []*database.Node
}

func (c *Cluster) AddDatacenter(cl database.Cluster,
	nodes []*database.Node, stores []*store.Store) error {
	if cl.ID == "" {
		cl.ID = c.generateUniqueID()
	}

	log.WithFields(log.Fields{
		"cluster":   cl.Name,
		"nodeNum":   len(nodes),
		"storesNum": len(stores),
	}).Info("Adding New Datacenter")

	err := cl.Insert()
	if err != nil {
		log.Errorf("DB Error,%s", err)
		return err
	}

	if stores == nil {
		stores = make([]*store.Store, 0, 3)
	}

	if nodes == nil {
		nodes = make([]*database.Node, 0, 100)
	}

	dc := &Datacenter{
		RWMutex: sync.RWMutex{},
		Cluster: &cl,
		stores:  stores,
		nodes:   nodes,
	}

	c.Lock()
	c.datacenters = append(c.datacenters, dc)
	c.Unlock()

	log.Infof("Added Datacenter:%s", dc.Name)

	return nil
}

func (dc *Datacenter) ListNode() ([]*database.Node, error) {
	dc.RLock()
	nodes := dc.nodes
	dc.RUnlock()

	return nodes, nil
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
		ID:           stringid.GenerateRandomID(),
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
