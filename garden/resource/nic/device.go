package nic

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/swarm/cluster"
	"github.com/pkg/errors"
)

const (
	_PFDevLabel = "PF_DEV_BW" // "PF_DEV_BW":"10G"

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

func ParseTotalDevice(e *cluster.Engine) (map[string]Device, int, error) {
	if e == nil || len(e.Labels) == 0 {
		return nil, 0, nil
	}

	devm := make(map[string]Device, len(e.Labels))
	total := 0

	for key, val := range e.Labels {
		if key == _PFDevLabel {
			n, err := parseBandwidth(val)
			if err != nil {
				return nil, 0, err
			}

			total = n

		} else if key == _ContainerNIC {
			devs := strings.Split(val, ",")
			for i := range devs {
				devm[devs[i]] = Device{
					Bond: devs[i],
				}
			}
		}
	}

	return devm, total, nil
}
