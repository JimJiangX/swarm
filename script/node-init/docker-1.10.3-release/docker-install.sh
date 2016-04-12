#!/bin/bash

# for SLES 12 sp1

DOCKER_PORT=2375

# stop docker
systemctl stop docker >/dev/null 2>&1

# copy the binary & set the permissions
cp bin/docker /usr/bin/docker; chmod +x /usr/bin/docker

# create the systemd files
cat << EOF > /etc/sysconfig/docker
## Path           : System/Management
## Description    : Extra cli switches for docker daemon
## Type           : string
## Default        : ""
## ServiceRestart : docker

#
DOCKER_OPTS="-H tcp://0.0.0.0:${DOCKER_PORT} -H unix:///var/run/docker.sock "

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

#
chmod 644 /etc/sysconfig/docker
chmod 644 /usr/lib/systemd/system/docker.service
chmod 644 /usr/lib/systemd/system/docker.socket

# reload
systemctl daemon-reload

# Enable & start the service
systemctl enable docker
systemctl start docker
