#!/bin/sh
# This code should (try to) follow Google's Shell Style Guide
# (https://google-styleguide.googlecode.com/svn/trunk/shell.xml)
set -e
set -o nounset

if [ $# -ne 10 ] && [ $# -ne 12 ]
then
  echo "Syntax:"
  echo "init_nic.sh -h <host_interface> -i <container_interface> -c <container> -ip <ip_addr>/<subnet>@<default_gateway> -v <vlan> [-b bandwidth]"
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
    -i) 
    # container_interface
    CONTAINER_IFNAME=$2
    shift 2 
    ;;
    -c) 
    # container
    CONTAINER=$2
    shift 2 
    ;;
    -ip) 
    # ip_addr/subnet@default_gateway
    IPADDR=$2
    shift 2 
    ;;
    -v) 
    # vlan
    VLAN=$2
    shift 2 
    ;;
    -b) 
    # bandwidth
    BANDWIDTH=$2
    shift 2
    ;;
    *)
    echo "Syntax:"
    echo "init_nic.sh -h <host_interface> -i <container_interface> -c <container> -ip <ip_addr>/<subnet>@<default_gateway> -v <vlan> [-b bandwidth]"
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

function getContainerPID () {
if installed docker
then
  RETRIES=3
  while [ "$RETRIES" -gt 0 ]; do
    DOCKERPID=$(docker inspect --format='{{ .State.Pid }}' "$CONTAINER")
    [ "$DOCKERPID" != 0 ] && break
    sleep 1
    RETRIES=$((RETRIES - 1))
  done

  [ "$DOCKERPID" = 0 ] && {
    die 1 "Docker inspect returned invalid PID 0"
  }

  [ "$DOCKERPID" = "<no value>" ] && {
    die 1 "Container $CONTAINER not found, and unknown to Docker."
  }
else
  die 1 "Container $CONTAINER not found, and Docker not installed."
fi

NSPID=$DOCKERPID
}

function getIPaddr () {
case "$IPADDR" in
  */*@*) : ;;
  *)
    warn "The IP address should include a netmask and a gateway"
    die 1 "example: 192.168.2.100/24@192.168.2.1 "
    ;;
esac
# Check if a gateway address was provided.
case "$IPADDR" in
  *@*)
    GATEWAY="${IPADDR#*@}" GATEWAY="${GATEWAY%%@*}"
    IPADDR="${IPADDR%%@*}"
    ;;
  *)
    warn "The IP address should include a netmask and a gateway"
    die 1 "example: 192.168.2.100/24@192.168.2.1 "
    ;;
esac
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

# check interface up
ifdown ${IFNAME}
sleep 2
# check interface up
if ! ifup ${IFNAME}
then
  die 1 "ifup ${IFNAME} failed"
fi

# Second step: find the container and get container pid
getContainerPID

# Check if a subnet mask was provided.
getIPaddr


# Check if an incompatible VLAN device already exists
if [ "$IFTYPE" = bond ] && [ "$VLAN" ]
then
  # If get bandwidth ,try to set bandwidth
  if [ "$BANDWIDTH" ]; then
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
            warn "Set $slave bandwidth faild"
          }
        fi
      done
    done
  fi

  # If get VLAN ,try to set VLAN
  if [ $VLAN -ne 0 ]; then
    # If it's a bond interface, create a vlan subinterface
    MTU=$(ip link show "$IFNAME" | awk '{print $5}')
    [ ! -d "/sys/class/net/${IFNAME}.${VLAN}" ] && {
      ip link add link "$IFNAME" name "$IFNAME.$VLAN" mtu "$MTU" type vlan id "$VLAN" || {
        die 1 "create $IFNAME.VLAN faild"
      }
    }
    ip link set "$IFNAME" up || {
      die 1 "$IFNAME.VLAN up faild"
    }
    GUEST_IFNAME=$IFNAME.$VLAN
  else
    GUEST_IFNAME=$IFNAME
  fi
elif [ "$IFTYPE" = phys ] && [ "$VLAN" ] 
then
  # If get bandwidth ,try to set bandwidth
  if [ "$BANDWIDTH" ]; then
    # Get host interface PF device name
    PF_NAME=`ls /sys/class/net/$IFNAME/device/physfn/net/ 2> /dev/null | tr -d "\n"`
    [ "$PF_NAME" ] || continue
    # If is sriov device, Get vf number
    VF_LIST=`ls -l /sys/class/net/$PF_NAME/device/ 2> /dev/null | grep virtfn | awk '{print $9}'`
    for vf in $VF_LIST
    do
      VF_NAME=`ls /sys/class/net/$PF_NAME/device/$vf/net/ 2>/dev/null | tr -d "\n"`
      if [ "$VF_NAME" = $IFNAME ]; then
        VF_NUM=`echo $vf | cut -c 7-`
        ip link set $PF_NAME vf $VF_NUM rate $BANDWIDTH || {
          warn "Set $IFNAME bandwidth faild"
        }
      fi
    done
  fi

  if [ $VLAN -ne 0 ]; then
    # If it's a physical interface, create a vlan subinterface
    MTU=$(ip link show "$IFNAME" | awk '{print $5}')
    [ ! -d "/sys/class/net/${IFNAME}.${VLAN}" ] && {
      ip link add link "$IFNAME" name "$IFNAME.$VLAN" mtu "$MTU" type vlan id "$VLAN" || {
        die 1 "create $IFNAME.VLAN faild"
      }
    }
    ip link set "$IFNAME" up || {
      die 1 "$IFNAME.VLAN up faild"
    }
    GUEST_IFNAME=$IFNAME.$VLAN
  else
    GUEST_IFNAME=$IFNAME
  fi
fi

# Create netns
[ ! -d /var/run/netns ] && mkdir -p /var/run/netns
rm -f "/var/run/netns/$NSPID"
ln -s "/proc/$NSPID/ns/net" "/var/run/netns/$NSPID"

# Link host interface to conatiner net namespace
ip link set "$GUEST_IFNAME" netns "$NSPID" || {
  die 1 "Link $GUEST_IFNAME to conatiner faild"
}

# Rename container interface
ip netns exec "$NSPID" ip link set "$GUEST_IFNAME" name "$CONTAINER_IFNAME" || {
  die 1 "Rename container interface faild"
}

# Add ip address to container interface
ip netns exec "$NSPID" ip addr add "$IPADDR" dev "$CONTAINER_IFNAME" || {
  die 1 "Add ip address to container interface faild"
}
[ "$GATEWAY" ] && {
  ip netns exec "$NSPID" ip route delete default >/dev/null 2>&1 && true
}

# set container interface up
ip netns exec "$NSPID" ip link set "$CONTAINER_IFNAME" up
[ "$GATEWAY" ] && {
  ip netns exec "$NSPID" ip route get "$GATEWAY" >/dev/null 2>&1 || \
  ip netns exec "$NSPID" ip route add "$GATEWAY/32" dev "$CONTAINER_IFNAME" || {
    die 1 "Add route faild"
  }
  ip netns exec "$NSPID" ip route replace default via "$GATEWAY"  || {
    die 1 "Replace route to default route faild"
  }
}

# Give our ARP neighbors a nudge about the new interface
if installed arping
then
  IPADDR=$(echo "$IPADDR" | cut -d/ -f1)
  ip netns exec "$NSPID" arping -c 1 -A -I "$CONTAINER_IFNAME" "$IPADDR" > /dev/null 2>&1 || true
else
  echo "Warning: arping not found; interface may not be immediately reachable"
fi

# Remove NSPID to avoid `ip netns` catch it.
rm -f "/var/run/netns/$NSPID"

# vim: set tabstop=2 shiftwidth=2 softtabstop=2 expandtab :
