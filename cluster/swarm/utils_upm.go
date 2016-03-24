package swarm

import (
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
)

func Generate8UUID() string {
	return generateUUID(8)
}

func Generate16UUID() string {
	return generateUUID(16)
}

func Generate32UUID() string {
	return generateUUID(32)
}

func Generate64UUID() string {
	return generateUUID(64)
}

func Generate128UUID() string {
	return generateUUID(128)
}

// generateUUID is used to generate a random UUID
func generateUUID(length int) string {
	buf := make([]byte, length/2)
	if _, err := crand.Read(buf); err != nil {
		panic(fmt.Errorf("failed to read random bytes: %v", err))
	}
	switch length {
	case 8:
		return fmt.Sprintf("%8x", buf)
	case 16:
		return fmt.Sprintf("%16x", buf)
	case 32:
		return fmt.Sprintf("%32x", buf)
	case 64:
		return fmt.Sprintf("%64x", buf)
	case 128:
		return fmt.Sprintf("%128x", buf)
	}
	return ""
}

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
