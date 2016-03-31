#!/bin/bash
set -o nounset

lun_id=$1
hostname=$2
hostlun_id=$3

unit=AMS2100_83004824
output=`mktemp /tmp/XXXXX`

aufibre1 -unit ${unit} -refer | sed  -n '/Link\ Status/,$'p  | grep -v '^$' | awk 'NR>2{print $1, $2}' > $output

while read line
do
	auhgwwn -unit ${unit}  -refer -permhg ${line} -gname ${hostname}
	if [ "$?" == "0" ]; then
	expect << EOF
	set timeout 3
	spawn auhgmap -unit ${unit} -add ${line} -gname ${hostname} -hlu ${hostlun_id} -lu ${lun_id}
	expect {
        	"y/n" {send "y\r"}
	}
	expect eof
EOF
	fi
done < $output

rm -f $output
