#!/usr/bin/env bash
set -o nounset

if [ $# -ne 1 ];then
        echo "args number must eqaul to 1"
        exit 2
fi

#nodelist=192.168.2.100:6375,192.168.2.101:6375,192.168.2.102:6375
node_list=$1
default_pass=dbaas

master_node=ip="${node_list%%,*}"
master_ip="${master_node%%:*}"
master_port="${master_node##*:}"

MODE=sharding_replication
CUR_DIR=`dirname $0`
TOOLS_DIR=${CUR_DIR}/tools

if [ ! -x ${TOOLS_DIR}/redis-cli ]; then
    echo "${TOOLS_DIR}/redis-cli not exist"
	exit 2
fi

node_arr=(${node_list//,/ })

for node in ${node_arr[@]}
do
    ip=${node%%:*}
    if [ "${ip}" == "${master_ip}" ]; then
        continue
    fi
    port=${node##*:}
    ${TOOLS_DIR}/redis-cli -h ${ip} -p ${port} -a ${default_pass} CONFIG SET slaveof ${master_ip} ${master_port}
    if [ $? -ne 0 ]; then
        echo "redis($node) set failed"
        exit 3
    fi

    ${TOOLS_DIR}/redis-cli -h ${ip} -p ${port} -a ${default_pass} CONFIG REWRITE
    if [ $? -ne 0 ]; then
        echo "redis($node) config rewrite failed"
        exit 3
    fi
done
