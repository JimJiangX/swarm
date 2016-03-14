package gardener

import (
	"encoding/binary"
	"net"
	"strings"
)

// convertKVStringsToMap converts ["key=value"] to {"key":"value"}
func convertKVStringsToMap(values []string) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		kv := strings.SplitN(value, "=", 2)
		if len(kv) == 1 {
			result[kv[0]] = ""
		} else {
			result[kv[0]] = kv[1]
		}
	}

	return result
}

// convertMapToKVStrings converts {"key": "value"} to ["key=value"]
func convertMapToKVStrings(values map[string]string) []string {
	result := make([]string, len(values))
	i := 0
	for key, value := range values {
		result[i] = key + "=" + value
		i++
	}
	return result
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
