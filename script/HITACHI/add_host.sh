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
			auhgdef -unit ${unit} -add ${line} -gname "${hostname}"
			auhgwwn -unit ${unit} -assign -permhg ${line} ${wwn} -gname "${hostname}"
		fi
	done

done < $output


rm -f $output
