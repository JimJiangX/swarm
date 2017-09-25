#!/bin/bash
set -o nounset
LANG=POSXIA

nfs_ip=$1
nfs_dir=$2
nfs_mount_dir=$3
nfs_mount_opt=$4

TYPE=nfs4

#check mount
df --type=$TYPE $nfs_mount_dir | grep -w $nfs_ip:$nfs_dir > /dev/null 2>&1
if [ $? -ne 0 ]; then 
	echo "not found nfs dir"
	exit 2
fi

free_space_kb=`df --type=$TYPE --output=avail $nfs_mount_dir | sed '1d'`
used_space_kb=`df --type=$TYPE --output=used $nfs_mount_dir | sed '1d'`
total_space_kb=`expr $free_space_kb + $used_space_kb`
free_space_byte=`expr ${free_space_kb} \* 1024`
total_space_byte=`expr ${total_space_kb} \* 1024`

echo "total_space:${total_space_byte}"
echo "free_space:${free_space_byte}"
