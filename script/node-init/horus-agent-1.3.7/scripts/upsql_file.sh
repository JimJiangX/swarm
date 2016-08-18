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
errfile=${database}/upsql.err
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
function gettables()
{	
    local tables
	database=$1

	cd $database
	if [ $? -ne 0 ];then
		exit 2
	fi

	for file in `ls *ibd`;do
		# if [ "$file" = "innodb_index_stats.ibd" ]  ||  [ "$file" = "slave_relay_log_info.ibd"  ]|| \
		#   [ "$file" = "innodb_table_stats.ibd" ] || [ "$file" = "slave_worker_info.ibd" ] || \
		#   [  "$file" = "slave_master_info.ibd" ];then
    #      	continue
    # fi
        tablesize=`du -b $file | awk '{print $1}'`
        tablename=`echo $file | awk -F . '{print $1}'`       
        tables=$tables#${database},${tablename},${tablesize}
	done

	echo $tables
}

cd $datadir
if [ $? -ne 0 ];then
  tablestr="err"
else
  for  file in `ls`;do
    if  [ -d $file ] && [ "$file" != "performance_schema" ] && [  "$file" != "mysql" ];then
  		tmp=`gettables $file 2>/dev/null`
  		tablestr=$tablestr$tmp
    fi
  done
   if [ "$tablestr" != "" ];then 
      tablestr=${tablestr:1}
   else
      tablestr="null"
   fi

fi

echo $errsize:$qrysize:$tablestr
