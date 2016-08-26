#!/bin/bash
set -o nounset


if [ $# -ne 3 ];then
	echo "must eqaul to 3"
  	exit 3
fi

INSTANCE=$1
USER=$2
PASSWD=$3
#QUOTA=$4
# 1024 * 1024 * 1024 * 5 =5368709120 = 5G
#QUOTA=5368709120
VARFILE=/tmp/${INSTANCE}_file_variables.data
datadir=/${INSTANCE}_DAT_LV
logdir=/${INSTANCE}_LOG_LV

running_status=`docker inspect -f "{{.State.Running}}" ${INSTANCE}`
if [ "${running_status}" != "true" ]; then
	exit 4
fi

 #upsql.error_file_size
errfile=${datadir}/upsql.err
errsize=`du -b $errfile 2>/dev/null | awk '{print $1}'`
if [ "$errsize" = "" ];then
	errsize=0
fi


#upsql.slow_query_file_size
queryfile=${logdir}/slow-query.log
qrysize=`du -b $queryfile 2>/dev/null | awk '{print $1}' `
if [ "$qrysize" = "" ];then
	qrysize=0
fi


#upsql.table_size
function gettables() {	
	database_dir=$1
	database=${database_dir##*/}
	local tables=''

	for file in `ls ${database_dir}/*ibd`;do
		tablesize=`du -b $file | awk '{print $1}'`
		table=${file##*/}
		tablename=`echo $table | awk -F . '{print $1}'`       
		tables=$tables#${database},${tablename},${tablesize}
	done
	echo $tables
}

if [ ! -d ${datadir} ];then
	tablestr="err"
else
	tablestr=''
	for  file in `ls ${datadir}`;do
		if  [ -d ${datadir}/${file} ] && [ "${datadir}/${file}" != "${datadir}/performance_schema" ] && [  "${datadir}/${file}" != "${datadir}/mysql" ];then
			tmp=`gettables ${datadir}/${file} 2>/dev/null`
			tablestr=$tablestr$tmp
		fi
	done
	if [ "${tablestr}" != "" ];then 
		tablestr=${tablestr:1}
	else
		tablestr="null"
	fi
fi

echo "$errsize:$qrysize:$tablestr"
