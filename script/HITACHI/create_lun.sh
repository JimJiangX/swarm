#!/bin/bash
set -o nounset

admin_unit=$1
rg=$2
lun_id=$3
lun_size=$4

auluref -unit ${admin_unit} -g -lu ${lun_id} 
if [ $? -eq 0 ]; then
	echo "lun (${lun_id}) is exist!"
	exit 2
fi

expect << EOF
set timeout 3
spawn auluadd -unit ${admin_unit} -rg ${rg} -lu ${lun_id} -size ${lun_size}m
expect {
	"y/n" {send "y\r"}
}
expect eof
EOF


loop=0

while(( $loop<=20 ))
do
	sleep 3
	auluref -unit ${admin_unit} -g -lu ${lun_id}
	if [ $? -eq 0 ]; then
		echo "create lun and format succeeded!"
		exit 0
	fi
	
	let "loop++"

done

# if timeout over exit 1
echo "check lun status timeout !"
exit 1
