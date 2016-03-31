#!/bin/bash
set -o nounset

rg=$1
lun_id=$2
lun_size=$3

unit=AMS2100_83004824

expect << EOF
set timeout 3
spawn auluadd -unit ${unit} -rg ${rg} -lu ${lun_id} -size ${lun_size}m
expect {
	"y/n" {send "y\r"}
}
expect eof
EOF

echo $lun_id
