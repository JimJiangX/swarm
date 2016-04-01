#!/bin/bash
set -o nounset

admin_unit=$1
rg=$2
lun_id=$3
lun_size=$4

expect << EOF
set timeout 3
spawn auluadd -unit ${admin_unit} -rg ${rg} -lu ${lun_id} -size ${lun_size}m
expect {
	"y/n" {send "y\r"}
}
expect eof
EOF

if [ $? -ne 0 ]; then
	echo "create lun fail !"
	exit 1
fi

echo $lun_id
