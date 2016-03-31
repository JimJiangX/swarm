#!/bin/bash
set -o nounset

admin_unit=$1
lun_id=$2
hostname=$3
hostlun_id=$4
output=`mktemp /tmp/XXXXX`

aufibre1 -unit ${admin_unit} -refer | sed  -n '/Link\ Status/,$'p  | grep -v '^$' | awk 'NR>2{print $1, $2}' > $output

while read line
do
	auhgwwn -unit ${admin_unit}  -refer -permhg ${line} -gname ${hostname}
	if [ "$?" == "0" ]; then
	expect << EOF
	set timeout 3
	spawn auhgmap -unit ${admin_unit} -add ${line} -gname ${hostname} -hlu ${hostlun_id} -lu ${lun_id}
	expect {
        	"y/n" {send "y\r"}
	}
	expect eof
EOF
	fi
done < $output

rm -f $output
