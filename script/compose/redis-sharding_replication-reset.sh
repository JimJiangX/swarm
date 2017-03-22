#!/bin/bash
set -o nounset

if [ $# -ne 1 ];then
        echo "args number must eqaul to 1"
        exit 2
fi

#nodelist=192.168.2.100:6375,192.168.2.101:6375,192.168.2.102:6375
node_list=$1

MODE=sharding_replication
CUR_DIR=`dirname $0`
TOOLS_DIR=${CUR_DIR}/tools


if [ ! -x ${TOOLS_DIR}/redis-trib.rb ]; then
	echo "${TOOLS_DIR}/redis-trib.rb not exist"
	exit 2
fi

node=${node_list##*,}
${TOOLS_DIR}/redis-trib.rb call ${node} FLUSHALL
if [ $? != 0 ]; then
	echo "exec ${TOOLS_DIR}/redis-trib.rb call ${node} FLUSHALL faild!"
	exit 3
fi

nodes=`echo ${node_list} | sed 's/,/\ /g'`
for node in ${nodes}
do
	${TOOLS_DIR}/redis-trib.rb call ${node} cluster reset soft
	if [ $? != 0 ]; then
		echo "exec ${TOOLS_DIR}/redis-trib.rb call ${node} cluster reset soft faild!"
		exit 3
	fi

	${TOOLS_DIR}/redis-trib.rb call ${node} cluster reset hard
	if [ $? != 0 ]; then
		echo "exec ${TOOLS_DIR}/redis-trib.rb call ${node} cluster reset hard faild!"
		exit 3
	fi
done
