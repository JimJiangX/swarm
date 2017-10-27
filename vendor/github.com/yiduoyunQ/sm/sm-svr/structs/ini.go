package structs

import (
	"bufio"
	"strings"
)

func ReadIniFileByLine(r *bufio.Reader) (string, string, error) {
	b, _, err := r.ReadLine()
	if err != nil {
		return "", "", err
	}

	s := strings.TrimSpace(string(b))
	if strings.Index(s, "#") == 0 {
		return "", "", nil
	}

	index := strings.Index(s, "=")
	if index < 0 {
		return "", "", nil
	}

	key := strings.TrimSpace(s[:index])
	if len(key) == 0 {
		return "", "", nil
	}

	val := strings.TrimSpace(s[index+1:])

	return key, val, nil
}
