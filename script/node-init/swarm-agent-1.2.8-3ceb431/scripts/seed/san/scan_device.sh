#!/bin/bash

for fc_host in `ls /sys/class/fc_host/`
do
	#echo 1 > /sys/class/fc_host/${fc_host}/issue_lip
	echo '- - -' > /sys/class/scsi_host/${fc_host}/scan
done
