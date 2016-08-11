#!/bin/bash

if [ $# -ne 3 ];then
	echo "must eqaul to 3"
  	exit 2
fi

INSTANCE=$1
USER=$2
PASSWD=$3

STATUSFILE=/tmp/${INSTANCE}_connection_status.data
VARFILE=/tmp/${INSTANCE}_connection_variables.data

running_status=`docker inspect -f "{{.State.Running}}" ${INSTANCE}`
if [ ${running_status} == "false" ]; then
	exit 4
fi


docker exec $INSTANCE mysql -S /DBAASDAT/upsql.sock mysql -u$USER -p$PASSWD  -e"show status where Variable_name in ('threads_running','threads_cached','Threads_connected','Aborted_connects');" >$STATUSFILE  2>/dev/null
if [ $? -ne 0 ];then
	echo "get status err"
	exit 2
fi

docker exec $INSTANCE mysql -S /DBAASDAT/upsql.sock mysql -u$USER -p$PASSWD  -e"show variables where Variable_name in ('max_connections', 'thread_cache_size');" >$VARFILE  2>/dev/null
if [ $? -ne 0 ];then
	echo "get variabes err"
	rm  $STATUSFILE
	exit 2
fi

while read LINE 
 do 
 	key=`echo $LINE| awk '{print $1}'`
 	value=`echo $LINE| awk '{print $2}'`
 	case $key in
    Aborted_connects)
      aborted=$value
     ;;
    Threads_cached)
      cached=$value
     ;;
     Threads_connected)
      #upsql.connection_attempts
      attempts=$value
     ;;
     Threads_running)
      #unsql.exec_thread
       exec_thread=$value
    esac
 done <$STATUSFILE


 while read LINE 
 do 
 	key=`echo $LINE| awk '{print $1}'`
 	value=`echo $LINE| awk '{print $2}'`
 	case $key in
    max_connections)
      #upsql.connection_total
      total=$value
     ;;
    thread_cache_size)
	  size=$value
    esac
 done <$VARFILE

#upsql.connection_usage
connection_usage=`echo "scale=2;$attempts/$total*100" |bc `

#upsql.thread_cache_usage
cache_usage=`echo "scale=3;$cached/$size*100" |bc `

echo $total:$attempts:$connection_usage:$exec_thread:$cache_usage

rm  $STATUSFILE  $VARFILE
