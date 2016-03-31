#!/bin/bash
set -o nounset


admin_unit=$1
lun_id=$2


expect << EOF
	set timeout 3
	spawn auludel -unit ${admin_unit} -lu ${lun_id}
	expect {
       	"y/n" {send "y\r"; exp_continue}
	"y/n" {send "y\r"; exp_continue}
	"y/n" {send "y\r"}
}
expect eof
EOF
