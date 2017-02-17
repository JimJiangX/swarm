#!/bin/bash
set -o nounset

admin_unit=$1
lun_id=$2
hostname=$3
hostlun_id=$4
output=`mktemp /tmp/XXXXX`

aufibre1 -unit ${admin_unit} -refer | sed  -n '/Link\ Status/,$'p  | grep -v '^$' | awk 'NR>2{print $1, $2}' > $output
if [ $? -ne 0 ]; then
	rm -f $output
	echo "aufibre1 failed !"
	exit 1
fi

#del_lunmap() {
#	while read line
#	do
#		num=`echo $line | awk '{print $2}'`
#		ch=${num:0:1}
#		port=${num:1:1}
#		hostlun_id=`echo $line | awk '{print $1}'`
#		hostname=`echo $line | awk '{print $3}' | awk -F: '{print $2}'`
#		expect << EOF
#		set timeout 3
#		spawn auhgmap -unit ${admin_unit} -rm $ch $port -gname ${hostname} -hlu ${hostlun_id} -lu ${lun_id}
#		expect {
#	        	"y/n" {send "y\r"}
#		}
#		expect eof
#EOF
#	done < $output
#}

while read line
do
	auhgwwn -unit ${admin_unit}  -refer -permhg ${line} -gname ${hostname} > /dev/null 2>&1
	if [ $? -eq 0 ]; then
		expect << EOF
		set timeout 3
		spawn auhgmap -unit ${admin_unit} -add ${line} -gname ${hostname} -hlu ${hostlun_id} -lu ${lun_id}
		expect {
        		"y/n" {send "y\r"}
		}
		expect eof
EOF
		port=`echo "${line}" | sed 's/\ //g'`
		loop=0
		while(( $loop<=10 ))
		do
			if [ ${loop} -gt 10  ]; then
				# if timeout over exit 1
				echo "create lun mapping faild !"
				#del_lunmap 
				exit 1

			fi
			auhgmap -unit ${admin_unit} -refer | grep -w ${port} | grep -w ${hostname}
			if [ $? -eq 0 ]; then
				echo "create lun mapping succeeded!"
				break
			fi
			let "loop++"
			sleep 2
		done
	fi
done < $output

rm -f $output
