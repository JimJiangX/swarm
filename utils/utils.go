package utils

import (
	crand "crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func Generate8UUID() string {
	return GenerateUUID(8)
}

func Generate16UUID() string {
	return GenerateUUID(16)
}

func Generate32UUID() string {
	return GenerateUUID(32)
}

func Generate64UUID() string {
	return GenerateUUID(64)
}

func Generate128UUID() string {
	return GenerateUUID(128)
}

// GenerateUUID is used to generate a random UUID
func GenerateUUID(length int) string {
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

/*
// generateUUID returns an unique id
func generateUUID() string {
	for {
		id := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, id); err != nil {
			panic(err) // This shouldn't happen
		}
		value := hex.EncodeToString(id)
		// if we try to parse the truncated for as an int and we don't have
		// an error then the value is all numberic and causes issues when
		// used as a hostname. ref #3869
		if _, err := strconv.ParseInt(TruncateID(value), 10, 64); err == nil {
			continue
		}
		return value
	}
}

func TruncateID(id string) string {
	shortLen := 12
	if len(id) < shortLen {
		shortLen = len(id)
	}
	return id[:shortLen]
}
*/

// RandomNumber returns a non-negative pseudo-random int.
func RandomNumber() int {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return r.Int()
}

// decodeBody is used to JSON decode a body
func DecodeBody(resp *http.Response, out interface{}) error {
	dec := json.NewDecoder(resp.Body)
	return dec.Decode(out)
}

// decode base64 string,return username,password
// http://play.golang.org/p/CNIwzF1L6l
func Base64Decode(auth string) (username, password string, err error) {
	authb, err := base64.StdEncoding.DecodeString(auth)
	if err != nil {
		return "", "", err
	}
	cone := strings.Split(string(authb), ":")
	username = cone[0]
	if len(cone) > 1 {
		password = cone[1]
	}
	return username, password, err
}

func Base64Encode(username, password string) string {
	src := []byte(username + ":" + password)
	return base64.StdEncoding.EncodeToString(src)
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

const defaultTimeLayout = "2006-01-02 15:04:05"

func TimeToString(t time.Time) string {
	if !t.IsZero() {
		return t.Format(defaultTimeLayout)
	}
	return ""
}

func ParseStringToTime(s string) (time.Time, error) {
	return time.Parse(defaultTimeLayout, s)
}

// ExecScript returns a command to execute a script
func ExecScript(script ...string) (*exec.Cmd, error) {
	var shell, flag string
	if runtime.GOOS == "windows" {
		shell = "cmd"
		flag = "/C"
	} else {
		shell = "/bin/sh"
		flag = "-c"
	}

	if other := os.Getenv("SHELL"); other != "" {
		shell = other
	}

	slice := make([]string, len(script)+1)
	slice[0] = flag
	copy(slice[1:], script)

	cmd := exec.Command(shell, slice...)
	fmt.Println("exec:", shell, slice...)

	return cmd, nil
}

// GetPrivateIP is used to return the first private IP address
// associated with an interface on the machine
func GetPrivateIP(addr string) (net.IP, error) {
	if addr == "localhost" {
		addr = "127.0.0.1"
	}
	ipnet := net.ParseIP(addr)
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	// Find private IPv4 address
	var ip net.IP
	for _, rawAddr := range addresses {
		switch addr := rawAddr.(type) {
		case *net.IPAddr:
			ip = addr.IP
		case *net.IPNet:
			ip = addr.IP
		default:
			continue
		}
		if ip.To4() == nil {
			continue
		}
		if ipnet.Equal(ip) {
			return ip.To4(), nil
		}
	}
	return nil, fmt.Errorf("private IP not found,%s", addr)
}

func GetAbsolutePath(isDir bool, path ...string) (string, error) {
	dir := filepath.Join(path...)
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}

	finfo, err := os.Stat(abs)
	if os.IsNotExist(err) {
		// no such file or dir
		return abs, err
	}

	if isDir && !finfo.IsDir() {
		// it's a directory
		return abs, fmt.Errorf("%s is not a directory", abs)
	}

	if !isDir && finfo.IsDir() {
		// it's a directory
		return abs, fmt.Errorf("%s is a directory", abs)
	}

	return abs, nil
}

func GetCPUNum(val string) (int64, error) {
	cpus, err := ParseUintList(val)
	if err != nil {
		return 0, err
	}

	ncpu := int64(0)

	for _, v := range cpus {
		if v {
			ncpu++
		}
	}

	return ncpu, nil
}

// copy from github.com/docker/docker/pkg/parsers.go
// ParseUintList parses and validates the specified string as the value
// found in some cgroup file (e.g. `cpuset.cpus`, `cpuset.mems`), which could be
// one of the formats below. Note that duplicates are actually allowed in the
// input string. It returns a `map[int]bool` with available elements from `val`
// set to `true`.
// Supported formats:
//     7
//     1-6
//     0,3-4,7,8-10
//     0-0,0,1-7
//     03,1-3      <- this is gonna get parsed as [1,2,3]
//     3,2,1
//     0-2,3,1
func ParseUintList(val string) (map[int]bool, error) {
	if val == "" {
		return map[int]bool{}, nil
	}

	availableInts := make(map[int]bool)
	split := strings.Split(val, ",")
	errInvalidFormat := fmt.Errorf("invalid format: %s", val)

	for _, r := range split {
		if !strings.Contains(r, "-") {
			v, err := strconv.Atoi(r)
			if err != nil {
				return nil, errInvalidFormat
			}
			availableInts[v] = true
		} else {
			split := strings.SplitN(r, "-", 2)
			min, err := strconv.Atoi(split[0])
			if err != nil {
				return nil, errInvalidFormat
			}
			max, err := strconv.Atoi(split[1])
			if err != nil {
				return nil, errInvalidFormat
			}
			if max < min {
				return nil, errInvalidFormat
			}
			for i := min; i <= max; i++ {
				availableInts[i] = true
			}
		}
	}
	return availableInts, nil
}
