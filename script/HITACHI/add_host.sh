#!/bin/bash
set -o nounset

admin_unit=$1
hostname=$2
shift 2

output=`mktemp /tmp/XXXXX`

aufibre1 -unit ${admin_unit} -refer | sed  -n '/Link\ Status/,$'p  | grep -v '^$' | awk 'NR>2{print $1, $2}' > $output
if [ $? -ne 0 ]; then
	rm -f $output
	echo "aufibre1 failed !"
        exit 1
fi

while read line
do
	for wwn in "$@"
	do
		ret=`auhgwwn -unit ${admin_unit}  -refer -login ${line} | grep  -i ${wwn} | wc -l`
		if [ "${ret}" == "1" ]; then
			auhgdef -unit ${admin_unit} -add ${line} -gname "${hostname}"
			if [ $? -ne 0 ]; then
				rm -f $output
				echo "auhgdef failed!"
				exit 1
			fi
			auhgwwn -unit ${admin_unit} -assign -permhg ${line} ${wwn} -gname "${hostname}"
			if [ $? -ne 0 ]; then
				rm -f $output
				echo "auhgwwn failed!"
				exit 1
			fi
		else
			rm -f $output
			echo "not find WWN(${wwn}) in storage system!"
			exit 1
		fi
	done

done < $output

rm -f $output
