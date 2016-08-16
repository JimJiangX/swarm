#!/bin/bash
set -o nounset

if [ $# -ne 3 ];then
	echo "must eqaul to 3"
  	exit 2
fi

INSTANCE=$1
USER=$2
PASSWD=$3

running_status=`docker inspect -f "{{.State.Running}}" ${INSTANCE}`
if [ "${running_status}" != "true" ]; then
	exit 4
fi

STATUSFILE=/tmp/${INSTANCE}_buffer_status.data

docker exec $INSTANCE mysql -S /DBAASDAT/upsql.sock  mysql -u$USER -p$PASSWD  -e"show status where Variable_name in ('Innodb_buffer_pool_pages_free','Innodb_page_size','Innodb_buffer_pool_pages_total','innodb_buffer_pool_reads','innodb_buffer_pool_read_requests');" >$STATUSFILE  2>/dev/null

if [ $? -ne 0 ];then
	echo "get status err"
	exit 2
fi

 while read LINE 
 do 
 	key=`echo $LINE| awk '{print $1}'`
 	value=`echo $LINE| awk '{print $2}'`
 	case $key in
    Innodb_buffer_pool_read_requests)
      requests=$value
    ;;
    Innodb_buffer_pool_reads)
	   reads=$value
	;;
	Innodb_buffer_pool_pages_total)
      pool_pages_total=$value
    ;;
    Innodb_buffer_pool_pages_free)
	  pool_pages_free=$value
	;;
	Innodb_page_size)
      page_size=$value
    esac
 done <$STATUSFILE

#upsql.buffer_pool_hit
hit=`echo "scale=2;($requests - $reads)/$requests" |bc `

#upsql.buffer_pool.size
total=`echo "$pool_pages_total*$page_size/1024/1024" |bc `

#upsql.buffer_pool.free_size
free=`echo "$pool_pages_free*$page_size/1024/1024" |bc `

#upsql.buffer_pool_dirty_page
dirty_page=`docker exec $INSTANCE mysql -S /DBAASDAT/upsql.sock mysql -u$USER -p$PASSWD  -e"show engine innodb status \G;" 2>/dev/null | grep '^Total memory allocated' | awk '{print $4}' | tr -d ";"`
if [ "$dirty_page" = "" ];then
   dirty="err"
else
   dirty=`echo "${dirty_page}/1024/1024" |bc `
fi

#upsql.buffer_pool_usage
usage=`echo "scale=2;($pool_pages_total-$pool_pages_free)/$pool_pages_total*100" |bc `

echo "$hit:$total:$free:$usage:$dirty"
rm $STATUSFILE
