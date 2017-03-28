#!/bin/bash
set -o nounset
LANG=POSXIA

nfs_ip=$1
nfs_dir=$2
nfs_mount_dir=$3
nfs_mount_opt=$4

TYPE=nfs4

#check mount
df $nfs_mount_dir > /dev/null 2>&1 || {
	mount -t $TYPE -o $nfs_mount_opt $nfs_ip:$nfs_dir $nfs_mount_dir || {
		echo "mount nfs dir fail"
		exit 2
	}
}

total_space=`df --type=$TYPE --output=avail $nfs_mount_dir | sed '1d'`
used_space=`df --type=$TYPE --output=used $nfs_mount_dir | sed '1d'`
free_space=`expr $total_space - $used_space`

echo "total_space:${total_space}"
echo "free_space:${free_space}"
