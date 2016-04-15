#!/bin/bash

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
cur_dir=`dirname $0`


# check NIC

# install consul agent
install_consul_agent() {
	# stop consul
	systemctl stop consul >/dev/null 2>&1

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
	cp ${cur_dir}/consul-agent-0.6.4-release/bin/consul /usr/bin/consul; chmod 755 /usr/bin/consul

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
	# scan wwn 
	wwn=""
	for fc_host in `ls /sys/class/fc_host/`
	do
		s=`cat /sys/class/fc_host/${fc_host}/port_name | cut -c 3-`
		wwn=$wwn + $s
	done
	
	wwn=${wwn:2}

	# stop docker
	systemctl stop docker >/dev/null 2>&1

	# copy the binary & set the permissions
	cp ${cur_dir}/docker-1.10.3-release/bin/docker /usr/bin/docker; chmod +x /usr/bin/docker
	

	# create the systemd files
	cat << EOF > /etc/sysconfig/docker
## Path           : System/Management
## Description    : Extra cli switches for docker daemon
## Type           : string
## Default        : ""
## ServiceRestart : docker

#
DOCKER_OPTS="-H tcp://0.0.0.0:${docker_port} -H unix:///var/run/docker.sock --label HBA_WWN="${wwn}"  "

EOF

	cat << EOF > /usr/lib/systemd/system/docker.service
[Unit]
Description=Docker Application Container Engine
Documentation=https://docs.docker.com
After=network.target docker.socket
Requires=docker.socket

[Service]
Type=notify
EnvironmentFile=/etc/sysconfig/docker
ExecStart=/usr/bin/docker daemon -H fd:// \$DOCKER_OPTS
LimitNOFILE=1048576
LimitNPROC=1048576
LimitCORE=infinity
TimeoutStartSec=0

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
	docker login -u ${registry_username} -p ${registry_passwd}  -e "unionpay.com" ${registry_domain}:${registry_port}
	if [ $? -ne 0 ]; then
                echo "init docker failed!"
                exit 2

        fi

}

# install docker plugin
install_docker_plugin() {
	local script_dir=/opt

	systemctl stop local-volume-plugin > /dev/null 2>&1

	# copy binary file
	cp ${cur_dir}/dbaas_volume_plugin-1.5.3/bin/local_volume_plugin /usr/bin/local_volume_plugin; chmod 755 /usr/bin/local_volume_plugin

	# copy script
	cp ${cur_dir}/script/*.sh ${script_dir}
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

# install swarm agent

# install hours agent


#check_nic
install_consul
install_docker_plugin
install_docker
init_docker
#install_swarm_agent
#install_hours_agent
