#!/bin/bash
set -o nounset

container_id=$1
swarm_ip=$2
swarm_port=$3

DOCKERBIN=docker

reset_master="RESET MASTER;"
reset_slave="RESET SLAVE;"

docker -H $swarm_ip:$swarm_port exec $container_id mysql -e "$reset_slave" && \
docker -H $swarm_ip:$swarm_port exec $container_id mysql -e "$reset_master"
