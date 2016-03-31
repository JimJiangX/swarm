#!/bin/bash
set -o nounset

unit=AMS2100_83004824

hostname=$1
shift

output=`mktemp /tmp/XXXXX`

aufibre1 -unit ${unit} -refer | sed  -n '/Link\ Status/,$'p  | grep -v '^$' | awk 'NR>2{print $1, $2}' > $output

while read line
do
	for wwn in "$@"
	do
		ret=`auhgwwn -unit ${unit}  -refer -login ${line} | grep  -i ${wwn} | wc -l`
		if [ "${ret}" == "1" ]; then
			expect << EOF
				set timeout 3
				spawn auhgdef -unit ${unit} -rm ${line} -gname "${hostname}"
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
