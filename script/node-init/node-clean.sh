#!/bin/bash

adm_ip=$1
consul_port=$2
node_id=$3
horus_server_ip=$4
horus_server_port=$5

dereg_to_horus_server() {
	local component_type=$1

	curl -X POST -H "Content-Type: application/json" -d '{"name": "'${node_id}':'${component_type}'"}' http://${horus_server_ip}:${horus_server_port}/v1/component/register
	if [ $? != 0 ]; then
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

	curl -X POST -H "Content-Type: application/json" http://${adm_ip}:${consul_port}/v1/agent/service/deregister/${node_id}':'${component_type}

	if [ $? != 0 ]; then
		echo "${component_type} deregister to consul failed"
		exit 2
	fi
}

remove_consul() {
	
	# stop consul
	pkill -9 consul >/dev/null 2>&1

	# remove data dir
        rm -rf /usr/local/consul/*
        rm -rf /etc/consul.d
	rm -rf  /etc/consul.d/config.json
	rm -rf /usr/bin/consul
	rm -rf /etc/sysconfig/consul
	rm -rf /usr/lib/systemd/system/consul.service
}

remove_docker() {
	# stop docker
	pkill -9 docker >/dev/null 2>&1
	rm -rf /usr/bin/docker
	rm -rf /etc/sysconfig/docker
	rm -rf /usr/lib/systemd/system/docker.service
	rm -rf /usr/lib/systemd/system/docker.socket
	rm -rf /etc/docker/
	rm -rf /var/lib/docker
}

remove_docker_plugin() {
	pkill -9 local-volume-plugin > /dev/null 2>&1
	rm -rf /usr/local/local_volume_plugin
	rm -rf /usr/bin/local_volume_plugin
	rm -rf /usr/lib/systemd/system/local-volume-plugin.service
}

remove_swarm_agent() {
	# stop swarm-agent
	pkill -9 swarm >/dev/null 2>&1
	rm -rf /usr/bin/swarm
	rm -rf /etc/sysconfig/swarm-agent
	rm -rf /usr/lib/systemd/system/swarm-agent.service
}

remove_horus_agent() {
	# stop swarm-agent
	pkill -9 horus-agent >/dev/null 2>&1
	rm -rf /usr/local/horus-agent
	rm -rf /usr/bin/horus-agent
	rm -rf /etc/sysconfig/horus-agent
	rm -rf /usr/lib/systemd/system/horus-agent.service
}



remove_check_script
remove_consul
remove_docker_plugin
dereg_to_consul DockerPlugin
dereg_to_horus_server DockerPlugin 
remove_docker
dereg_to_consul Docker
dereg_to_horus_server Docker
remove_swarm_agent
dereg_to_consul SwarmAgent
dereg_to_horus_server SwarmAgent
remove_horus_agent
dereg_to_consul HorusAgent 
dereg_to_horus_server HorusAgent

exit 0
