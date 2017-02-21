package resource

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/scplib"
	"github.com/docker/swarm/garden/utils"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

const (
	statusTaskCreate = iota
	statusTaskRunning
	statusTaskStop
	statusTaskCancel
	statusTaskDone
	statusTaskTimeout
	statusTaskFailed
)

const (
	statusNodeImport = iota
	statusNodeInstalling
	statusNodeInstalled
	statusNodeInstallFailed
	statusNodeTesting
	statusNodeFailedTest
	statusNodeEnable
	statusNodeDisable

	statusNodeSSHLoginFailed
	statusNodeSCPFailed
	statusNodeSSHExecFailed
	statusNodeRegisterFailed
	statusNodeRegisterTimeout
	statusNodeDeregisted
)

// parseNodeStatus returns the meaning of the number corresponding
func parseNodeStatus(status int) string {
	switch status {
	case statusNodeImport:
		return "importing"
	case statusNodeInstalling:
		return "installing"
	case statusNodeInstalled:
		return "install failed"
	case statusNodeTesting:
		return "testing"
	case statusNodeFailedTest:
		return "test failed"
	case statusNodeEnable:
		return "enable"
	case statusNodeDisable:
		return "disable"
	case statusNodeDeregisted:
		return "deregister"
	default:
	}

	return "unknown"
}

const (
	dockerNodesKVPath = "docker/nodes/KVPath"
)

type Node struct {
	node   database.Node
	belong *database.Cluster
	eng    *cluster.Engine
	no     database.NodeOrmer
}

func newNode(n database.Node, eng *cluster.Engine, no database.NodeOrmer) Node {
	return Node{
		node: n,
		eng:  eng,
		no:   no,
	}
}

func (n Node) getCluster() (*database.Cluster, error) {
	if n.belong != nil {
		return n.belong, nil
	}

	c, err := n.no.GetCluster(n.node.ClusterID)
	if err == nil {
		n.belong = &c
	}

	return n.belong, err
}

func (n Node) removeCondition() error {
	return nil
}

func (m master) getNode(nameOrID string) (Node, error) {
	n, err := m.dco.GetNode(nameOrID)
	if err != nil {
		return Node{}, err
	}

	if n.EngineID == "" {
		return Node{node: n}, nil
	}

	eng := m.clsuter.Engine(n.EngineID)

	return newNode(n, eng, m.dco), nil
}

func (m master) updateNode(nameOrID string, status, maxContainer int) (database.Node, error) {
	n, err := m.getNode(nameOrID)
	if err != nil {
		return database.Node{}, err
	}

	if status != 0 {
		n.node.Status = status
	}
	if maxContainer != 0 {
		n.node.MaxContainer = maxContainer
	}

	err = m.dco.SetNodeParams(n.node)
	if err != nil {
		return n.node, err
	}

	return n.node, nil
}

type nodeWithTask struct {
	hdd    []string
	ssd    []string
	client scplib.ScpClient
	Node   database.Node
	Task   database.Task
}

func NewNodeWithTask(user, password string, hdd, ssd []string, n database.Node) (nodeWithTask, error) {
	c, err := scplib.NewScpClient(n.Addr, user, password)
	if err != nil {
		return nodeWithTask{}, err
	}

	t := database.NewTask()

	return nodeWithTask{
		hdd:    hdd,
		ssd:    ssd,
		client: c,
		Node:   n,
		Task:   t,
	}, nil
}

func NewNodeWithTaskList(len int) []nodeWithTask {
	return make([]nodeWithTask, len)
}

// InstallNodes install new nodes,list should has same ClusterID
func (m master) InstallNodes(ctx context.Context, horus string, list []nodeWithTask) error {
	nodes := make([]database.Node, len(list))
	tasks := make([]database.Task, len(list))

	for i := range list {
		nodes[i] = list[i].Node
		tasks[i] = list[i].Task
	}

	select {
	default:
	case <-ctx.Done():
		return ctx.Err()
	}

	err := m.dco.InsertNodesAndTask(nodes, tasks)
	if err != nil {
		return err
	}

	config, err := m.dco.GetSysConfig()
	if err != nil {
		return err
	}

	timeout := 250*time.Second + time.Duration(len(list)*30)*time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)

	for i := range list {
		go list[i].distribute(ctx, horus, m.dco, config)
	}

	go m.registerNodes(ctx, cancel, list, config)

	return nil
}

func (nt *nodeWithTask) distribute(ctx context.Context, horus string, ormer database.ClusterOrmer, config database.SysConfig) (err error) {
	entry := logrus.WithFields(logrus.Fields{
		"Node": nt.Node.Name,
		"host": nt.Node.Addr,
	})

	nodeState := statusNodeInstalling

	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("Recover from Panic:%v", r)
		}

		if err == nil {
			nt.Node.Status = statusNodeInstalled
			nt.Task.Status = statusTaskRunning
		} else {
			if nodeState == statusNodeInstalling {
				nt.Node.Status = statusNodeInstallFailed
			} else {
				nt.Node.Status = nodeState
			}

			nt.Task.Status = statusTaskFailed
			nt.Task.Errors = err.Error()
			nt.Task.FinishedAt = time.Now()
		}

		_err := ormer.RegisterNode(nt.Node, nt.Task)
		if _err != nil {
			entry.Errorf("%+v,%+v", err, _err)
		}
	}()

	script, err := nt.modifyProfile(horus, &config)
	if err != nil {
		entry.WithError(err).Error("modify profile")

		return err
	}

	entry = entry.WithFields(logrus.Fields{
		"source":      config.SourceDir,
		"destination": config.Destination,
	})

	select {
	default:
	case <-ctx.Done():
		return ctx.Err()
	}

	defer nt.client.Close()

	if err := nt.client.UploadDir(config.Destination, config.SourceDir); err != nil {
		entry.WithError(err).Error("SSH upload dir:" + config.SourceDir)

		if err := nt.client.UploadDir(config.Destination, config.SourceDir); err != nil {
			entry.WithError(err).Error("SSH upload dir twice:" + config.SourceDir)

			nodeState = statusNodeSCPFailed
			return err
		}
	}

	_, filename, _ := config.DestPath()

	if err := nt.client.Upload(config.Registry.CA_CRT, filename, 0644); err != nil {
		entry.WithError(err).Error("SSH upload file:" + filename)

		if err := nt.client.Upload(config.Registry.CA_CRT, filename, 0644); err != nil {
			entry.WithError(err).Error("SSH upload file twice:" + filename)

			nodeState = statusNodeSCPFailed
			return err
		}
	}

	select {
	default:
	case <-ctx.Done():
		return ctx.Err()
	}

	out, err := nt.client.Exec(script)
	if err != nil {
		entry.WithError(err).Errorf("exec remote command:'%s',output:%s", script, out)

		if out, err = nt.client.Exec(script); err != nil {
			entry.WithError(err).Errorf("exec remote command twice:'%s',output:%s", script, out)

			nodeState = statusNodeSSHExecFailed
			return err
		}
	}

	entry.Infof("SSH remote PKG install successed! output:\n%s")

	return nil
}

// CA,script,error
func (node *nodeWithTask) modifyProfile(horus string, config *database.SysConfig) (string, error) {
	horusIP, horusPort, err := net.SplitHostPort(horus)
	if err != nil {
		return "", errors.Wrap(err, "Horus addr:"+horus)
	}

	sourceDir, err := utils.GetAbsolutePath(true, config.SourceDir)
	if err != nil {
		return "", errors.Wrap(err, "get sourceDir:"+config.SourceDir)
	}

	config.SourceDir = sourceDir
	path, caFile, _ := config.DestPath()

	buf, err := json.Marshal(config.GetConsulAddrs())
	if err != nil {
		return "", errors.Wrap(err, "JSON marshal consul addrs")
	}
	/*
		#!/bin/bash
		swarm_key=$1
		adm_ip=$2
		cs_nodes=$3
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
		path, path, dockerNodesKVPath, node.Node.Addr, config.ConsulDatacenter, string(buf),
		config.Registry.Domain, config.Registry.Address, config.Registry.Port,
		config.Registry.Username, config.Registry.Password, caFile,
		config.DockerPort, hdd, ssd, config.HorusAgentPort, config.ConsulPort,
		node.Node.ID, horusIP, horusPort, config.PluginPort,
		config.NFSOption.Addr, config.NFSOption.Dir, config.MountDir, config.MountOptions)

	return script, nil
}

// registerNodes register Nodes
func (m master) registerNodes(ctx context.Context, cancel context.CancelFunc, nodes []nodeWithTask, config database.SysConfig) {
	defer cancel()

	cID := ""
	for i := range nodes {
		if nodes[i].Node.ClusterID != "" {
			cID = nodes[i].Node.ClusterID
			break
		}
	}
	if cID == "" {
		logrus.Error("ClusterID is required")
		return
	}

	field := logrus.WithField("Cluster", cID)
	t := time.NewTicker(time.Minute * 2)
	defer t.Stop()

	for {
		select {
		case <-t.C:

		case <-ctx.Done():
			err := m.registerNodesTimeout(nodes, ctx.Err())
			field.Errorf("deal with Nodes timeout%+v", err)

			return
		}

		list, err := m.dco.ListNodeByCluster(cID)
		if err != nil {
			field.Errorf("%+v", err)
			continue
		}

		for i := range nodes {
			for l := range list {
				if list[l].ID == nodes[i].Node.ID {
					nodes[i].Node = list[l]
					break
				}
			}
		}

		count := 0
		for i := range nodes {
			n := nodes[i].Node
			fields := field.WithFields(logrus.Fields{
				"Node": n.Name,
				"addr": n.Addr,
			})

			if n.Status != statusNodeInstalled {
				if n.Status > statusNodeInstalled {
					count++
					if count >= len(nodes) {
						return
					}
					continue
				}

				fields.Warnf("status not match,%d!=%d", n.Status, statusNodeInstalled)
				continue
			}

			addr := n.Addr + ":" + strconv.Itoa(config.DockerPort)
			eng := m.clsuter.EngineByAddr(addr)
			if eng == nil || !eng.IsHealthy() {
				fields.Error(err)

				continue
			}

			n.EngineID = eng.ID
			n.Status = statusNodeEnable
			n.RegisterAt = time.Now()

			t := nodes[i].Task
			t.Status = statusTaskDone
			t.FinishedAt = n.RegisterAt
			t.Errors = ""

			err = m.dco.RegisterNode(n, t)
			if err != nil {
				fields.Errorf("%+v", err)
			}
		}
	}
}

func (m master) registerNodesTimeout(nodes []nodeWithTask, er error) error {
	if len(nodes) == 0 {
		return nil
	}

	cID, in := "", make([]string, 0, len(nodes))

	for i := range nodes {
		if nodes[i].Node.ID != "" {
			in = append(in, nodes[i].Node.ID)
			if cID == "" {
				cID = nodes[i].Node.ClusterID
			}
		}
	}

	list, err := m.dco.ListNodeByCluster(cID)
	if err != nil {
		return err
	}

	for n := range nodes {
		for i := range list {
			if list[i].ID == nodes[n].Node.ID {
				nodes[n].Node = list[i]
				break
			}
		}
	}

	for i := range nodes {
		n, t := nodes[i].Node, nodes[i].Task
		if n.Status >= statusNodeEnable {
			continue
		}

		if n.Status != statusNodeInstalled {
			n.Status = statusNodeRegisterTimeout
			n.RegisterAt = time.Now()

			t.Status = statusTaskFailed
			t.FinishedAt = n.RegisterAt
			t.Errors = er.Error()
		}

		err = m.dco.RegisterNode(n, t)
		if err != nil {
			logrus.WithField("Node", n.Name).WithError(err).Error("Node register timeout")
		}
	}

	return nil
}

func (m master) removeNode(ID string) error {

	return m.dco.DelNode(ID)
}

func (m master) RemoveNode(ctx context.Context, horus, nameOrID, user, password string, force bool) error {
	node, err := m.getNode(nameOrID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil
		}

		return err
	}

	if !force {
		if err := node.removeCondition(); err != nil {
			return err
		}
	}

	copy := node.node
	copy.Status = statusNodeDisable

	err = node.no.SetNodeParams(copy)
	if err != nil {
		return err
	}

	config, err := m.dco.GetSysConfig()
	if err != nil {
		return err
	}

	client, err := scplib.NewScpClient(node.node.Addr, user, password)
	if err != nil {
		return err
	}
	defer client.Close()

	select {
	default:
	case <-ctx.Done():
		return ctx.Err()
	}

	err = node.nodeClean(ctx, client, horus, config)
	if err != nil {
		return err
	}

	err = m.removeNode(node.node.ID)

	return err
}

func (n *Node) nodeClean(ctx context.Context, client scplib.ScpClient, horus string, config database.SysConfig) error {
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
		"host":        n.node.Addr,
		"source":      srcFile,
		"destination": destName,
	})

	select {
	default:
	case <-ctx.Done():
		return ctx.Err()
	}

	if err := client.UploadFile(destName, srcFile); err != nil {
		entry.Errorf("SSH UploadFile %s Error,%s", srcFile, err)

		if err := client.UploadFile(destName, srcFile); err != nil {
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

	select {
	default:
	case <-ctx.Done():
		return ctx.Err()
	}

	script := fmt.Sprintf("chmod 755 %s && %s %s %d %s %s %s %s",
		destName, destName, n.node.Addr, config.ConsulPort, n.node.ID,
		horusIP, horusPort, config.NFSOption.MountDir)

	out, err := client.Exec(script)
	if err != nil {
		entry.Errorf("exec remote command: %s,%v,Output:%s", script, err, out)

		out, err := client.Exec(script)
		if err != nil {
			entry.Errorf("exec remote command twice: %s,%v,Output:%s", script, err, out)
			return err
		}
	}

	entry.Infof("SSH Remote Exec Successed! Output:\n%s", out)

	return nil
}
