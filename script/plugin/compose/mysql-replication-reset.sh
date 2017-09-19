#!/bin/bash
set -o nounset

container_id=$1
swarm_ip=$2
swarm_port=$3

CUR_DIR=`dirname $0`
TOOLS_DIR=${CUR_DIR}/tools
DOCKERBIN=${TOOLS_DIR}/docker

reset_master="RESET MASTER;"
reset_slave="RESET SLAVE;"

${DOCKERBIN} -H $swarm_ip:$swarm_port exec $container_id mysql -e "$reset_slave" && \
${DOCKERBIN} -H $swarm_ip:$swarm_port exec $container_id mysql -e "$reset_master"
