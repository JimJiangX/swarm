package utils

import (
	"context"
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

// Generate8UUID is used to generate a random UUID,lenth of string is 8
func Generate8UUID() string {
	return GenerateUUID(8)
}

// Generate16UUID is used to generate a random UUID,lenth of string is 16
func Generate16UUID() string {
	return GenerateUUID(16)
}

// Generate32UUID is used to generate a random UUID,lenth of string is 32
func Generate32UUID() string {
	return GenerateUUID(32)
}

// Generate64UUID is used to generate a random UUID,lenth of string is 64
func Generate64UUID() string {
	return GenerateUUID(64)
}

// Generate128UUID is used to generate a random UUID,lenth of string is 128
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

// DecodeBody is used to JSON decode a body
func DecodeBody(resp *http.Response, out interface{}) error {
	dec := json.NewDecoder(resp.Body)
	return dec.Decode(out)
}

// Base64Decode decode base64 string,return username,password
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

// Base64Encode encode string by base64
func Base64Encode(username, password string) string {
	src := []byte(username + ":" + password)
	return base64.StdEncoding.EncodeToString(src)
}

// IPToUint32 convert a IP string to unit32
func IPToUint32(ip string) uint32 {
	addr := net.ParseIP(ip)
	if addr == nil {
		return 0
	}
	return binary.BigEndian.Uint32(addr.To4())
}

// Uint32ToIP convert a unit32 to IP
func Uint32ToIP(cidr uint32) net.IP {
	addr := make([]byte, 4)
	binary.BigEndian.PutUint32(addr, cidr)
	return net.IP(addr)
}

const defaultTimeLayout = "2006-01-02 15:04:05"

// TimeToString format a time t to string,time loyout is "2006-01-02 15:04:05"
func TimeToString(t time.Time) string {
	if !t.IsZero() {
		return t.Format(defaultTimeLayout)
	}
	return ""
}

// ParseStringToTime returns local time with time loyout "2006-01-02 15:04:05",local zone
func ParseStringToTime(s string) (time.Time, error) {
	t, err := time.Parse(defaultTimeLayout, s)
	if err != nil {
		return time.Time{}, err
	}

	local := t.Local()
	_, offset := local.Zone()

	return local.Add(-time.Duration(offset) * time.Second), nil
}

// ExecScript returns a command to execute a script.
func ExecScript(args ...string) *exec.Cmd {
	return ExecContext(context.Background(), args...)
}

// ExecContext returns a context command to execute a script.
func ExecContext(ctx context.Context, args ...string) *exec.Cmd {
	shell, flag := "/bin/bash", "-c"

	if runtime.GOOS == "windows" {
		shell = "cmd"
		flag = "/C"
	}

	return exec.CommandContext(ctx, shell, flag, strings.Join(args, " "))
}

// ExecContextTimeout exec command with timeout
func ExecContextTimeout(ctx context.Context, timeout time.Duration, debug bool, args ...string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := ExecContext(ctx, args...)
	if debug {
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		fmt.Println(cmd.Args)
	}

	err := cmd.Start()
	if err != nil {
		return nil, err
	}

	wait := make(chan error, 1)
	go func() {
		wait <- cmd.Wait()
		close(wait)
	}()

	select {
	case err = <-wait:
	case <-ctx.Done():
		err = ctx.Err()
	}

	return nil, err
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

// GetAbsolutePath returns absolute path
func GetAbsolutePath(isDir bool, path ...string) (string, error) {
	dir := filepath.Join(path...)
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}

	finfo, err := os.Stat(abs)
	if os.IsNotExist(err) {
		return abs, err
	}

	if isDir && !finfo.IsDir() {
		return abs, fmt.Errorf("%s is not a directory", abs)
	}

	if !isDir && finfo.IsDir() {
		return abs, fmt.Errorf("%s is a directory", abs)
	}

	return abs, nil
}

// CountCPU returns CPU num,calls ParseUintList
func CountCPU(val string) (int64, error) {
	if val == "" {
		return 0, nil
	}

	cpus, err := ParseUintList(val)
	if err != nil {
		return 0, err
	}

	return int64(len(cpus)), nil
}

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
// copy from github.com/moby/moby/pkg/parsers/parsers.go#L34
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
