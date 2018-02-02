#!/bin/bash

for fc_host in `ls /sys/class/fc_host/`
do
	echo 1 > /sys/class/fc_host/${fc_host}/issue_lip
done
