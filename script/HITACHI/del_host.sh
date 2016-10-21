#!/bin/bash
set -o nounset

admin_unit=$1
hostname=$2
output=`mktemp /tmp/XXXXX`

aufibre1 -unit ${admin_unit} -refer | sed  -n '/Link\ Status/,$'p  | grep -v '^$' | awk 'NR>2{print $1, $2}' > $output
if [ $? -ne 0 ]; then
	rm -f $output
	echo "aufibre1 failed !"
 	exit 1
fi

while read line
do
	ret=`auhgwwn -unit ${admin_unit}  -refer -permhg ${line} -gname ${hostname} | grep -i ${hostname} | wc -l`
	if [ "${ret}" == "1" ]; then
		expect << EOF
			set timeout 3
			spawn auhgdef -unit ${admin_unit} -rm ${line} -gname "${hostname}"
			expect {
       			"y/n" {send "y\r"; exp_continue}
			"y/n" {send "y\r"; exp_continue}
			"y/n" {send "y\r"}
			}
		expect eof
EOF
	fi
done < $output

rm -f $output
