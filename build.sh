#!/bin/bash
ip=146.32.33.17
root_user=cup_dba
root_pwd=AN8uRFhY
mon_user=db_aasmn
mon_pwd=qRSB21Lk
repl_user=db_aasrp
repl_pwd=2NxcV1wp
chk_user=db_aasck
chk_pwd=N8uRF2Nx
go_respon=github.com/docker/swarm

go build -v  -ldflags "-w -X ${go_respon}/version.GITCOMMIT=$(git rev-parse --short HEAD) -X ${go_respon}/version.BUILDTIME=$(date -u +%FT%T%z) -X ${go_respon}/vars.root=${root_user} -X ${go_respon}/vars.root_pwd=${root_pwd} -X ${go_respon}/vars.mon=${mon_user} -X ${go_respon}/vars.mon_pwd=${mon_pwd} -X ${go_respon}/vars.repl=${repl_user} -X ${go_respon}/vars.repl_pwd=${repl_pwd} -X ${go_respon}/vars.check=${chk_user} -X ${go_respon}/vars.check_pwd=${chk_pwd}"
if [ $? -ne 0 ]; then
	echo "build failed!!!!!!!!!"
	exit 2
fi
