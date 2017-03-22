#!/bin/bash
set -o nounset


role=
master_ip=
master_port=
repl_user=
repl_pwd=
slave_ip=
slave_port=


case $FLAG in
	master)
		echo "role is master, nothing to do"
		exit 0
		;;
	slave)
		set_replication
		;;
	*)
		echo "${self_role} role unspport, role "
		echo "Invalid attribute in role: ${role}" >&2
		exit 1
		;;
esac

set_replication() {
	

}
