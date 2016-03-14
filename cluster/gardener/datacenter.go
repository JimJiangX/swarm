package gardener

import (
	"bytes"
	"errors"
	"io"
	"sync"

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

func (c *Cluster) AddDatacenter(cluster database.Cluster,
	nodes []*database.Node, stores []*store.Store) error {
	if cluster.ID == "" {
		cluster.ID = c.generateUniqueID()
	}

	err := cluster.Insert()
	if err != nil {
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
		Cluster: &cluster,
		stores:  stores,
		nodes:   nodes,
	}

	c.Lock()
	c.datacenters = append(c.datacenters, dc)
	c.Unlock()

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

	dir, err := modifyProfile("")
	if err != nil {
		return err
	}

	if err := Distribute("", dir, m); err != nil {
		return err
	}

	dc.Lock()

	if err := node.Insert(); err != nil {

		dc.Unlock()

		return err
	}

	dc.nodes = append(dc.nodes, node)

	dc.Unlock()

	return nil
}
func modifyProfile(path string) (string, error) {
	return "", nil
}

func Distribute(dst, src string, m map[string]string) error {

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

	c, err := ssh.New(r)
	if err != nil {
		// t.Fatalf("error creating communicator: %s", err)
		return err
	}
	defer c.Disconnect()

	if err := c.UploadDir("dst", src); err != nil {
		if err := c.UploadDir("dst", src); err != nil {
			return err
		}
	}

	buffer := new(bytes.Buffer)

	cmd := remote.Cmd{
		Command: "echo foo",
		Stdout:  buffer,
		Stderr:  buffer,
	}

	err = c.Start(&cmd)
	if err != nil {
		// t.Fatalf("error executing remote command: %s", err)
	}
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
		// t.Fatalf("error creating communicator: %s", err)
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
