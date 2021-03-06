package storage

import (
	"bufio"
	"bytes"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/docker/swarm/garden/database"
	"github.com/pkg/errors"
)

func findIdleNum(min, max int, filter []int) (bool, int) {
	sort.Sort(sort.IntSlice(filter))

loop:
	for val := min; val <= max; val++ {

		for _, in := range filter {
			if val == in {
				continue loop
			}
		}

		return true, val
	}

	return false, 0
}

func intSliceToString(input []int, sep string) string {

	a := make([]string, len(input))
	for i, v := range input {
		a[i] = strconv.Itoa(v)
	}

	return strings.Join(a, sep)
}

func maxIdleSizeRG(m map[database.RaidGroup]Space) database.RaidGroup {
	var (
		key database.RaidGroup
		max int64
	)

	for k, val := range m {
		if !k.Enabled {
			continue
		}
		if val.Free > max {
			max = val.Free
			key = k
		}
	}

	return key
}

// Space ---> RG
type Space struct {
	Enable bool
	ID     string
	Total  int64
	Free   int64
	State  string
	LunNum int
}

func parseSpace(r io.Reader) (map[string]Space, []error) {
	spaces := make(map[string]Space)
	errs := make([]error, 0, 10)

	for br := bufio.NewReader(r); ; {
		line, _, err := br.ReadLine()
		if err != nil {
			if err != io.EOF {
				errs = append(errs, errors.WithStack(err))
			}
			break
		}

		out := bytes.Split(line, []byte{' '})
		parts := make([][]byte, 0, 5)

		for i := range out {
			if len(out[i]) > 0 {
				parts = append(parts, out[i])
			}
		}

		if len(parts) >= 5 {
			var (
				space = Space{}
				err   error
			)

			space.ID = string(bytes.TrimSpace(parts[0]))

			if space.ID == "" {
				errs = append(errs, errors.Errorf("RG ID is required,'%s'", line))
				continue
			}

			space.Total, err = strconv.ParseInt(string(parts[1]), 10, 64)
			if err != nil {
				errs = append(errs, errors.Errorf("parse '%s':'%s' error,%s", line, parts[1], err))
			}
			space.Total = space.Total << 20

			space.Free, err = strconv.ParseInt(string(parts[2]), 10, 64)
			if err != nil {
				errs = append(errs, errors.Errorf("parse '%s':'%s' error,%s", line, parts[2], err))
			}
			space.Free = space.Free << 20

			space.State = string(parts[3])

			space.LunNum, err = strconv.Atoi(string(parts[4]))
			if err != nil {
				errs = append(errs, errors.Errorf("parse '%s':'%s' error,%s", line, parts[4], err))
			}

			spaces[space.ID] = space
		}
	}

	return spaces, errs
}

func generateHostName(name string) string {

	name = strings.Replace(name, ":", "", -1)
	name = strings.Replace(name, "|", "", -1)

	if len(name) >= maxHostLen {
		return name[:maxHostLen]
	}

	return name
}
