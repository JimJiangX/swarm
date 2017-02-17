#!/bin/bash
set -o nounset

Instance_ID=$1

sudo raidcom get resource -I${Instance_ID}
if [ $? -ne 0 ]; then
	echo "get resource failed!"
	exit 2
fi


