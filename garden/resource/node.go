package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"

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

	nodeState, taskState := int64(statusNodeInstalling), int64(statusTaskRunning)

	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("Recover from Panic:%v", r)
		}

		var msg string
		if err == nil {
			nodeState = statusNodeInstalled
		} else {
			if nodeState == statusNodeInstalling {
				nodeState = statusNodeInstallFailed
			}
			taskState = statusTaskFailed
			msg = err.Error()
		}

		//			r := database.TxUpdateNodeStatus(node.Node, node.task,
		//				nodeState, taskState, msg)
		if err != nil {
			entry.Error(msg)
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

	logrus.Infof("Registry.CA_CRT:%d %s", len(config.Registry.CA_CRT), config.Registry.CA_CRT)

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
