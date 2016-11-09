#!/bin/bash
set -o nounset

adm_ip=$1
consul_port=$2
node_id=$3
horus_server_ip=$4
horus_server_port=$5
backup_dir=$6
hdd_vgname=${HOSTNAME}_HDD_VG
ssd_vgname=${HOSTNAME}_SSD_VG


remove_vg() {
	local vg_name=$1
	vgs ${vg_name} >/dev/null 2>&1
	if [ $? -eq 0 ]; then
		vgremove -f ${vg_name}
	fi
}

umount_backup_dir() {
	umount -f $backup_dir
}


dereg_to_horus_server() {
	local component_type=$1

	stat_code=`curl -o /dev/null -s -w %{http_code} -X POST -H "Content-Type: application/json" -d '{"name": "'${node_id}':'${component_type}'"}' http://${horus_server_ip}:${horus_server_port}/v1/component/deregister`
	if [ "${stat_code}" != "200" ]; then
		echo "${component_type} deregister to horus server failed"
		exit 2
	fi
}

remove_check_script() {
	local dir=/opt/DBaaS/script
	rm -rf ${dir}
}


dereg_to_consul() {
	local component_type=$1

	stat_code=`curl -o /dev/null -s -w %{http_code} -X POST -H "Content-Type: application/json" http://${adm_ip}:${consul_port}/v1/agent/service/deregister/${node_id}':'${component_type}`
	if [ "${stat_code}" != "200" ]; then
		echo "${component_type} deregister to consul failed"
	fi
}

remove_consul() {
	
	# stop consul
	systemctl stop consul.service >/dev/null 2>&1
	pkill -9 consul >/dev/null 2>&1

	# remove data dir
        rm -rf /usr/local/consul/*
        rm -rf /etc/consul.d
	rm -rf  /etc/consul.d/config.json
	rm -rf /usr/bin/consul
	rm -rf /etc/sysconfig/consul
	rm -rf /usr/lib/systemd/system/consul.service
	rm -rf /var/lib/docker
}

remove_docker() {
	# stop docker
	systemctl stop docker.service >/dev/null 2>&1
	pkill -9 docker >/dev/null 2>&1
	rm -rf /usr/bin/docker /usr/bin/docker-containerd /usr/bin/docker-containerd-shim /usr/bin/docker-containerd-ctr /usr/bin/docker-runc
	rm -rf /etc/sysconfig/docker
	rm -rf /usr/lib/systemd/system/docker.service
	rm -rf /usr/lib/systemd/system/docker.socket
	rm -rf /etc/docker/
}

remove_docker_plugin() {
	systemctl stop local-volume-plugin.service >/dev/null 2>&1
	pkill -9 local-volume-plugin > /dev/null 2>&1
	rm -rf /usr/local/local_volume_plugin
	rm -rf /usr/bin/local_volume_plugin
	rm -rf /usr/lib/systemd/system/local-volume-plugin.service
}

remove_swarm_agent() {
	# stop swarm-agent
	systemctl stop swarm-agent.service >/dev/null 2>&1
	pkill -9 swarm >/dev/null 2>&1
	rm -rf /usr/bin/swarm
	rm -rf /etc/sysconfig/swarm-agent
	rm -rf /usr/lib/systemd/system/swarm-agent.service
}

remove_horus_agent() {
	# stop swarm-agent
	systemctl stop horus-agent.service >/dev/null 2>&1
	pkill -9 horus-agent >/dev/null 2>&1
	rm -rf /usr/local/horus-agent
	rm -rf /usr/bin/horus-agent
	rm -rf /etc/sysconfig/horus-agent
	rm -rf /usr/lib/systemd/system/horus-agent.service
}


umount_backup_dir
dereg_to_consul DockerPlugin
dereg_to_horus_server DockerPlugin 
dereg_to_consul Docker
dereg_to_horus_server Docker
dereg_to_consul SwarmAgent
dereg_to_horus_server SwarmAgent
dereg_to_consul HorusAgent 
dereg_to_horus_server HorusAgent

remove_docker_plugin
remove_docker
remove_swarm_agent
remove_check_script
remove_horus_agent
remove_consul
remove_vg ${hdd_vgname}
remove_vg ${ssd_vgname}

exit 0
