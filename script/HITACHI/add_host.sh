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

for wwn in $@
do
	count=0
	for port in ${port_list}
	do
		hosts=`sudo raidcom  get host_grp  -I${Instance_ID} -port $port | sed '1d' | awk '{print $2}'`
		for host in $hosts
		do
			ret=`sudo raidcom get hba_wwn -I${Instance_ID} -port $port-$host | grep -i $wwn | wc -l`
			if [ "${ret}" == "1" ]; then
				old_host=`sudo raidcom get hba_wwn -I${Instance_ID} -port $port-$host | grep -i $wwn | awk '{print $3}'`
				sudo raidcom delete host_grp -I${Instance_ID} -port ${port} ${old_host}
				if [ $? -ne 0  ]; then
					echo "delete host_grp failed!"
					exit 3
				fi
				sudo raidcom  get host_grp  -I${Instance_ID} -port $port | grep ${hostname}
				if [ $? -ne 0 ]; then
					sudo raidcom add host_grp -I${Instance_ID} -port ${port} -host_grp_name ${hostname}
					if [ $? -ne 0 ]; then
						echo "add host_grp failed!"
						exit 2
					fi
				fi
				sudo raidcom add hba_wwn -I${Instance_ID} -port ${port} ${hostname} -hba_wwn ${wwn}
				if [ $? -ne 0 ]; then
					echo "add hba_wwn failed!"
					exit 1
				fi
				count=`expr $count + 1`
			fi
		done
	done

	if [ $count == 0 ]; then
		for port in ${port_list}
		do
			ret=`sudo raidcom get port -I${Instance_ID} -port ${port} | grep -w ${wwn} | wc -l`
			if [ "${ret}" == "1" ]; then
				ret=`sudo raidcom get host_grp -I${Instance_ID} -port ${port} | grep -w ${hostname} | wc -l`
				if [ "${ret}" == "0" ]; then
					sudo raidcom add host_grp -I${Instance_ID} -port ${port} -host_grp_name ${hostname}
					if [ $? -ne 0 ]; then
						echo "add host_grp failed!"
						exit 2
					fi
				fi
				sudo raidcom add hba_wwn -I${Instance_ID} -port ${port} ${hostname} -hba_wwn ${wwn}
				if [ $? -ne 0 ]; then
					echo "add hba_wwn failed!"
					exit 1
				fi
			fi
		done
	fi
done
