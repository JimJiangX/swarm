package resource

import (
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/utils"
	"github.com/pkg/errors"
)

type networks struct {
	nwo database.NetworkingOrmer
}

func newNetworks(nwo database.NetworkingOrmer) networks {
	return networks{
		nwo: nwo,
	}
}

func (nws networks) AddNetworking(start, end, gateway, _type string, prefix int) (database.Networking, []database.IP, error) {
	startU32 := utils.IPToUint32(start)
	endU32 := utils.IPToUint32(end)
	none := database.Networking{}

	if move := uint(32 - prefix); (startU32 >> move) != (endU32 >> move) {
		return none, nil, errors.Errorf("%s-%s is different network segments", start, end)
	}
	if startU32 > endU32 {
		startU32, endU32 = endU32, startU32
	}

	net := database.Networking{
		ID:      utils.Generate64UUID(),
		Type:    _type,
		Gateway: gateway,
		Enabled: true,
	}

	num := int(endU32 - startU32 + 1)
	ips := make([]database.IP, num)
	for i := range ips {
		ips[i] = database.IP{
			IPAddr:       startU32,
			Prefix:       prefix,
			NetworkingID: net.ID,
			Allocated:    false,
		}

		startU32++
	}

	err := nws.nwo.InsertNetworking(net, ips)
	if err != nil {
		return none, nil, err
	}

	return net, ips, nil
}
