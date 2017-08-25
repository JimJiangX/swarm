#!/bin/bash
set -o nounset

Instance_ID=$1
hostname=$2
# support multi wwn
shift 2

port_list=`sudo raidcom get port -I${Instance_ID} 2> /dev/null |grep PtoP| awk '{print $1}' | uniq`

if [ "${port_list}" == ''  ]; then
	echo "get port failed!"
	exit 2
fi


temp1=`mktemp -u /tmp/XXXXXXX`
for port in ${port_list}
do
	hosts=`sudo raidcom  get host_grp  -I${Instance_ID} -port ${port} | sed '1d' | awk '{print $2}'`
	for host in ${hosts}
	do
		sudo raidcom get hba_wwn -I${Instance_ID} -port ${port}-${host} | awk '{print $1,$3,$4}' >> ${temp1}
	done
done


temp_file=`mktemp -u /tmp/XXXXXXX`
for wwn in $@
do
	grep  ${wwn} ${temp1} >> ${temp_file}
done
rm -rf ${temp1}

if [ -z ${temp_file} ]; then
	for port in ${port_list}
	do
		ret=`sudo raidcom get port -I${Instance_ID} -port ${port} | grep -w ${wwn} | wc -l`
		if [ "${ret}" == "1" ]; then
			ret=`sudo raidcom get host_grp -I${Instance_ID} -port ${port} | grep -w ${hostname} | wc -l`
			if [ "${ret}" == "0" ]; then
				sudo raidcom add host_grp -I${Instance_ID} -port ${port} -host_grp_name ${hostname}
				if [ $? -ne 0 ]; then
					echo "add host_grp failed!"
					rm -rf ${temp_file}
					exit 2
				fi
			fi
			sudo raidcom add hba_wwn -I${Instance_ID} -port ${port} ${hostname} -hba_wwn ${wwn}
			if [ $? -ne 0 ]; then
				echo "add hba_wwn failed!"
				rm -rf ${temp_file}
				exit 1
			fi
		fi
	done
	
	rm -rf ${temp_file}
	exit 0
fi

while read LINE 
do
	port=`echo ${LINE} | awk '{print $1}'`
	old_host=`echo ${LINE} | awk '{print $2}'`
	wwn=`echo ${LINE} | awk '{print $3}'`
	sudo raidcom delete host_grp -I${Instance_ID} -port ${port} ${old_host}
	ret=$?
	if [ ${ret} -ne 0 ] && [ ${ret} -ne 205 ]; then
		echo "Delete old host $old_host on port $wwn failed!"
		rm -rf ${temp_file}
		exit 1
	fi
	sudo raidcom add host_grp -I${Instance_ID} -port ${port} -host_grp_name ${hostname}
	ret=$?
	if [ ${ret} -ne 0 ] && [ ${ret} -ne 221 ]; then
		echo "Add new host_grp  ${hostname} failed!"
		rm -rf ${temp_file}
		exit 2
	fi
	sudo raidcom add hba_wwn -I${Instance_ID} -port ${port} ${hostname} -hba_wwn ${wwn}
	if [ $? -ne 0 ]; then
		echo "add hba_wwn failed!"
		rm -rf ${temp_file}
		exit 3
	fi
done <${temp_file}

rm -rf ${temp_file}
