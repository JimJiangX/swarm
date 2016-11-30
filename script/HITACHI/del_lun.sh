#!/bin/bash
set -o nounset


admin_unit=$1
lun_id=$2

auluref -unit ${admin_unit} -g -lu ${lun_id}
if [ $? -ne 0 ]; then
	echo "The specified logical unit is not defined."
	exit 0
fi

expect << EOF
	set timeout 3
	spawn auludel -unit ${admin_unit} -lu ${lun_id}
	expect {
       	"y/n" {send "y\r"; exp_continue}
	"y/n" {send "y\r"; exp_continue}
	"y/n" {send "y\r"}
}
EOF

loop=0

while(( $loop<=4 ))

do
	auluref -unit ${admin_unit} -g -lu ${lun_id}
	if [ $? -ne 0 ]; then
		echo "delete lun succeeded!"
		exit 0
	fi
	
	let "loop++"

	sleep 1
done

# if timeout over exit 1
echo "delete lun failed !"
exit 1
