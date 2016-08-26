#!/bin/bash
LANG=POSXIA

usage=`mpstat 1 5 2>/dev/null | grep "Average" | awk '{print 100 -$NF}'`
if [ "${usage}" == "" ];then
	echo "get data fail"
	exit 2
fi
echo "${usage}"
