package nic

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/swarm/cluster"
	"github.com/pkg/errors"
)

const (
	_PFDevLabel = "PF_DEV" // "PF_DEV_":"dev1,10G"

	// "VF_DEV_":"bond0,10M,192.168.1.1,255.255.255.0,192.168.3.0,vlan_xxxx"
	_VFDevPrefix = "VF_DEV_"
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

func parseContainerDevice(c *cluster.Container) []Device {
	if c == nil {
		return nil
	}

	out := make([]Device, 0, 2)

	for key, val := range c.Labels {
		if !strings.HasPrefix(key, _VFDevPrefix) {
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

			if !strings.HasPrefix(key, _VFDevPrefix) {
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

func ParseTotalDevice(e *cluster.Engine) (map[string]Device, int, error) {
	if e == nil || len(e.Labels) == 0 {
		return nil, 0, nil
	}

	devm := make(map[string]Device, len(e.Labels))
	total := 0

	for key, val := range e.Labels {
		if key == _PFDevLabel {
			i := strings.LastIndexByte(val, ',')
			n, err := parseBandwidth(string(val[i+1:]))
			if err != nil {
				return nil, 0, err
			}

			total = n

		} else if strings.HasPrefix(key, _VFDevPrefix) {
			parts := strings.Split(val, ",")
			if len(parts) >= 2 {
				devm[parts[0]] = Device{
					Bond: parts[0],
				}
			}
		}
	}

	return devm, total, nil

}

func ParseEngineNetDevice(e *cluster.Engine) (map[string]Device, int, error) {
	if e == nil || len(e.Labels) == 0 {
		return nil, 0, nil
	}

	devm, total, err := ParseTotalDevice(e)
	if err != nil {
		return devm, total, err
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
		key := fmt.Sprintf("%s%d", _VFDevPrefix, i)
		config.Config.Labels[key] = devices[i].String()
	}
}
