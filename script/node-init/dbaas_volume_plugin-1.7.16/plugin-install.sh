#!/bin/bash

script_dir=/opt

systemctl stop local-volume-plugin > /dev/null 2>&1

# copy binary file
cp bin/local_volume_plugin /usr/bin/local_volume_plugin
chmod 755 /usr/bin/local_volume_plugin

# copy script
cp script/*.sh ${script_dir}
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
