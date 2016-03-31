#!/bin/bash
set -o nounset

admin_unit=$1
hostname=$2
shift 2

output=`mktemp /tmp/XXXXX`

aufibre1 -unit ${admin_unit} -refer | sed  -n '/Link\ Status/,$'p  | grep -v '^$' | awk 'NR>2{print $1, $2}' > $output

while read line
do
	for wwn in "$@"
	do
		ret=`auhgwwn -unit ${admin_unit}  -refer -login ${line} | grep  -i ${wwn} | wc -l`
		if [ "${ret}" == "1" ]; then
			auhgdef -unit ${admin_unit} -add ${line} -gname "${hostname}"
			auhgwwn -unit ${admin_unit} -assign -permhg ${line} ${wwn} -gname "${hostname}"
		fi
	done

done < $output


rm -f $output
