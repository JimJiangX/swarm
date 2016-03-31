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

echo $lun_id
