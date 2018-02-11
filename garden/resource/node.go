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
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/resource/storage"
	"github.com/docker/swarm/garden/scplib"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/utils"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

const _SAN_HBA_WWN_Lable = "HBA_WWN"

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

var dockerNodesKVPath = "/docker/swarm/nodes"

// SetNodesKVPath set kv path of nodes
func SetNodesKVPath(path string) {
	dockerNodesKVPath = path
}

// Node represents a host
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
	errs := make([]string, 0, 3)

	if n.node.EngineID != "" {
		count, err := n.no.CountUnitByEngine(n.node.EngineID)
		if err != nil || count > 0 {
			errs = append(errs, fmt.Sprintf("Node %s is in using(%d) or error happens,%+v", n.node.Addr, count, err))
		}
	}

	if n.eng != nil {
		err := n.eng.RefreshContainers(true)
		if err != nil {
			errs = append(errs, err.Error())
		}

		if num := len(n.eng.Containers()); num > 0 {
			errs = append(errs, fmt.Sprintf("%d containers exists in Node %s", num, n.node.Addr))
		}

		err = n.eng.RefreshVolumes()
		if err != nil {
			errs = append(errs, err.Error())
		}

		if num := len(n.eng.Volumes()); num > 0 {
			errs = append(errs, fmt.Sprintf("%d volumes exists in Node %s", num, n.node.Addr))
		}
	}

	if len(errs) == 0 {
		return nil
	}

	return errors.Errorf("warnings:%s", errs)
}

func (m hostManager) getNode(nameOrID string) (Node, error) {
	n, err := m.dco.GetNode(nameOrID)
	if err != nil {
		return Node{}, err
	}

	if n.EngineID == "" {
		return newNode(n, nil, m.dco), nil
	}

	eng := m.ec.Engine(n.EngineID)

	return newNode(n, eng, m.dco), nil
}

type nodeWithTask struct {
	hdd     []string
	ssd     []string
	config  structs.SSHConfig
	client  scplib.ScpClient
	Node    database.Node
	Task    database.Task
	timeout time.Duration
}

// NewNodeWithTask node with task and ssh login config,prepare for install host
func NewNodeWithTask(n database.Node, hdd, ssd []string, ssh structs.SSHConfig) nodeWithTask {

	t := database.NewTask(n.Addr, database.NodeInstall, n.ID, "install softwares on host", nil, 300)

	return nodeWithTask{
		hdd:    hdd,
		ssd:    ssd,
		config: ssh,
		client: nil,
		Node:   n,
		Task:   t,
	}
}

// NewNodeWithTaskList returns len of slice nodeWithTask
func NewNodeWithTaskList(len int) []nodeWithTask {
	return make([]nodeWithTask, len)
}

// InstallNodes install new nodes,list should has same ClusterID
func (m hostManager) InstallNodes(ctx context.Context, horus string, reg kvstore.Register) error {
	nodes := make([]database.Node, len(m.nodes))
	tasks := make([]database.Task, len(m.nodes))
	timeout := 250*time.Second + time.Duration(len(m.nodes)*30)*time.Second

	for i := range m.nodes {
		nodes[i] = m.nodes[i].Node
		tasks[i] = m.nodes[i].Task
		tasks[i].Timeout = timeout
		m.nodes[i].timeout = timeout
	}

	select {
	default:
	case <-ctx.Done():
		return ctx.Err()
	}

	config, err := m.dco.GetSysConfig()
	if err != nil {
		return err
	}

	err = m.dco.InsertNodesAndTask(nodes, tasks)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)

	for i := range m.nodes {
		go m.nodes[i].distribute(ctx, horus, m.dco, config)
	}

	go m.registerNodesLoop(ctx, cancel, config, reg)

	return nil
}

func (nt *nodeWithTask) distribute(ctx context.Context, horus string, ormer database.ClusterOrmer, config database.SysConfig) (err error) {
	entry := logrus.WithFields(logrus.Fields{
		"host": nt.Node.Addr,
	})

	nodeState := statusNodeInstalling

	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("panic:%v", r)
		}

		if err == nil {
			nt.Node.Status = statusNodeInstalled
			nt.Task.Status = database.TaskRunningStatus
		} else {
			if nodeState == statusNodeInstalling {
				nt.Node.Status = statusNodeInstallFailed
				nt.Node.Enabled = false
			} else {
				nt.Node.Status = nodeState
			}

			nt.Task.Status = database.TaskFailedStatus
			nt.Task.SetErrors(err)
			nt.Task.FinishedAt = time.Now()
		}

		_err := ormer.RegisterNode(&nt.Node, &nt.Task)
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

	if nt.client == nil {
		addr := nt.Node.Addr
		if nt.config.Port > 0 {
			addr = net.JoinHostPort(nt.Node.Addr, strconv.Itoa(nt.config.Port))
		}

		nt.client, err = scplib.NewScpClient(addr, nt.config.Username, nt.config.Password, nt.timeout)
		if err != nil {
			entry.WithError(err).Error("ssh dial error")

			nodeState = statusNodeSSHLoginFailed

			return err
		}
	}
	defer nt.client.Close()

	select {
	default:
	case <-ctx.Done():
		return ctx.Err()
	}

	if err := nt.client.UploadDir(config.Destination, config.SourceDir); err != nil {
		entry.WithError(err).Error("SSH upload dir:" + config.SourceDir)

		if err := nt.client.UploadDir(config.Destination, config.SourceDir); err != nil {
			entry.WithError(err).Error("SSH upload dir twice:" + config.SourceDir)

			nodeState = statusNodeSCPFailed
			return err
		}
	}

	_, filename, _ := config.DestPath()

	if err := nt.client.Upload(config.Registry.CACert, filename, 0644); err != nil {
		entry.WithError(err).Error("SSH upload file:" + filename)

		if err := nt.client.Upload(config.Registry.CACert, filename, 0644); err != nil {
			entry.WithError(err).Error("SSH upload file twice:" + filename)

			nodeState = statusNodeSCPFailed
			return err
		}
	}

	select {
	default:
	case <-ctx.Done():
		return errors.WithStack(ctx.Err())
	}

	entry.Debug("SSH Exec:", script)

	out, err := nt.client.Exec(script)
	if err != nil {
		entry.WithError(err).Errorf("exec remote command:'%s',output:%s", script, out)

		nodeState = statusNodeSSHExecFailed
		return err
	}

	entry.Infof("SSH remote PKG install successed! output:\n%s", out)

	return nil
}

// CA,script,error
func (nt *nodeWithTask) modifyProfile(horus string, config *database.SysConfig) (string, error) {
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
		set -o nounset

		swarm_key=${1}
		adm_ip=${2}
		cs_datacenter=${3}
		cs_list=${4}
		registry_domain=${5}
		registry_ip=${6}
		registry_port=${7}
		registry_username=${8}
		registry_passwd=${9}
		registry_ca_file=${10}
		docker_port=${11}
		hdd_dev=${12}
		ssd_dev=${13}
		consul_port=${14}
		node_id=${15}
		horus_server_ip=${16}
		horus_server_port=${17}
		swarm_agent_port=${18}
		nfs_ip=${19}
		nfs_dir=${20}
		nfs_mount_dir=${21}
		nfs_mount_opts=${22}
		san_id=${23}
	*/
	hdd, ssd, store := "null", "null", "null"
	if len(nt.hdd) > 0 {
		hdd = strings.Join(nt.hdd, ",")
	}
	if len(nt.ssd) > 0 {
		ssd = strings.Join(nt.ssd, ",")
	}
	if nt.Node.Storage != "" {
		nt.Node.Storage = store
	}

	script := fmt.Sprintf(`chmod 755 %s && %s %s %s %s '%s' %s %s %d %s %s %s %d %s %s %d %s %s %s %d %s %s %s %s %s`,
		path, path, dockerNodesKVPath, nt.Node.Addr, config.ConsulDatacenter, string(buf),
		config.Registry.Domain, config.Registry.Address, config.Registry.Port,
		config.Registry.Username, config.Registry.Password, caFile,
		config.Ports.Docker, hdd, ssd, config.ConsulPort,
		nt.Node.ID, horusIP, horusPort, config.Ports.SwarmAgent,
		nt.Node.NFS.Addr, nt.Node.NFS.Dir, nt.Node.NFS.MountDir, nt.Node.NFS.Options, store)

	return script, nil
}

// registerNodes register Nodes
func (m hostManager) registerNodesLoop(ctx context.Context, cancel context.CancelFunc, sys database.SysConfig, reg kvstore.Register) {
	defer cancel()

	t := time.NewTicker(time.Second * 30)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			m.registerNodes(ctx, sys, reg)

		case <-ctx.Done():
			// try again
			m.registerNodes(ctx, sys, reg)

			err := m.registerNodesTimeout(ctx.Err())
			logrus.Errorf("deal with Nodes timeout%+v", err)

			return
		}
	}
}

//func (m *hostManager) removeNodeTask(id string) {
//	list := make([]nodeWithTask, 0, len(m.nodes))

//	for i := range m.nodes {
//		if m.nodes[i].Node.ID != id {
//			list = append(list, m.nodes[i])
//		}
//	}

//	m.nodes = list
//}

func (m hostManager) registerNodes(ctx context.Context, sys database.SysConfig, reg kvstore.Register) {
	for _, node := range m.nodes {

		field := logrus.WithField("host", node.Node.Addr)

		n, err := m.dco.GetNode(node.Node.ID)
		if err != nil {
			field.Warnf("%+v", err)

			//			if database.IsNotFound(err) {
			//				t := node.Task
			//				t.Status = database.TaskFailedStatus
			//				t.FinishedAt = time.Now()
			//				t.SetErrors(err)

			//				err = m.dco.RegisterNode(nil, &t)
			//				if err != nil {
			//					field.Errorf("%+v", err)
			//				}

			//				m.removeNodeTask(n.ID)
			//			}

			continue
		}

		// node.Node = n

		if n.Status != statusNodeInstalled {
			if n.Status > statusNodeInstalled {
				continue
			}

			field.Warnf("status not match,%d!=%d", n.Status, statusNodeInstalled)
			continue
		}

		addr := fmt.Sprintf("%s:%d", n.Addr, sys.Docker)
		eng := m.ec.EngineByAddr(addr)
		if eng == nil || !eng.IsHealthy() {
			field.Errorf("engine:%s is nil or unhealthy,engine=%v", addr, eng)
			continue
		}

		// register Node to SAN storage
		if n.Storage != "" {
			san, err := storage.DefaultStores().Get(n.Storage)
			if err != nil {
				continue
			}

			wwn := eng.Labels[_SAN_HBA_WWN_Lable]
			list := strings.Split(wwn, ",")

			if err = san.AddHost(eng.ID, list...); err != nil {
				field.Errorf("register to SAN,WWN:%s,%+v", wwn, err)
				continue
			}
		}

		err = registerHost(ctx, node, reg, eng.Labels["CONTAINER_NIC"])
		if err != nil {
			field.Errorf("%+v", err)
			continue
		}

		n.EngineID = eng.ID
		n.Status = statusNodeEnable
		n.Enabled = true
		n.RegisterAt = time.Now()

		t := node.Task
		t.Status = database.TaskDoneStatus
		t.FinishedAt = n.RegisterAt
		t.SetErrors(nil)

		err = m.dco.RegisterNode(&n, &t)
		if err != nil {
			field.Errorf("%+v", err)
		} /*else {
			m.removeNodeTask(n.ID)
		}*/
	}
}

func registerHost(ctx context.Context, node nodeWithTask, reg kvstore.Register, dev string) error {
	addr := node.Node.Addr
	if node.config.Port > 0 {
		addr = net.JoinHostPort(node.Node.Addr, strconv.Itoa(node.config.Port))
	}

	body := structs.HorusRegistration{}

	body.Node.Select = true
	body.Node.Name = node.Node.ID

	body.Node.IPAddr = addr
	body.Node.OSUser = node.config.Username
	body.Node.OSPassword = node.config.Password
	body.Node.CheckType = "health"
	body.Node.NetDevice = strings.Split(dev, ",")

	err := reg.RegisterService(ctx, "", structs.ServiceRegistration{Horus: &body})

	return err
}

func (m hostManager) registerNodesTimeout(err error) error {
	if len(m.nodes) == 0 {
		return nil
	}

	for i := range m.nodes {
		t := m.nodes[i].Task
		n := m.nodes[i].Node

		n1, err := m.dco.GetNode(n.ID)
		if err != nil {
			logrus.WithField("host", n.Addr).Errorf("%+v", err)

			if database.IsNotFound(err) {
				t.Status = database.TaskFailedStatus
				t.FinishedAt = time.Now()
				t.SetErrors(err)

				err = m.dco.RegisterNode(nil, &t)
				if err != nil {
					logrus.WithField("host", n.Addr).Errorf("%+v", err)
				}
			}

			continue
		}

		n = n1

		if n.Status >= statusNodeEnable {
			continue
		}

		if n.Status == statusNodeInstalled {
			n.Status = statusNodeRegisterTimeout
			n.Enabled = false
			n.RegisterAt = time.Now()

			t.Status = database.TaskFailedStatus
			t.FinishedAt = n.RegisterAt
			t.SetErrors(err)
		}

		err = m.dco.RegisterNode(&n, &t)
		if err != nil {
			logrus.WithField("Addr", n.Addr).WithError(err).Error("Node register timeout")
		}
	}

	return nil
}

func (m hostManager) removeNode(ID string) error {

	return m.dco.DelNode(ID)
}

// RemoveNode
func (m hostManager) RemoveNode(ctx context.Context, port, horus, nameOrID, user, password string, force bool, timeout time.Duration, reg kvstore.Register) error {
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

	err = node.no.SetNodeEnable(node.node.ID, false)
	if err != nil {
		return err
	}

	if node.node.Status == statusNodeSCPFailed ||
		node.node.Status == statusNodeSSHLoginFailed {

		return m.removeNode(node.node.ID)
	}

	addr := node.node.Addr
	if port != "" {
		addr = net.JoinHostPort(node.node.Addr, port)
	}

	err = reg.DeregisterService(ctx, structs.ServiceDeregistration{
		Type:     "hosts",
		Key:      node.node.ID,
		Addr:     addr,
		User:     user,
		Password: password,
	}, true)
	if err != nil {
		return err
	}

	config, err := m.dco.GetSysConfig()
	if err != nil {
		return err
	}

	client, err := scplib.NewScpClient(addr, user, password, timeout)
	if err != nil {
		return err
	}
	defer client.Close()

	select {
	default:
	case <-ctx.Done():
		return errors.WithStack(ctx.Err())
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

		return errors.WithStack(err)
	}

	entry := logrus.WithFields(logrus.Fields{
		"host":        n.node.Addr,
		"source":      srcFile,
		"destination": destName,
	})

	select {
	default:
	case <-ctx.Done():
		return errors.WithStack(ctx.Err())
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
		horusIP, horusPort, n.node.NFS.MountDir)

	entry.Debug("SSH Exec:", script)

	out, err := client.Exec(script)
	if err != nil {
		entry.Errorf("exec remote command: %s,%v,Output:%s", script, err, out)

		return err
	}

	entry.Infof("SSH Remote Exec Successed! Output:\n%s", out)

	return nil
}
