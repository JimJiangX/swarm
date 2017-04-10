package nic

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/swarm/cluster"
	"github.com/pkg/errors"
)

const (
	// "PF_DEV_BW":"10G"
	_PFDevLabel = "PF_DEV_BW"

	// "CONTAINER_NIC":"bond0,bond1,bond2"
	_ContainerNIC = "CONTAINER_NIC"
)

type Device struct {
	Bond      string
	Bandwidth int // M/s
	IP        string
	Mask      string
	Gateway   string
	VLAN      int
	err       error
}

// "VF_DEV_0":"bond0,10M,192.168.1.1,255.255.255.0,192.168.3.0,vlan_xxxx"
func (d Device) String() string {
	if d.err != nil {
		return d.err.Error()
	}
	return fmt.Sprintf("%s,%dM,%s,%s,%s,%d", d.Bond, d.Bandwidth,
		d.IP, d.Mask, d.Gateway, d.VLAN)
}

func parseBandwidth(width string) (int, error) {
	if len(width) == 0 {
		return 0, nil
	}

	n := 1
	switch width[len(width)-1] {
	case 'G', 'g':
		n = 1024
	case 'M', 'm':
		n = 1
	default:
		return 0, errors.Errorf("parse bandwidth '%s' error", width)
	}

	w := strings.TrimSpace(width[:len(width)-1])
	if len(w) < 1 {
		return 0, nil
	}

	num, err := strconv.Atoi(w)
	if err != nil {
		return 0, errors.Wrapf(err, "parse bandwidth '%s' error", width)
	}

	return num * n, nil
}

func parseDevice(dev string) Device {
	parts := strings.Split(dev, ",")
	if len(parts) < 6 {
		return Device{err: errors.Errorf("illegal device:'%s'", dev)}
	}

	n, err := strconv.Atoi(strings.TrimSpace(parts[5]))
	if err != nil {
		err = errors.Wrapf(err, "illegal VLAN %s,%s", parts[5], dev)
		return Device{err: err}
	}

	d := Device{
		Bond:    parts[0],
		IP:      parts[2],
		Mask:    parts[3],
		Gateway: parts[4],
		VLAN:    n,
	}

	d.Bandwidth, d.err = parseBandwidth(parts[1])

	return d
}

func ParseEngineDevice(e *cluster.Engine) ([]string, int, error) {
	if e == nil || len(e.Labels) == 0 {
		return nil, 0, nil
	}

	val, ok := e.Labels[_PFDevLabel]
	if !ok {
		return nil, 0, errors.Errorf("Engine Label:%s is required", _PFDevLabel)
	}

	total, err := parseBandwidth(val)
	if err != nil {
		return nil, 0, err
	}

	val, ok = e.Labels[_ContainerNIC]
	if !ok {
		return nil, total, errors.Errorf("Engine Label:%s is required", _ContainerNIC)
	}

	return strings.Split(val, ","), total, nil
}
