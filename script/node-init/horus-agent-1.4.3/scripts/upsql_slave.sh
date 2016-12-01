#!/bin/bash
set -o nounset

if [ $# -ne 3 ];then
	echo "must eqaul to 3"
  	exit 2
fi

INSTANCE=$1
USER=$2
PASSWD=$3
SLAVEFILE=/tmp/${INSTANCE}_file_slave.data

mhost=''
mport=''
mID=''
slaveIO=''
slaveSQL=''
seconds_behind_master=''
relay_Log_File=''
relay_Log_Pos=''
master_Log_File=''
read_Master_Log_Pos=''

outprint() {
	set +o nounset
	# upsql.replication.status
	replication="${mhost}#${mport}#${rwmode}#${mID}#${slaveIO}#${slaveSQL}#${seconds_behind_master}#${relay_Log_File}#${relay_Log_Pos}#${master_Log_File}#${read_Master_Log_Pos}#${host}#${port}"
	echo "$replication:$slaveIO:$slaveSQL"
	set -o nounset
}

rmtempfile() {
	if [ -e ${SLAVEFILE} ]; then
		rm -f ${SLAVEFILE}
	fi

}

running_status=`docker inspect -f "{{.State.Running}}" ${INSTANCE}`
if [ "${running_status}" != "true" ]; then
	outprint
	rmtempfile
	exit
fi

EXEC_BIN=`which mysql 2>/dev/null`
if [ "${EXEC_BIN}" == '' ]; then
	outprint
	rmtempfile
	exit
fi

${EXEC_BIN} -S /${INSTANCE}_DAT_LV/upsql.sock -u${USER} -p${PASSWD}  -e"show slave status \G;" >${SLAVEFILE}  2>/dev/null
if [ $? -ne 0 ]; then
	outprint
	rmtempfile
	exit
fi

host=`awk -F= '/bind_address=/{print $2}' /${INSTANCE}_DAT_LV/my.cnf 2>/dev/null`
port=`awk -F= '/port=/{print $2}' /${INSTANCE}_DAT_LV/my.cnf 2>/dev/null`
rwmode=`${EXEC_BIN} -S /${INSTANCE}_DAT_LV/upsql.sock -u${USER} -p${PASSWD}  -e"show variables like 'read_only';" 2>/dev/null |tail -n 1 | awk '{print $2}'`

while read LINE 
do  
	#去掉开头空格 awk -F : {print $1}
 	key=`echo $LINE| awk '{print $1}'`
 	value=`echo $LINE| awk '{print $2}'`
	case $key in
	Master_Host:)
		mhost=$value
	;;
	Master_Port:)
		mport=$value
	;;
	Master_Server_Id:)
		mID=$value
	;;
	Slave_IO_Running:)
		slaveIO=$value
	;;
	Slave_SQL_Running:)
		slaveSQL=$value
	;;
	Seconds_Behind_Master:)
		seconds_behind_master=$value
	;;
	Relay_Log_File:)
		relay_Log_File=$value
	;;
	Relay_Log_Pos:)
		relay_Log_Pos=$value
	;;
	Master_Log_File:)
		master_Log_File=$value
	;;
	Read_Master_Log_Pos:)
		read_Master_Log_Pos=$value
	esac
done <${SLAVEFILE}

outprint
rmtempfile
exit
