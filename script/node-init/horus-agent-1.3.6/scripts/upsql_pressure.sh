#!/bin/bash
set -o nounset

if [ $# -ne 3 ];then
	echo "must eqaul to 3"
  	exit 2
fi

INSTANCE=$1
USER=$2
PASSWD=$3

STATSFILE=/tmp/${INSTANCE}_pressure_status.data

running_status=`docker inspect -f "{{.State.Running}}" ${INSTANCE}`
if [ "${running_status}" != "true" ]; then
	exit 4
fi

docker exec $INSTANCE mysql -S /DBAASDAT/upsql.sock mysql -u$USER -p$PASSWD  -e"show status where Variable_name in ('Com_insert','Com_update','Com_delete','Com_select','open_tables');" >$STATSFILE  2>/dev/null
if [ $? -ne 0 ];then
	echo "get data err"
	exit 2
fi
 
 while read LINE 
 do 
 	key=`echo $LINE| awk '{print $1}'`
 	value=`echo $LINE| awk '{print $2}'`
 	case $key in
 	Com_delete)

# upsql.com_delete
      delete=$value
    ;;
    Com_insert)
# upsql.com_insert
      insert=$value
    ;;
    Com_select)
# upsql.com_select
      com_select=$value
    ;;
    Com_update)
# upsql.com_update
      update=$value
    ;;
    Open_tables)
# upsql.innodb_open_table
     tables=$value
    esac
  
 done <$STATSFILE

 echo $delete:$insert:$com_select:$update:$tables

rm $STATSFILE


