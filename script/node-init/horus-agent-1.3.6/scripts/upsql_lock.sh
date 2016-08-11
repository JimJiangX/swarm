#!/bin/bash

if [ $# -ne 3 ];then
	echo "must eqaul to 3"
  	exit 2
fi

INSTANCE=$1
USER=$2
PASSWD=$3

LockFILE=/tmp/${INSTANCE}_pressure_lock.data
dataFile=/tmp/${INSTANCE}_pressure_lock_data.data

running_status=`docker inspect -f "{{.State.Running}}" ${INSTANCE}`
if [ ${running_status} == "false" ]; then
	exit 4
fi

docker exec $INSTANCE mysql -S /DBAASDAT/upsql.sock mysql -u$USER -p$PASSWD  -e"select r.trx_mysql_thread_id waiting_thread,b.trx_mysql_thread_id blocking_thread from information_schema.innodb_lock_waits w inner join information_schema.innodb_trx b on b.trx_id= w.blocking_trx_id inner join information_schema.innodb_trx r on r.trx_id=w.requesting_trx_id;" >$LockFILE  2>/dev/null
if [ $? -ne 0 ];then
	echo "get lockID err"
	exit 2
fi

while read LINE 
 do 
 	lock1=`echo $LINE| awk '{print $1}'`
 	if [ "$lock1" = "waiting_thread" ];then 
      continue
    fi
 	lock2=`echo $LINE| awk '{print $2}'`
 	locks=$locks,$lock1,$lock2
 done <$LockFILE

if [ "$locks" = "" ];then
   rm  $LockFILE 
   echo "nolock"
   exit 0
fi

locks=${locks:1}

docker exec $INSTANCE mysql -S /DBAASDAT/upsql.sock mysql -u$USER -p$PASSWD  -e"select * from information_schema.processlist where ID in (${locks});"  >$dataFile  2>/dev/null
if [ $? -ne 0 ];then
	echo "get information_schema.processlist err"
	rm  $LockFILE 
	exit 2
fi

sed -i 's/\t/|/g' $dataFile

while read LINE 
 do 
  key=`echo $LINE| awk -F'|' '{print $1}'`
  value=${LINE}
  map[$key]=$value
 done <$dataFile

while read LINE 
 do 
  lock1=`echo $LINE| awk '{print $1}'`
  if [ "$lock1" = "waiting_thread" ];then 
    continue
  fi
  lock2=`echo $LINE| awk '{print $2}'`
  lockdatas=${lockdatas}##${map[$lock1]}^^${map[$lock2]}
done <$LockFILE
lockdatas=${lockdatas:2}

lockdatas=`echo $lockdatas | sed 's/\:/@/g'`
if [ "$lockdatas" = "" ];then
    lockdatas="nolock"
fi

echo  $lockdatas
 
rm $LockFILE  $dataFile

