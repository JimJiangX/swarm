# local_plugin_volume

example:
##create
docker  volume  create --driver lvm --opt size=1024 --opt  fstype=xfs --opt vgname=local --name DBASS_DAT

##mount && unmount
docker run --rm -ti --volume-driver lvm  -v DBASS_DAT:/mnd  ubuntu bin/bash 

##remove:
docker volume  rm   DBASS_DAT


##CREATE SAN VG:
curl -H "Content-Type: application/json" -X POST  --data '{"hostLunId":[1,2],"vgName":"TEST_SAN","type":"HUAWEI"}'  http://127.0.0.1:3333/san/vgcreate

##EXTEND SAN VG:
curl -H "Content-Type: application/json" -X POST  --data '{"hostLunId":[3],"vgName":"TEST_SAN","type":"HUAWEI"}'  http://127.0.0.1:3333/san/vgextend

##Activate
curl -H "Content-Type: application/json" -X POST  --data '{"lvName":["test_LOG","test_DAT"],"vgName":"TEST_SAN"}'  http://127.0.0.1:3333/san/activate

##Deactivate
curl -H "Content-Type: application/json" -X POST  --data '{"lvName":["test_LOG","test_DAT"],"vgName":"TEST_SAN","vendor":"HUAWEI","hostLunId":[1,2]}'  http://127.0.0.1:3333/san/deactivate

##LV UPDATE
curl -H "Content-Type: application/json" -X POST  --data '{"lvsName":"test_LOG","vgName":"TEST_SAN","driverType":"HUAWEI","fsType":"xfs","size":"3000"}'  http://127.0.0.1:3333/VolumeDriver.Update
