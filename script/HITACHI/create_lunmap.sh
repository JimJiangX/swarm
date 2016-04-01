#!/bin/bash
set -o nounset

admin_unit=$1
lun_id=$2
hostname=$3
hostlun_id=$4
output=`mktemp /tmp/XXXXX`
c1=0
c2=0

aufibre1 -unit ${admin_unit} -refer | sed  -n '/Link\ Status/,$'p  | grep -v '^$' | awk 'NR>2{print $1, $2}' > $output
if [ $? -ne 0 ]; then
	rm -f $output
	echo "aufibre1 failed !"
	exit 1
fi

while read line
do
	auhgwwn -unit ${admin_unit}  -refer -permhg ${line} -gname ${hostname} > /dev/null 2>&1
	if [ $? -eq 0 ]; then
		c1=$[ $c1 + 1 ]
		expect << EOF
		set timeout 3
		spawn auhgmap -unit ${admin_unit} -add ${line} -gname ${hostname} -hlu ${hostlun_id} -lu ${lun_id}
		expect {
        		"y/n" {send "y\r"}
		}
		expect eof
EOF
		if [ $? -eq  0 ]; then
			c2=$[ $c2 + 1 ]
		fi
	fi
done < $output

rm -f $output

if [ $c1 -ne $c2 ]; then
	exit 2
fi


