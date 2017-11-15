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

func parseSpace(r io.Reader) ([]Space, []error) {
	spaces := make([]Space, 0, 10)
	errs := make([]error, 0, 10)

	br := bufio.NewReader(r)

	for {
		line, _, err := br.ReadLine()
		if err != nil {
			if err != io.EOF {
				errs = append(errs, errors.WithStack(err))
			}
			break
		}

		parts := bytes.Split(line, []byte{' '})

		if len(parts) >= 5 {

			if len(bytes.TrimSpace(parts[0])) == 0 {
				errs = append(errs, errors.Errorf("RG ID is required,'%s'", line))
				continue
			}

			var (
				space = Space{}
				err   error
			)

			space.ID = string(parts[0])

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

			spaces = append(spaces, space)
		}
	}

	return spaces, errs
}
