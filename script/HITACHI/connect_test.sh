#!/bin/bash
set -o nounset

unit=$1

ausystemparam -unit ${unit} -refer > /dev/null 2>&1
if [ $? -ne 0 ]; then
	exit 2
fi

exit 0

