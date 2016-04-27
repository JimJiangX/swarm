#!/bin/bash

usage=`mpstat 2>/dev/null | grep all | awk '{print 100 -$NF}'`

if [ "$usage" = "" ];then
		echo "get data fail"
		exit 2
fi
echo $usage