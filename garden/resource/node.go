package resource

import (
	"context"
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

var dockerNodesKVPath = "docker/nodes/KVPath"

type Node struct {
	node database.Node
	eng  *cluster.Engine
}

func newNode(n database.Node, eng *cluster.Engine) Node {
	return Node{
		node: n,
		eng:  eng,
	}
}

type nodeWithTask struct {
	hdd    []string
	ssd    []string
	client scplib.ScpClient
	Node   database.Node
	Task   database.Task
}

func NewNodeWithTask(user, password string, hdd []string, n database.Node) (nodeWithTask, error) {
	c, err := scplib.NewScpClient(n.Addr, user, password)
	if err != nil {
		return nodeWithTask{}, err
	}

	t := database.NewTask()

	return nodeWithTask{
		hdd:    hdd,
		client: c,
		Node:   n,
		Task:   t,
	}, nil
}

func NewNodeWithTaskList(len int) []nodeWithTask {
	return make([]nodeWithTask, len)
}

func (nt *nodeWithTask) distribute(ctx context.Context, ormer database.ClusterOrmer, config database.SysConfig) (err error) {
	entry := logrus.WithFields(logrus.Fields{
		"Node": nt.Node.Name,
		"host": nt.Node.Addr,
	})

	nodeState := int64(statusNodeInstalling)

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
		if err != nil {
			entry.Errorf("%+v", _err)
		}
	}()

	script, err := nt.modifyProfile(config)
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
func (node *nodeWithTask) modifyProfile(config database.SysConfig) (string, error) {
	//	horus, err := getHorusFromConsul()
	//	if err != nil {
	//		return nil, "", err
	//	}

	// TODO:
	horus := "horus address"

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
func (dc *Datacenter) registerNodes(ctx context.Context, nodes []nodeWithTask, config database.SysConfig) {
	d := 30 * time.Second
	timer := time.NewTimer(d)

	defer func(t *time.Timer) {
		if !timer.Stop() {
			<-t.C
		}
	}(timer)

	for {
		select {
		case <-timer.C:
			timer.Reset(d)

		case <-ctx.Done():
			err := dc.registerNodesTimeout(nodes, ctx.Err())
			logrus.Errorf("%+v", err)

			return
		}

		count := 0
		for i := range nodes {
			fields := logrus.WithFields(logrus.Fields{
				"Node": nodes[i].Node.Name,
				"addr": nodes[i].Node.Addr,
			})

			n, err := dc.dco.GetNode(nodes[i].Node.ID)
			if err != nil {
				fields.Warnf("%+v", err)
				continue
			}

			if n.Status != statusNodeInstalled {
				if n.Status > statusNodeInstalled {
					count++
					if count >= len(nodes) {
						return
					}
				}

				fields.Warnf("status not match,%d!=%d", n.Status, statusNodeInstalled)
				continue
			}

			addr := n.Addr + ":" + strconv.Itoa(config.DockerPort)
			eng := dc.clsuter.EngineByAddr(addr)
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

			err = dc.dco.RegisterNode(n, t)
			if err != nil {
				fields.Errorf("%+v", err)
			}
		}
	}
}

func (dc *Datacenter) registerNodesTimeout(nodes []nodeWithTask, er error) error {
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

	list, err := dc.dco.ListNodeByCluster(cID)
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

		err = dc.dco.RegisterNode(n, t)
		if err != nil {
			logrus.WithField("Node", n.Name).WithError(err).Error("Node register timeout")
		}
	}

	return nil
}

//func nodeClean(node, addr, user, password string) error {
//	config, err := database.GetSystemConfig()
//	if err != nil {
//		return err
//	}

//	horus, err := getHorusFromConsul()
//	if err != nil {
//		return err
//	}

//	horusIP, horusPort, err := net.SplitHostPort(horus)
//	if err != nil {
//		return errors.Wrap(err, "check Horus Addr:"+horus)
//	}

//	_, _, destName := config.DestPath()

//	srcFile, err := utils.GetAbsolutePath(false, config.SourceDir, config.CleanScriptName)
//	if err != nil {
//		logrus.Errorf("%s %s", srcFile, err)

//		return errors.Wrap(err, "get absolute path")
//	}

//	entry := logrus.WithFields(logrus.Fields{
//		"host":        addr,
//		"user":        user,
//		"source":      srcFile,
//		"destination": destName,
//	})

//	c, err := scplib.NewClient(addr, user, password)
//	if err != nil {
//		return err
//	}
//	defer c.Close()

//	if err := c.UploadFile(destName, srcFile); err != nil {
//		entry.Errorf("SSH UploadFile %s Error,%s", srcFile, err)

//		if err := c.UploadFile(destName, srcFile); err != nil {
//			entry.Errorf("SSH UploadFile %s Error Twice,%s", srcFile, err)

//			return err
//		}
//	}

//	/*
//		adm_ip=$1
//		consul_port=${2}
//		node_id=${3}
//		horus_server_ip=${4}
//		horus_server_port=${5}
//		backup_dir = ${6}
//	*/

//	script := fmt.Sprintf("chmod 755 %s && %s %s %d %s %s %s %s",
//		destName, destName, addr, config.ConsulPort, node,
//		horusIP, horusPort, config.NFSOption.MountDir)

//	out, err := c.Exec(script)
//	if err != nil {
//		entry.Errorf("exec remote command: %s,%v,Output:%s", script, err, out)

//		out, err := c.Exec(script)
//		if err != nil {
//			entry.Errorf("exec remote command twice: %s,%v,Output:%s", script, err, out)
//			return err
//		}
//	}

//	entry.Infof("SSH Remote Exec Successed! Output:\n%s", out)

//	return nil
//}
