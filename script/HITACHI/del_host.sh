#!/bin/bash
set -o nounset

admin_unit=$1
hostname=$2
shift

output=`mktemp /tmp/XXXXX`

aufibre1 -unit ${admin_unit} -refer | sed  -n '/Link\ Status/,$'p  | grep -v '^$' | awk 'NR>2{print $1, $2}' > $output

while read line
do
	for wwn in "$@"
	do
		ret=`auhgwwn -unit ${admin_unit}  -refer -login ${line} | grep  -i ${wwn} | wc -l`
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
	done

done < $output

rm -f $output
