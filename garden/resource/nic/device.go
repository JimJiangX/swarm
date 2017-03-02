package nic

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/swarm/cluster"
	"github.com/pkg/errors"
)

const (
	_PF_DEV_Label = "PF_DEV" // "PF_DEV_":"dev1,10G"

	// "VF_DEV_":"bond0,mac_xxxx,10M,192.168.1.1,255.255.255.0,192.168.3.0,vlan_xxxx"
	_VF_DEV_Prefix = "VF_DEV_"
)

type Device struct {
	Bond      string
	MAC       string
	Bandwidth int // M/s
	IP        string
	Mask      string
	Gateway   string
	VLAN      string
	err       error
}

// "VF_DEV_0":"bond0,mac_xxxx,10M,192.168.1.1,255.255.255.0,192.168.3.0,vlan_xxxx"
func (d Device) String() string {
	if d.err != nil {
		return d.err.Error()
	}
	return fmt.Sprintf("%s,%s,%dM,%s,%s,%s,%s", d.Bond, d.MAC, d.Bandwidth,
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
	if len(parts) < 7 {
		return Device{err: errors.Errorf("illegal device:'%s'", dev)}
	}

	d := Device{
		Bond:    parts[0],
		MAC:     parts[1],
		IP:      parts[3],
		Mask:    parts[4],
		Gateway: parts[5],
		VLAN:    parts[6],
	}

	d.Bandwidth, d.err = parseBandwidth(parts[2])

	return d
}

func parseContainerDevice(c *cluster.Container) []Device {
	if c == nil {
		return nil
	}

	out := make([]Device, 0, 2)

	for key, val := range c.Labels {
		if !strings.HasPrefix(key, _VF_DEV_Prefix) {
			continue
		}

		d := parseDevice(val)
		if d.Bond != "" {
			out = append(out, d)
		}
	}

	if c.Config != nil && len(c.Config.Labels) > 0 {

		for key, val := range c.Config.Labels {
			if c.Labels[key] == c.Config.Labels[key] {
				continue
			}

			if !strings.HasPrefix(key, _VF_DEV_Prefix) {
				continue
			}

			d := parseDevice(val)
			if d.Bond != "" {
				out = append(out, d)
			}
		}
	}

	return out
}

func ParseEngineNetDevice(e *cluster.Engine) (map[string]Device, int, error) {
	if e == nil || len(e.Labels) == 0 {
		return nil, 0, nil
	}

	devm := make(map[string]Device, len(e.Labels))
	total := 0

	for key, val := range e.Labels {
		if key == _PF_DEV_Label {
			i := strings.LastIndexByte(val, ',')
			n, err := parseBandwidth(string(val[i+1:]))
			if err != nil {
				return nil, 0, err
			}

			total = n

		} else if strings.HasPrefix(key, _VF_DEV_Prefix) {
			parts := strings.Split(val, ",")
			if len(parts) >= 2 {
				devm[parts[0]] = Device{
					Bond: parts[0],
					MAC:  parts[1],
				}
			}
		}
	}

	for _, c := range e.Containers() {

		out := parseContainerDevice(c)

		for i := range out {
			if out[i].err != nil {
				delete(devm, out[i].Bond)
				continue
			}
			if _, exist := devm[out[i].Bond]; exist {
				devm[out[i].Bond] = out[i]
			}
		}
	}

	return devm, total, nil
}

func SaveIntoContainerLabel(config *cluster.ContainerConfig, devices []Device) {
	if config == nil {
		return
	}

	if config.Labels == nil {
		config.Labels = make(map[string]string, 5)
	}

	for i := range devices {
		key := fmt.Sprintf("%s%d", _VF_DEV_Prefix, i)
		config.Config.Labels[key] = devices[i].String()
	}
}
