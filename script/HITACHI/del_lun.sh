#!/bin/bash
set -o nounset


lun_id=$1

unit=AMS2100_83004824

expect << EOF
	set timeout 3
	spawn auludel -unit ${unit} -lu ${lun_id}
	expect {
       	"y/n" {send "y\r"; exp_continue}
	"y/n" {send "y\r"; exp_continue}
	"y/n" {send "y\r"}
}
expect eof
EOF
