package resource

import (
	"encoding/json"
	"fmt"
	"net"
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

var dockerNodesKVPath = ""

func SetNodesKVPath(path string) {
	dockerNodesKVPath = path
}

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

	count, err := n.no.CountUnitByEngine(n.node.EngineID)
	if err != nil || count > 0 {
		errs = append(errs, fmt.Sprintf("Node %s is in using(%d) or error happens,%+v", n.node.Addr, count, err))
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
	hdd    []string
	ssd    []string
	config structs.SSHConfig
	client scplib.ScpClient
	Node   database.Node
	Task   database.Task
}

func NewNodeWithTask(n database.Node, hdd, ssd []string, ssh structs.SSHConfig) nodeWithTask {

	t := database.NewTask(n.Addr, database.NodeInstall, n.ID, "install softwares on host", "", 300)

	return nodeWithTask{
		hdd:    hdd,
		ssd:    ssd,
		config: ssh,
		client: nil,
		Node:   n,
		Task:   t,
	}
}

func NewNodeWithTaskList(len int) []nodeWithTask {
	return make([]nodeWithTask, len)
}

// InstallNodes install new nodes,list should has same ClusterID
func (m hostManager) InstallNodes(ctx context.Context, horus string, list []nodeWithTask, reg kvstore.Register) error {
	nodes := make([]database.Node, len(list))
	tasks := make([]database.Task, len(list))
	timeout := 250*time.Second + time.Duration(len(list)*30)*time.Second

	for i := range list {
		nodes[i] = list[i].Node
		tasks[i] = list[i].Task
		tasks[i].Timeout = timeout
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

	for i := range list {
		go list[i].distribute(ctx, horus, m.dco, config)
	}

	go m.registerNodesLoop(ctx, cancel, list, config, reg)

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

	if nt.client == nil {
		nt.client, err = scplib.NewScpClient(nt.Node.Addr, nt.config.Username, nt.config.Password)
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
		return ctx.Err()
	}

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
		consul_port=${14}
		node_id=${15}
		horus_server_ip=${16}
		horus_server_port=${17}
		docker_plugin_port=${18}
		swarm_agent_port=${19}
		nfs_ip=${20}
		nfs_dir=${21}
		nfs_mount_dir=${22}
		nfs_mount_opts=${23}
	*/
	hdd, ssd := "null", "null"
	if len(nt.hdd) > 0 {
		hdd = strings.Join(nt.hdd, ",")
	}
	if len(nt.ssd) > 0 {
		ssd = strings.Join(nt.ssd, ",")
	}

	script := fmt.Sprintf(`chmod 755 %s && %s %s %s %s '%s' %s %s %d %s %s %s %d %s %s %d %s %s %s %d %d %s %s %s %s`,
		path, path, dockerNodesKVPath, nt.Node.Addr, config.ConsulDatacenter, string(buf),
		config.Registry.Domain, config.Registry.Address, config.Registry.Port,
		config.Registry.Username, config.Registry.Password, caFile,
		config.Ports.Docker, hdd, ssd, config.ConsulPort,
		nt.Node.ID, horusIP, horusPort, config.Ports.Plugin, config.Ports.SwarmAgent,
		nt.Node.NFS.Addr, nt.Node.NFS.Dir, nt.Node.NFS.MountDir, nt.Node.NFS.Options)

	return script, nil
}

// registerNodes register Nodes
func (m hostManager) registerNodesLoop(ctx context.Context, cancel context.CancelFunc,
	nodes []nodeWithTask, sys database.SysConfig, reg kvstore.Register) {

	defer cancel()

	t := time.NewTicker(time.Second * 30)
	defer t.Stop()

	for {
		select {
		case <-t.C:

		case <-ctx.Done():
			// try again
			err := m.registerNodes(ctx, nodes, sys, reg)
			if err != nil {
				logrus.Errorf("reigster nodes error,%+v", err)
			}

			err = m.registerNodesTimeout(nodes, ctx.Err())
			logrus.Errorf("deal with Nodes timeout%+v", err)

			return
		}

		err := m.registerNodes(ctx, nodes, sys, reg)
		if err != nil {
			logrus.Errorf("reigster nodes error,%+v", err)
		}
	}
}

func (m hostManager) registerNodes(ctx context.Context, nodes []nodeWithTask, sys database.SysConfig, reg kvstore.Register) error {
	var (
		_err  error
		count int
	)

	for i := range nodes {

		n, err := m.dco.GetNode(nodes[i].Node.ID)
		if err != nil {
			return err
		}

		nodes[i].Node = n

		field := logrus.WithField("host", n.Addr)

		if n.Status != statusNodeInstalled {
			if n.Status > statusNodeInstalled {
				count++
				if count >= len(nodes) {
					return nil
				}
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
				_err = err
				field.Errorf("register to SAN,WWN:%s,%+v", wwn, err)
				continue
			}
		}

		err = registerHost(ctx, nodes[i], reg, eng.Labels["CONTAINER_NIC"])
		if err != nil {
			_err = err
			field.Errorf("%+v", err)
			continue
		}

		n.EngineID = eng.ID
		n.Status = statusNodeEnable
		n.Enabled = true
		n.RegisterAt = time.Now()

		t := nodes[i].Task
		t.Status = database.TaskDoneStatus
		t.FinishedAt = n.RegisterAt
		t.Errors = ""

		err = m.dco.RegisterNode(n, t)
		if err != nil {
			_err = err
			field.Errorf("%+v", err)
		}
	}

	return _err
}

func registerHost(ctx context.Context, node nodeWithTask, reg kvstore.Register, dev string) error {
	body := structs.HorusRegistration{}

	body.Node.Select = true
	body.Node.Name = node.Node.ID

	body.Node.IPAddr = node.Node.Addr
	body.Node.OSUser = node.config.Username
	body.Node.OSPassword = node.config.Password
	body.Node.CheckType = "health"
	body.Node.NetDevice = strings.Split(dev, ",")

	err := reg.RegisterService(ctx, "", structs.ServiceRegistration{Horus: &body})

	return err
}

func (m hostManager) registerNodesTimeout(nodes []nodeWithTask, er error) error {
	if len(nodes) == 0 {
		return nil
	}

	for i := range nodes {
		n, err := m.dco.GetNode(nodes[i].Node.ID)
		if err != nil {
			logrus.WithField("host", nodes[i].Node.Addr).Errorf("%+v", err)
			continue
		}

		nodes[i].Node = n

		if n.Status >= statusNodeEnable {
			continue
		}

		t := nodes[i].Task

		if n.Status == statusNodeInstalled {
			n.Status = statusNodeRegisterTimeout
			n.Enabled = false
			n.RegisterAt = time.Now()

			t.Status = database.TaskFailedStatus
			t.FinishedAt = n.RegisterAt
			t.Errors = er.Error()
		}

		err = m.dco.RegisterNode(n, t)
		if err != nil {
			logrus.WithField("Addr", n.Addr).WithError(err).Error("Node register timeout")
		}
	}

	return nil
}

func (m hostManager) removeNode(ID string) error {

	return m.dco.DelNode(ID)
}

func (m hostManager) RemoveNode(ctx context.Context, horus, nameOrID, user, password string, force bool, reg kvstore.Register) error {
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

	err = reg.DeregisterService(ctx, structs.ServiceDeregistration{
		Type:     "hosts",
		Key:      node.node.ID,
		User:     user,
		Password: password,
	})
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
		horusIP, horusPort, n.node.NFS.MountDir)

	out, err := client.Exec(script)
	if err != nil {
		entry.Errorf("exec remote command: %s,%v,Output:%s", script, err, out)

		return err
	}

	entry.Infof("SSH Remote Exec Successed! Output:\n%s", out)

	return nil
}
