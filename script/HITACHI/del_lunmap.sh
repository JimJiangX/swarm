#!/bin/bash
set -o nounset

lun_id=$1

unit=AMS2100_83004824
output=`mktemp /tmp/XXXXX`


auluref -unit ${unit} -pathinfo -lu ${lun_id} | grep -v '^$' | sed '1,3d' > $output

while read line
do
	num=`echo $line | awk '{print $2}'`
	ch=${num:0:1}
	port=${num:1:1}
	hostlun_id=`echo $line | awk '{print $1}'`
	hostname=`echo $line | awk '{print $3}' | awk -F: '{print $2}'`
	expect << EOF
	set timeout 3
	spawn auhgmap -unit ${unit} -rm $ch $port -gname ${hostname} -hlu ${hostlun_id} -lu ${lun_id}
	expect {
        	"y/n" {send "y\r"}
	}
	expect eof
EOF
done < $output

rm -f $output
