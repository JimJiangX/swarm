#!/bin/bash
set -o nounset

container_id=$1
swarm_ip=$2
swarm_port=$3
role=$4
master_ip=$5
master_port=$6
repl_user=$7
repl_pwd=$8
slave_ip=$9
slave_port=${10}

CUR_DIR=`dirname $0`
TOOLS_DIR=${CUR_DIR}/tools
DOCKERBIN=${TOOLS_DIR}/docker

stop_slave="STOP SLAVE;"

change_master="CHANGE MASTER TO MASTER_HOST = '$master_ip', MASTER_PORT = $master_port, MASTER_USER = '$repl_user', MASTER_PASSWORD = '$repl_pwd', MASTER_AUTO_POSITION = 1;"

start_slave="START SLAVE;"

set_replication() {
	${DOCKERBIN} -H $swarm_ip:$swarm_port exec $container_id mysql -e "$stop_slave" && \
	${DOCKERBIN} -H $swarm_ip:$swarm_port exec $container_id mysql -e "$change_master" && \
	${DOCKERBIN} -H $swarm_ip:$swarm_port exec $container_id mysql -e "$start_slave"
}

if [ "$role" == MASTER ]; then
	echo "role is master, nothing to do"
	exit 0
elif [ "$role" == SLAVE ]; then
	set_replication
else 
	echo "${self_role} role unspport, role "
	echo "Invalid attribute in role: ${role}" >&2
	exit 1
fi

