#!/bin/bash

for fc_host in `ls /sys/class/fc_host/`
do
	echo '- - -' > /sys/class/scsi_host/${fc_host}/scan
done
