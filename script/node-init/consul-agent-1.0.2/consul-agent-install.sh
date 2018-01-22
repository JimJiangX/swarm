#!/bin/bash
set -o nounset

ip_addr=$1
#cs_list='"10.211.55.61","10.211.55.62","10.211.55.62"'
cs_list=$2
DC_NAME=dc1

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
    "http": "${ip_addr}"
  },
  "start_join": [
    ${cs_list}
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
CONSUL_OPTS="agent -protocol=3 -log-level=debug -config-dir=/etc/consul.d -bind=${ip_addr}"

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
