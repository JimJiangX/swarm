#!/usr/bin/env bash
set -o nounset

if [ $# -ne 1 ];then
        echo "args number must eqaul to 1"
        exit 2
fi

#nodelist=192.168.2.100:6375,192.168.2.101:6375,192.168.2.102:6375
node_list=$1
default_pass=5aiup_rd

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
    # check network
    ping -w 6 -c 3 ${ip}
    if [ $? -ne 0 ]; then
        echo "ping ${ip} failed"
        exit 4
    fi
    port=${node##*:}
    rel=$(${TOOLS_DIR}/redis-cli -h ${ip} -p ${port} -a ${default_pass} flushall | grep OK | wc -l)
    if [ ${rel} -ne 1 ]; then
        echo "redis($node) flashall failed"
        exit 3
    fi
done
