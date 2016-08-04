#!/bin/bash
set -o nounset

swarm_key=$1
adm_ip=$2
cs_datacenter=$3
cs_list=$4
registry_domain=$5
registry_ip=$6
registry_port=$7
registry_username=$8
registry_passwd=$9
regstry_ca_file=${10}
docker_port=${11}
hdd_dev=${12}
ssd_dev=${13}
horus_agent_port=${14}
consul_port=${15}
node_id=${16}
horus_server_ip=${17}
horus_server_port=${18}
docker_plugin_port=${19}
nfs_ip=${20}
nfs_dir=${21}
nfs_mount_dir=${22}
nfs_mount_opts=${23}
cur_dir=`dirname $0`

hdd_vgname=${HOSTNAME}_HDD_VG
ssd_vgname=${HOSTNAME}_SSD_VG

adm_nic=bond0
int_nic=bond1
ext_nic=bond2

nfs_mount() {
	umount -f ${nfs_mount_dir} > /dev/null 2>&1
	sed -i "/${nfs_ip}:${nfs_dir}/d" /etc/fstab > /dev/null 2>&1
	rm -rf ${nfs_mount_dir}
	mkdir ${nfs_mount_dir}
	mount -t nfs -o ${nfs_mount_opts} ${nfs_ip}:${nfs_dir} ${nfs_mount_dir} && echo "${nfs_ip}:${nfs_dir}	${nfs_mount_dir}	nfs	defaults	0 0" >> /etc/fstab
	if [ $? -ne 0 ]; then
		echo "nfs mount failed"
		exit 2
	fi	
}


reg_to_horus_server() {
	local component_type=$1

	stat_code=`curl -o /dev/null -s -w %{http_code} -X POST -H "Content-Type: application/json" -d '{ "endpoint": "'${node_id}'","name": "'${node_id}':'${component_type}'","type": "'${component_type}'","checktype": "health" }' http://${horus_server_ip}:${horus_server_port}/v1/component/register`
	if [ ${stat_code} != '200' ]; then
		echo "${component_type} register to horus server failed"
		exit 2
	fi
}

create_check_script() {

	local dir=/opt/DBaaS/script
	mkdir -p ${dir}

	cat << EOF > ${dir}/check_swarmagent.sh
#!/bin/bash
ps -ef | grep -v "grep" | grep "/usr/bin/swarm join"
if [ \$? -ne 0 ]; then
	exit 2
else
	exit 0
fi
EOF

	chmod +x ${dir}/check_swarmagent.sh
	
	cat << EOF > ${dir}/check_db.sh
#!/bin/bash

container_name=\$1

docker inspect \${container_name} > /dev/null 2>&1
if [ \$? -ne 0 ]; then
	 exit 2
fi

docker exec \${container_name} /root/check_db
if [ \$? -ne 0 ]; then
	 exit 2
fi
EOF
	chmod +x ${dir}/check_db.sh


	cat << EOF > ${dir}/check_proxy.sh
#!/bin/bash

container_name=\$1

docker inspect \$container_name > /dev/null 2>&1
if [ \$? -ne 0 ]; then
	 exit 2
fi

docker exec \$container_name /root/check_proxy --default-file /DBAASCNF/upsql-proxy.conf
if [ \$? -ne 0 ]; then
	 exit 2
fi
EOF
	chmod +x ${dir}/check_proxy.sh

	cat << EOF > ${dir}/check_switchmanager.sh
#!/bin/bash

container_name=\$1
output=\`mktemp /tmp/XXXXX\`

docker inspect \$container_name > \$output 2>&1
if [ \$? -ne 0 ]; then
	 exit 2
fi

ip_addr=\`cat \$output | grep IPADDR | awk -F= '{print \$2}' | sed 's/",//g'\`
port=\`cat \$output | grep PORT | awk -F= '{print \$2}' | sed 's/",//g'\`

rm -f \$output

stat_code=\`curl -o /dev/null -s -w %{http_code} -X POST http://\${ip_addr}:\${port}/ping\`
if [ \${stat_code} == '200' ]; then
	 exit 0
 else
	 exit 2
fi
EOF

	chmod +x ${dir}/check_switchmanager.sh
}


reg_to_consul_for_swarm() {
	local component_type=SwarmAgent

	stat_code=`curl -o /dev/null -s -w %{http_code} -X POST -H "Content-Type: application/json" -d '{"ID": "'${node_id}':'${component_type}'","Name": "'${node_id}':'${component_type}'", "Tags": [], "Address": "'${adm_ip}'", "Check": {"Script": "/opt/DBaaS/script/check_swarmagent.sh ", "Interval": "10s" }}' http://${adm_ip}:${consul_port}/v1/agent/service/register`

	if [ ${stat_code} != '200' ]; then
		echo "${component_type} register to consul failed"
		exit 2
	fi
}


# init VG


reg_to_consul() {
	local component_type=$1
	local component_port=$2

	stat_code=`curl -o /dev/null -s -w %{http_code} -X POST -H "Content-Type: application/json" -d '{"ID": "'${node_id}':'${component_type}'","Name": "'${node_id}':'${component_type}'", "Tags": [], "Address": "'${adm_ip}'", "Port": '${component_port}', "Check": { "tcp": "'${adm_ip}':'${component_port}'", "Interval": "10s", "timeout": "3s" }}' http://${adm_ip}:${consul_port}/v1/agent/service/register`
	if [ ${stat_code} != "200" ]; then
		echo "${component_type} register to consul failed"
		exit 2
	fi
}


# init VG
init_hdd_vg() {
	local hdd_dev_list=''
	if [ ${hdd_dev} == "null" ]; then
		hdd_dev=''
		hdd_vgname=''
		return
	fi

	hdd_dev_array=( ${hdd_dev/\,/\ } )
		
	for dev_name in ${hdd_dev_array[@]}
	do
		pvcreate -ffy /dev/${dev_name}
		if [ $? -ne 0 ]; then
			echo "${dev_name} pvcreate failed"
			exit 2
		fi
		hdd_dev_list=${hdd_dev_list}" /dev/${dev_name}"
	done

	vgcreate -fy ${hdd_vgname} ${hdd_dev_list}
	if [ $? -ne 0 ]; then
		echo "${hdd_dev} vgcreate failed"
		exit 2
	fi	
}

# init VG
init_ssd_vg() {
	local hdd_dev_list=''
	if [ ${ssd_dev} == "null" ]; then
		ssd_dev=''
		ssd_vgname=''
		return
	fi

	ssd_dev_array=( ${ssd_dev/\,/\ } )
		
	for dev_name in ${ssd_dev_array[@]}
	do
		pvcreate -ffy /dev/${dev_name}
		if [ $? -ne 0 ]; then
			echo "${dev_name} pvcreate failed"
			exit 2
		fi
		ssd_dev_list=${ssd_dev_list}" /dev/${dev_name}"
	done

	vgcreate -fy ${ssd_vgname} ${ssd_dev_list}
	if [ $? -ne 0 ]; then
		echo "${ssd_dev} vgcreate failed"
		exit 2
	fi	
}

# install consul agent
install_consul() {
	local version=0.6.4
	
	# stop consul
	pkill -9 consul >/dev/null 2>&1

	# remove data dir
        rm -rf /usr/local/consul/*

	# check consul dir
	if [ ! -d /etc/consul.d ]; then
        	mkdir -p /etc/consul.d
	fi

	# create consul config file
	cat << EOF > /etc/consul.d/config.json
{
  "server": false,
  "datacenter": "${cs_datacenter}",
  "data_dir": "/usr/local/consul",
  "node_name": "${HOSTNAME}",
  "disable_update_check": true,
  "log_level": "INFO",
  "addresses": {
    "http": "${adm_ip}",
    "rpc": "${adm_ip}"
  },
  "start_join": ${cs_list}
}

EOF

	# copy binary file
	cp ${cur_dir}/consul-agent-${version}-release/bin/consul /usr/bin/consul; chmod 755 /usr/bin/consul

	# create systemd config file
	cat << EOF > /etc/sysconfig/consul
## Path           : System/Management
## Description    : consul
## Type           : string
## Default        : ""
## ServiceRestart : consul

#
CONSUL_OPTS="agent -config-dir=/etc/consul.d -bind=${adm_ip}"

EOF


	cat << EOF > /usr/lib/systemd/system/consul.service
[Unit]
Description=Consul 
Documentation=https://www.consul.io
Wants=basic.target
After=network.target

[Service]
EnvironmentFile=/etc/sysconfig/consul
Environment=GOMAXPROCS=2
Restart=on-failure
RestartSec=42s
ExecStart=/usr/bin/consul \$CONSUL_OPTS
ExecReload=/bin/kill -HUP \$MAINPID
KillMode=process

[Install]
WantedBy=multi-user.target

EOF

	chmod 644 /etc/sysconfig/consul
	chmod 644 /usr/lib/systemd/system/consul.service

	# reload
	systemctl daemon-reload

	# Enable & start the service
	systemctl enable consul
	systemctl start consul
        systemctl status consul
        if [ $? -ne 0 ]; then
                echo "start consul failed!"
                exit 2
        fi
}

# install docker
install_docker() {
	local version=1.11.2
	# scan wwn 
	wwn=""
	for fc_host in `ls /sys/class/fc_host/`
	do
		s=`cat /sys/class/fc_host/${fc_host}/port_name | cut -c 3-`
		wwn=$wwn + $s
	done
	
	wwn=${wwn:2}

	# check nic
	ifconfig $adm_nic >/dev/null 2>&1 
	if [ $? -ne 0 ]; then
		echo "not find adm_nic ${adm_nic}"
		exit 2
	fi

	ifconfig $int_nic >/dev/null 2>&1 
	if [ $? -ne 0 ]; then
		echo "not find int_nic ${int_nic}"
		exit 2
	fi

	ifconfig $ext_nic >/dev/null 2>&1 
	if [ $? -ne 0 ]; then
		echo "not find ext_nic ${ext_nic}"
		ext_nic=""
	fi

	# stop docker
	pkill -9 docker >/dev/null 2>&1
	pkill -9 docker-contarinerd >/dev/null 2>&1

	# copy the binary & set the permissions
	cp ${cur_dir}/docker-${version}-release/bin/docker* /usr/bin/; chmod +x /usr/bin/docker*
	

	# create the systemd files
	cat << EOF > /etc/sysconfig/docker
## Path           : System/Management
## Description    : Extra cli switches for docker daemon
## Type           : string
## Default        : ""
## ServiceRestart : docker

#
DOCKER_OPTS=-H tcp://0.0.0.0:${docker_port} -H unix:///var/run/docker.sock --label NODE_ID=${node_id} --label HBA_WWN=${wwn} --label HDD_VG=${hdd_vgname} --label SSD_VG=${ssd_vgname} --label ADM_NIC=${adm_nic} --label INT_NIC=${int_nic} --label EXT_NIC=${ext_nic}

EOF

	cat << EOF > /usr/lib/systemd/system/docker.service
[Unit]
Description=Docker Application Container Engine
Documentation=https://docs.docker.com
After=network.target docker.socket
Requires=docker.socket

[Service]
# the default is not to use systemd for cgroups because the delegate issues still
# exists and systemd currently does not support the cgroup feature set required
# for containers run by docker
Type=notify
EnvironmentFile=/etc/sysconfig/docker
ExecStart=/usr/bin/docker daemon -H fd:// \$DOCKER_OPTS
ExecReload=/bin/kill -s HUP \$MAINPID
LimitNOFILE=1048576
LimitNPROC=1048576
LimitCORE=infinity
TimeoutStartSec=0
## man systemd.resource-control check support
## systemd-210 unsupport
# set delegate yes so that systemd does not reset the cgroups of docker containers
#Delegate=yes

[Install]
WantedBy=multi-user.target

EOF

	cat << EOF > /usr/lib/systemd/system/docker.socket
[Unit]
Description=Docker Socket for the API
PartOf=docker.service

[Socket]
ListenStream=/var/run/docker.sock
SocketMode=0660
SocketUser=root
SocketGroup=docker

[Install]
WantedBy=sockets.target
EOF

	chmod 644 /etc/sysconfig/docker
	chmod 644 /usr/lib/systemd/system/docker.service
	chmod 644 /usr/lib/systemd/system/docker.socket

	# reload
	systemctl daemon-reload

	# Enable & start the service
	systemctl enable docker
	systemctl start docker
	systemctl status docker
	if [ $? -ne 0 ]; then
		echo "start docker failed!"
		exit 2
	fi

}



init_docker() {
	local cert_file=$regstry_ca_file
	local cert_dir="/etc/docker/certs.d/${registry_domain}:${registry_port}"

	# add DNS 
	echo "${registry_ip}    ${registry_domain}" >> /etc/hosts

	# add cert file
	mkdir -p ${cert_dir}
	cp ${cert_file} ${cert_dir}/ca.crt
	docker login -u ${registry_username} -p ${registry_passwd}  ${registry_domain}:${registry_port}
	if [ $? -ne 0 ]; then
                echo "init docker failed!"
                exit 2

        fi

}

# register docker

# install docker plugin
install_docker_plugin() {
	local version=1.6.5
	local script_dir=/usr/local/local_volume_plugin/scripts
	mkdir -p ${script_dir}

	pkill -9 local-volume-plugin > /dev/null 2>&1

	# copy binary file
	cp ${cur_dir}/dbaas_volume_plugin-${version}/bin/local_volume_plugin /usr/bin/local_volume_plugin; chmod 755 /usr/bin/local_volume_plugin

	# copy script
	cp ${cur_dir}/dbaas_volume_plugin-${version}/scripts/*.sh ${script_dir}
	chmod +x ${script_dir}/*.sh

	cat << EOF > /usr/lib/systemd/system/local-volume-plugin.service
[Unit]
Description=DBaaS local disk volume plugin for docker
Wants=docker.service
After=docker.service


[Service]
Restart=on-failure
RestartSec=30s
ExecStart=/usr/bin/local_volume_plugin

[Install]
WantedBy=multi-user.target

EOF

	chmod 644 /usr/lib/systemd/system/local-volume-plugin.service

	# reload
	systemctl daemon-reload
	# Enable & start the service
	systemctl enable local-volume-plugin
	systemctl start local-volume-plugin	
	systemctl status local-volume-plugin
        if [ $? -ne 0 ]; then
                echo "start docker-plugin failed!"
                exit 2
        fi
}

# register docker plugin

# install swarm agent
install_swarm_agent() {
	local version=1.2.0
	# stop swarm-agent
	pkill -9 swarm >/dev/null 2>&1

	# copy binary file
	cp ${cur_dir}/swarm-agent-${version}-release/bin/swarm /usr/bin/swarm; chmod 755 /usr/bin/swarm

	#nohup swarm join --advertise=${adm_ip}:${docker_port} consul://${adm_ip}:${consul_port}/DBaaS  >> /var/log/swarm.log &
	# create systemd config file
	cat << EOF > /etc/sysconfig/swarm-agent
## Path           : System/Management
## Description    : docker swarm
## Type           : string
## Default        : ""
## ServiceRestart : swarm

#
SWARM_AGENT_OPTS="join --advertise=${adm_ip}:${docker_port} consul://${adm_ip}:${consul_port}/DBaaS"

EOF

	cat << EOF > /usr/lib/systemd/system/swarm-agent.service
[Unit]
Description=Docker Swarm agent
Documentation=https://docs.docker.com
After=network.target
Requires=consul.service

[Service]
EnvironmentFile=/etc/sysconfig/swarm-agent
ExecStart=/usr/bin/swarm  \$SWARM_AGENT_OPTS

[Install]
WantedBy=multi-user.target

EOF

	chmod 644 /etc/sysconfig/swarm-agent
	chmod 644 /usr/lib/systemd/system/swarm-agent.service

	# reload
	systemctl daemon-reload

	# Enable & start the service
	systemctl enable swarm-agent
	systemctl start swarm-agent
	if [ $? -ne 0 ]; then
		echo "start swarm-agent failed!"
		exit 2
	fi
}

# register swarm-agent

# install horus agent
install_horus_agent() {
	local version=1.3.6
	# stop swarm-agent
	pkill -9 horus-agent >/dev/null 2>&1

	zypper --non-interactive install sysstat

	# copy binary file
	mkdir -p /usr/local/horus-agent
	cp ${cur_dir}/horus-agent-${version}/bin/horus-agent /usr/bin/horus-agent; chmod 755 /usr/bin/horus-agent
	cp -r ${cur_dir}/horus-agent-${version}/scripts /usr/local/horus-agent/scripts; chmod -R +x /usr/local/horus-agent/scripts/*.sh

	local nets_dev="${adm_nic}#${int_nic}"
	if [ ! -z "${ext_nic}" ]; then
		local nets_dev="${nets_dev}#${ext_nic}"
	fi

	# create systemd config file
	cat << EOF > /etc/sysconfig/horus-agent
## Path           : System/Management
## Description    : horus agent
## Type           : string
## Default        : ""
## ServiceRestart : horus-agent

#
HORUS_AGENT_OPTS="-loglevel debug -consulip ${adm_ip}:${consul_port} -datacenter ${cs_datacenter} -hsrv ${horus_server_ip}:${horus_server_port} -ip ${adm_ip} -name ${node_id} -nets ${nets_dev} -port ${horus_agent_port} "

EOF

	cat << EOF > /usr/lib/systemd/system/horus-agent.service
[Unit]
Description=Horus agent
Documentation=
After=network.target
Requires=consul.service

[Service]
EnvironmentFile=/etc/sysconfig/horus-agent
ExecStart=/usr/bin/horus-agent  \$HORUS_AGENT_OPTS

[Install]
WantedBy=multi-user.target

EOF

	chmod 644 /etc/sysconfig/horus-agent
	chmod 644 /usr/lib/systemd/system/horus-agent.service

	# reload
	systemctl daemon-reload

	# Enable & start the service
	systemctl enable horus-agent
	systemctl start horus-agent
	if [ $? -ne 0 ]; then
		echo "start horus-agent failed!"
		exit 2
	fi

}



create_check_script
nfs_mount
init_hdd_vg
init_ssd_vg
install_consul
install_docker_plugin
reg_to_consul DockerPlugin ${docker_plugin_port}
reg_to_horus_server DockerPlugin 
install_docker
init_docker
reg_to_consul Docker ${docker_port}
reg_to_horus_server Docker
install_swarm_agent
reg_to_consul_for_swarm
reg_to_horus_server SwarmAgent
install_horus_agent
reg_to_consul HorusAgent ${horus_agent_port}
reg_to_horus_server HorusAgent



exit 0
