#!/bin/bash
set -o nounset

IP_ADDR=$1

#CS_LIST='"10.211.55.61","10.211.55.62","10.211.55.62"'
CS_LIST=$2

CS_NUM=`echo ${CS_LIST} | awk -F\, '{print NF}'`
DC_NAME=dc1
HOSTNAME=`hostname`

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
  "datacenter": "${DC_NAME}",
  "data_dir": "/usr/local/consul",
  "node_name": "${HOSTNAME}",
  "disable_update_check": true,
  "log_level": "INFO",
  "addresses": {
    "http": "${IP_ADDR}",
    "rpc": "${IP_ADDR}"
  },
  "start_join": [
    ${CS_LIST}
 ]
}

EOF

# copy binary file
cp bin/consul /usr/bin/consul
chmod 755 /usr/bin/consul


# create systemd config file
cat << EOF > /etc/sysconfig/consul
## Path           : System/Management
## Description    : consul
## Type           : string
## Default        : ""
## ServiceRestart : consul

#
CONSUL_OPTS="agent -config-dir=/etc/consul.d -bind=${IP_ADDR}"

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
