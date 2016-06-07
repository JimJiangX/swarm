#!/bin/bash

set -o nounset

domain=$1
ip_addr=$2
port=$3



cert_file=/tmp/registry.crt
cert_dir="/etc/docker/certs.d/${domain}:${port}"

# add DNS 
echo "${ip_addr}    ${domain}" >> /etc/hosts

# add cert file
mkdir -p ${cert_dir}
cp ${cert_file} ${cert_dir}/ca.crt



