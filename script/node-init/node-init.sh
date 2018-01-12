#!/bin/bash
set -o nounset

swarm_key=${1}
adm_ip=${2}
cs_datacenter=${3}
cs_list=${4}
registry_domain=${5}
registry_ip=${6}
registry_port=${7}
registry_username=${8}
registry_passwd=${9}
registry_ca_file=${10}
docker_port=${11}
hdd_dev=${12}
ssd_dev=${13}
consul_port=${14}
node_id=${15}
horus_server_ip=${16}
horus_server_port=${17}
swarm_agent_port=${18}
nfs_ip=${19}
nfs_dir=${20}
nfs_mount_dir=${21}
nfs_mount_opts=${22}
san_id=${23}
if [ "$san_id" == "null" ]; then
	san_id=''
fi

cur_dir=`dirname $0`

hdd_vgname=${HOSTNAME}_HDD_VG
ssd_vgname=${HOSTNAME}_SSD_VG

bond_dev=bond0

bond_mode=`cat /sys/class/net/${bond_dev}/bonding/mode`

bond_slaves=`cat /sys/class/net/${bond_dev}/bonding/slaves`
if [ ! -n "${bond_slaves}" ]; then
	echo "${bond_dev} slaves is null"
	exit 2
fi

pf_dev_bw_num=0
if [ "${bond_mode}" == "balance-xor 2" ]; then
	for d in ${bond_slaves}
	do
		#d_bw=`ethtool enp130s0f0| grep Speed | awk -F: '{print $2}' | awk -FMb '{print $1}'`	
		#pf_dev_bw_num=`expr ${pf_dev_bw_num} + ${d_bw}`
                d_bw=`ethtool ${d} | grep Speed | awk -F: '{print $2}' | awk -FMb '{print $1}' | sed s/[[:space:]]//g`    
		if [ ${d_bw} -gt ${pf_dev_bw_num} ]; then
                	pf_dev_bw_num=${d_bw}
		fi
	done
	pf_dev_bw=${pf_dev_bw_num}M
elif [ "${bond_mode}" == "active-backup 1" ]; then
	for d in ${bond_slaves}
        do
                d_bw=`ethtool ${d} | grep Speed | awk -F: '{print $2}' | awk -FMb '{print $1}' | sed s/[[:space:]]//g`    
		if [ ${d_bw} -gt ${pf_dev_bw_num} ]; then
                	pf_dev_bw_num=${d_bw}
		fi
        done
        pf_dev_bw=${pf_dev_bw_num}M
else
	echo "${bond_dev} bond mode unsupport "
	exit 2		
fi


PT=${cur_dir}/rpm/percona-toolkit-2.2.20-1.noarch.rpm

docker_version=17.12.0
consul_version=1.0.2
swarm_agent_version=1.2.8-f70ba02
logicalVolume_volume_plugin_version=3.0.0

platform="$(uname -s)"
yum --nogpgcheck -y install lsb
release=""
if [ "${platform}" = "Linux" ]; then
	kernel="$(uname -r)"
	release="$(lsb_release -is)"
else
	echo "only support linux platform"
	exit 3
fi

# check container_nic
container_nic=`ifconfig | grep -e 'cbond[0-9]\{1,3\}' | awk '{print $1}' | sed 's/://g' |  tr "\n" "," |sed 's/.$//'` 
if [ ${container_nic} = '' ]; then
	echo "not found container nic"
	exit 2
fi

set_sysctl() {
	cat << EOF > /etc/sysctl.conf
net.ipv4.ip_forward = 0
net.ipv6.conf.all.forwarding = 0
net.ipv4.tcp_tw_recycle = 1
net.ipv4.tcp_tw_reuse = 1
vm.dirty_ratio = 1
vm.dirty_background_ratio = 1
vm.dirty_writeback_centisecs = 10
vm.dirty_expire_centisecs = 3
vm.drop_caches = 1
vm.swappiness = 0
vm.vfs_cache_pressure = 200
vm.pagecache_limit_mb = 10240
net.ipv4.conf.all.accept_redirects = 0
net.ipv4.conf.all.accept_source_route = 0
net.ipv4.conf.all.rp_filter = 1
net.ipv4.conf.all.secure_redirects = 0
net.ipv4.conf.all.send_redirects = 0
net.ipv4.conf.default.accept_redirects = 0
net.ipv4.conf.default.accept_source_route = 0
net.ipv4.conf.default.rp_filter = 1
net.ipv4.conf.default.secure_redirects = 0
net.ipv4.conf.default.send_redirects = 0
net.ipv4.icmp_echo_ignore_broadcasts = 1
net.ipv4.icmp_ignore_bogus_error_responses = 1
net.ipv4.tcp_max_syn_backlog = 4096
net.ipv4.tcp_syncookies = 1
fs.aio-max-nr = 262144
EOF
	sysctl -p
}

rpm_install() {
	if [ "${release}" == "SUSE LINUX" ]; then
		zypper --no-gpg-checks --non-interactive install nfs-utils curl sysstat mariadb-client ${PT}
		if [ $? -ne 0 ]; then
			echo "rpm install faild"
			exit 2
		fi
	elif [ "${release}" == "RedHatEnterpriseServer" ] || [ "${release}" == "CentOS" ]; then
		yum --nogpgcheck -y install nfs-utils curl sysstat mariadb ${PT}
		if [ $? -ne 0 ]; then
			echo "rpm install faild"
			exit 2
		fi
	fi	
	cp ${cur_dir}/rpm/jq /usr/bin/
}

nfs_mount() {
	local fstab=/etc/fstab
        
        mount | grep "${nfs_ip}:${nfs_dir}"
        if [ $? -eq 0 ]; then
		umount ${nfs_mount_dir} > /dev/null 2>&1
		if [ $? -ne 0 ]; then
                	echo "nfs unmount failed"
                	exit 2
        	fi
	fi
	rm -rf ${nfs_mount_dir}
	mkdir ${nfs_mount_dir}
	mount -t nfs -o ${nfs_mount_opts} ${nfs_ip}:${nfs_dir} ${nfs_mount_dir} 
	if [ $? -ne 0 ]; then
		echo "nfs mount failed"
		exit 2
	else
		grep "${nfs_ip}:${nfs_dir}" ${fstab} 
		if [ $? -ne 0 ]; then
			echo "${nfs_ip}:${nfs_dir}	${nfs_mount_dir}	nfs	defaults	0 0" >> ${fstab}
		fi
	fi	
}

reg_to_horus_server() {
	local component_type=$1

	stat_code=`curl -o /dev/null -s -w %{http_code} -X POST -H "Content-Type: application/json" -d '{ "endpoint": "'${node_id}'","name": "'${node_id}':'${component_type}'","type": "'${component_type}'","checktype": "health" }' http://${horus_server_ip}:${horus_server_port}/v1/component/register`
	if [ "${stat_code}" != "200" ]; then
		echo "${component_type} register to horus server failed"
		exit 2
	fi
}

reg_to_consul() {
	local component_type=$1
	local component_port=$2

	stat_code=`curl -o /dev/null -s -w %{http_code} -X POST -H "Content-Type: application/json" -d '{"ID": "'${node_id}':'${component_type}'","Name": "'${node_id}':'${component_type}'", "Tags": [], "Address": "'${adm_ip}'", "Port": '${component_port}', "Check": { "tcp": "'${adm_ip}':'${component_port}'", "Interval": "10s", "timeout": "3s" }}' http://${adm_ip}:${consul_port}/v1/agent/service/register`
	if [ "${stat_code}" != "200" ]; then
		echo "${component_type} register to consul failed"
		exit 2
	fi
}

# init VG
init_hdd_vg() {
	local hdd_dev_list=''
	if [ "${hdd_dev}" == "null" ]; then
		hdd_dev=''
		hdd_vgname=''
		hdd_vg_size=''
		return
	fi

	hdd_dev_array=${hdd_dev//\,/\ }
		
	for dev_name in ${hdd_dev_array}
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
	hdd_vg_size=`vgdisplay --units B ${hdd_vgname} | awk '/VG\ Size/{print $3}'`
}

# init VG
init_ssd_vg() {
	local ssd_dev_list=''
	if [ "${ssd_dev}" == "null" ]; then
		ssd_dev=''
		ssd_vgname=''
		ssd_vg_size=''
		return
	fi

	ssd_dev_array=${ssd_dev//\,/\ }
		
	for dev_name in ${ssd_dev_array}
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
	ssd_vg_size=`vgdisplay --units B ${ssd_vgname} | awk '/VG\ Size/{print $3}'`
}

# install consul agent
install_consul() {
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
  "protocol": 3,
  "addresses": {
    "http": "${adm_ip}"
  },
  "start_join": ${cs_list}
}

EOF

	# copy binary file
	cp ${cur_dir}/consul-agent-${consul_version}/bin/consul /usr/bin/consul
	chmod +x /usr/bin/consul

	# create systemd config file
	cat << EOF > /etc/sysconfig/consul
## Path           : System/Management
## Description    : consul
## Type           : string
## Default        : ""
## ServiceRestart : consul

#
CONSUL_OPTS="agent -protocol=3 -log-level=debug -config-dir=/etc/consul.d -bind=${adm_ip}"

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
	systemctl enable consul.service
	systemctl restart consul.service
        systemctl status consul.service
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
		wwn=${wwn}","${s}
	done
	
	wwn=${wwn:1}

	if [ "${release}" == "RedHatEnterpriseServer" ] || [ "${release}" == "CentOS" ]; then
		if [ "${san_id}" != '' ]; then
			systemctl enable multipathd.service
			systemctl start multipathd.service
			systemctl status multipathd.service
			if [ $? -ne 0 ]; then
				echo "start multipathd failed!"
				exit 2
			fi
		fi
		
		yum --nogpgcheck -y install ${cur_dir}/rpm/docker-ce-${docker_version}.el7/*.rpm
		if [ $? -ne 0 ]; then
			echo "docker rpm install faild"
			exit 2
		fi
		cat << EOF > /etc/sysconfig/docker
## Path           : System/Management
## Description    : Extra cli switches for docker daemon
## Type           : string
## Default        : ""
## ServiceRestart : docker

#
DOCKER_OPTS=--host=tcp://0.0.0.0:${docker_port} --host=unix:///var/run/docker.sock --label="NODE_ID=${node_id}" --label="HBA_WWN=${wwn}" --label="HDD_VG=${hdd_vgname}" --label="HDD_VG_SIZE=${hdd_vg_size}" --label="SSD_VG=${ssd_vgname}" --label="SSD_VG_SIZE=${ssd_vg_size}" --label="CONTAINER_NIC=${container_nic}" --label PF_DEV_BW=${pf_dev_bw}

EOF

		cat << EOF > /usr/lib/systemd/system/docker.service

[Unit]
Description=Docker Application Container Engine
Documentation=https://docs.docker.com
After=network-online.target firewalld.service
Wants=network-online.target

[Service]
Type=notify
# the default is not to use systemd for cgroups because the delegate issues still
# exists and systemd currently does not support the cgroup feature set required
# for containers run by docker
EnvironmentFile=/etc/sysconfig/docker
ExecStart=/usr/bin/dockerd \$DOCKER_OPTS
ExecReload=/bin/kill -s HUP \$MAINPID
# Having non-zero Limit*s causes performance problems due to accounting overhead
# in the kernel. We recommend using cgroups to do container-local accounting.
LimitNOFILE=infinity
LimitNPROC=infinity
LimitCORE=infinity
# Uncomment TasksMax if your systemd version supports it.
# Only systemd 226 and above support this version.
#TasksMax=infinity
TimeoutStartSec=0
# set delegate yes so that systemd does not reset the cgroups of docker containers
Delegate=yes
# kill only the docker process, not all processes in the cgroup
KillMode=process
# restart the docker process if it exits prematurely
Restart=on-failure
StartLimitBurst=3
StartLimitInterval=60s

[Install]
WantedBy=multi-user.target
EOF
	else 
		echo "only support Centos and RHEL"
		exit 2
	fi

	# reload
	systemctl daemon-reload

	# Enable & start the service
	systemctl enable docker.service
	systemctl restart docker.service
	systemctl status docker.service
	if [ $? -ne 0 ]; then
		echo "start docker failed!"
		exit 2
	fi

	init_docker

}

init_docker() {
	local cert_file=${registry_ca_file}
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

# install docker plugin
install_docker_plugin() {
	local base_dir=/usr/local/logicalVolume-volume-plugin
	local script_dir=${base_dir}/scripts

	mkdir -p ${base_dir}/bin
	mkdir -p ${script_dir}

	pkill -9 local-volume-plugin > /dev/null 2>&1

	# copy binary file
	cp ${cur_dir}/logicalVolume-volume-plugin-${logicalVolume_volume_plugin_version}/bin/logicalVolume_volume_plugin ${base_dir}/bin/logicalVolume_volume_plugin
	chmod +x /usr/bin/logicalVolume_volume_plugin

	# copy script
	cp ${cur_dir}/logicalVolume-volume-plugin-${logicalVolume_volume_plugin_version}/scripts/*.sh ${script_dir}
	chmod +x ${script_dir}/*.sh

	cat << EOF > /usr/lib/systemd/system/logicalVolume-volume-plugin.service
[Unit]
Description=DBaaS logicalVolume volume plugin for docker
Wants=docker.service
After=docker.service


[Service]
Restart=on-failure
RestartSec=30s
ExecStart=${base_dir}/bin/logicalVolume_volume_plugin

[Install]
WantedBy=multi-user.target

EOF

	chmod 644 /usr/lib/systemd/system/logicalVolume-volume-plugin.service

	# reload
	systemctl daemon-reload
	# Enable & start the service
	systemctl enable logicalVolume-volume-plugin.service
	systemctl restart logicalVolume-volume-plugin.service
	systemctl status logicalVolume-volume-plugin.service
        if [ $? -ne 0 ]; then
                echo "start docker-plugin failed!"
                exit 2
        fi
}

# install swarm agent
install_swarm_agent() {
	local base_dir=/usr/local/swarm-agent
	local script_dir=${base_dir}/scripts
	local bin_dir=${base_dir}/bin

	# stop swarm-agent
	pkill -9 swarm >/dev/null 2>&1

	# copy script dir
	mkdir -p ${script_dir}
	cp -r ${cur_dir}/swarm-agent-${swarm_agent_version}/scripts/* ${script_dir}/
	chmod +x ${script_dir}/seed/net/* ${script_dir}/seed/san/*

	# copy binary file
	mkdir -p ${bin_dir}
	cp ${cur_dir}/swarm-agent-${swarm_agent_version}/bin/swarm ${bin_dir}
	chmod 755 ${bin_dir}/swarm

	# create systemd config file
	cat << EOF > /etc/sysconfig/swarm-agent
## Path           : System/Management
## Description    : docker swarm
## Type           : string
## Default        : ""
## ServiceRestart : swarm

#
SWARM_AGENT_OPTS="seedjoin --seedAddr ${adm_ip}:${swarm_agent_port} --script=${script_dir}/seed/ --advertise=${adm_ip}:${docker_port} consul://${adm_ip}:${consul_port}/${swarm_key}"

EOF

	cat << EOF > /usr/lib/systemd/system/swarm-agent.service
[Unit]
Description=Docker Swarm agent
Documentation=https://docs.docker.com
After=network.target consul.service

[Service]
Environment=CONSUL_HTTP_DATACENTER=${cs_datacenter}
EnvironmentFile=/etc/sysconfig/swarm-agent
ExecStart=${bin_dir}/swarm  \$SWARM_AGENT_OPTS

[Install]
WantedBy=multi-user.target

EOF

	chmod 644 /etc/sysconfig/swarm-agent
	chmod 644 /usr/lib/systemd/system/swarm-agent.service

	# reload
	systemctl daemon-reload

	# Enable & start the service
	systemctl enable swarm-agent.service
	systemctl restart swarm-agent.service
	systemctl status swarm-agent.service
	if [ $? -ne 0 ]; then
		echo "start swarm-agent failed!"
		exit 2
	fi
}

set_sysctl
rpm_install
nfs_mount
init_hdd_vg
init_ssd_vg
install_consul

install_docker_plugin
#reg_to_consul DockerPlugin ${docker_plugin_port}
#reg_to_horus_server DockerPlugin 

install_docker
reg_to_consul Docker ${docker_port}
#reg_to_horus_server Docker

install_swarm_agent
reg_to_consul SwarmAgent ${swarm_agent_port}
#reg_to_horus_server SwarmAgent

exit 0
