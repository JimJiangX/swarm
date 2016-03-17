package gardener

import (
	"encoding/binary"
	"net"
)

func IPToUint32(ip string) uint32 {
	addr := net.ParseIP(ip)
	if addr == nil {
		return 0
	}
	return binary.BigEndian.Uint32(addr.To4())
}

func Uint32ToIP(cidr uint32) net.IP {
	addr := make([]byte, 4)
	binary.BigEndian.PutUint32(addr, cidr)
	return net.IP(addr)
}
