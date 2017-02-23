package resource

import (
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/utils"
	"github.com/pkg/errors"
)

type Networking struct {
	nwo database.NetworkingOrmer
}

func NewNetworks(nwo database.NetworkingOrmer) Networking {
	return Networking{
		nwo: nwo,
	}
}

func (nw Networking) AddNetworking(start, end, gateway, networkingID, vlan string, prefix int) (int, error) {
	startU32 := utils.IPToUint32(start)
	endU32 := utils.IPToUint32(end)

	if move := uint(32 - prefix); (startU32 >> move) != (endU32 >> move) {
		return 0, errors.Errorf("%s-%s is different network segments", start, end)
	}
	if startU32 > endU32 {
		startU32, endU32 = endU32, startU32
	}

	num := int(endU32 - startU32 + 1)
	ips := make([]database.IP, num)
	for i := range ips {
		ips[i] = database.IP{
			IPAddr:     startU32,
			Prefix:     prefix,
			Networking: networkingID,
			VLAN:       vlan,
			Gateway:    gateway,
			Enabled:    true,
		}

		startU32++
	}

	err := nw.nwo.InsertNetworking(ips)
	if err != nil {
		return 0, err
	}

	return len(ips), nil
}
