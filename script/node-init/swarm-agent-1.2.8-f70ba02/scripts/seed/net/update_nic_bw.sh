#!/bin/sh
# This code should (try to) follow Google's Shell Style Guide
# (https://google-styleguide.googlecode.com/svn/trunk/shell.xml)
set -e
set -o nounset

if [ $# -ne 4 ]
then
  echo "Syntax:"
  echo "update_nic_bw.sh -h <host_interface> -b <bandwidth>"
  exit 1
fi

while [ $# -gt 0 ]
do
  opts=$1
  case $opts in
    -h) 
    # host_interface
    IFNAME=$2
    shift 2 
    ;;
    -b) 
    # bandwidth
    BANDWIDTH=$2
    shift 2
    ;;
    *)
    echo "Syntax:"
    echo "update_nic_bw.sh -h <host_interface> -b <bandwidth>"
    exit 1
    ;;
  esac
done

# Succeed if the given utility is installed. Fail otherwise.
# For explanations about `which` vs `type` vs `command`, see:
# http://stackoverflow.com/questions/592620/check-if-a-program-exists-from-a-bash-script/677212#677212
# (Thanks to @chenhanxiao for pointing this out!)
installed () {
  command -v "$1" >/dev/null 2>&1
}

# Google Styleguide says error messages should go to standard error.
warn () {
  echo "$@" >&2
}
die () {
  status="$1"
  shift
  warn "$@"
  exit "$status"
}

# First step: determine type of first argument (bridge, physical interface...),
if [ -d "/sys/class/net/$IFNAME" ]
then
  if [ -d "/sys/class/net/$IFNAME/bridge" ]; then
    die 1 "unsupported for Linux bridge."
  elif installed ovs-vsctl && ovs-vsctl list-br|grep -q "^${IFNAME}$"; then
    die 1 "unsupported for ovs."
  elif [ "$(cat "/sys/class/net/$IFNAME/type")" -eq 32 ]; then # Infiniband IPoIB interface type 32
    die 1 "unsupported for IPoIB."
  elif [ -d "/sys/class/net/$IFNAME/bonding" ]; then
    IFTYPE=bond
  else 
    IFTYPE=phys
  fi
else
  die 1 "I do not know how to setup interface $IFNAME."
fi

if [ "$IFTYPE" = bond ] && [ "$BANDWIDTH" ]
then
  # Get bond slave device
  SLAVES=`cat /sys/class/net/$IFNAME/bonding/slaves`
  for slave in $SLAVES
  do
    # Get bond slave PF device name
    PF_NAME=`ls /sys/class/net/$slave/device/physfn/net/ 2> /dev/null | tr -d "\n"`
    [ "$PF_NAME" ] || continue
    # If is sriov device, Get vf number
    VF_LIST=`ls -l /sys/class/net/$PF_NAME/device/ 2> /dev/null | grep virtfn | awk '{print $9}'`
    for vf in $VF_LIST
    do
      VF_NAME=`ls /sys/class/net/$PF_NAME/device/$vf/net/ 2>/dev/null | tr -d "\n"`
      if [ "$VF_NAME" = $slave ]; then
        VF_NUM=`echo $vf | cut -c 7-`
        ip link set $PF_NAME vf $VF_NUM rate $BANDWIDTH || {
          die 1 "Set $slave bandwidth faild"
        }
      fi
    done
  done
fi

if [ "$IFTYPE" = phys ] && [ "$BANDWIDTH" ]
then
  # Get host interface PF device name
  PF_NAME=`ls /sys/class/net/$IFNAME/device/physfn/net/ 2> /dev/null | tr -d "\n"`
  [ "$PF_NAME" ] || continue
  # If is sriov device, Get vf number
  VF_LIST=`ls -l /sys/class/net/$PF_NAME/device/ 2> /dev/null | grep virtfn | awk '{print $9}'`
  for vf in $VF_LIST
  do
    VF_NAME=`ls /sys/class/net/$PF_NAME/device/$vf/net/ 2>/dev/null | tr -d "\n"`
    if [ "$VF_NAME" = $slave ]; then
      VF_NUM=`echo $vf | cut -c 7-`
      ip link set $PF_NAME vf $VF_NUM rate $BANDWIDTH || {
        die 1 "Set $IFNAME bandwidth faild"
      }
    fi
  done
fi
