package storage

import (
	"bufio"
	"bytes"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/docker/swarm/garden/database"
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

func parseSpace(r io.Reader) []Space {
	spaces := make([]Space, 0, 10)
	br := bufio.NewReader(r)

	for {
		line, _, err := br.ReadLine()
		if err != nil {
			break
		}

		parts := bytes.Split(line, []byte{' '})

		if len(parts) == 5 {
			var (
				space = Space{}
				err   error
			)
			space.ID = string(parts[0])

			space.Total, err = strconv.ParseInt(string(parts[1]), 10, 64)
			if err != nil {
				continue
			}
			space.Total = space.Total << 20

			space.Free, err = strconv.ParseInt(string(parts[2]), 10, 64)
			if err != nil {
				continue
			}
			space.Free = space.Free << 20

			space.State = string(parts[3])

			space.LunNum, err = strconv.Atoi(string(parts[4]))
			if err != nil {
				continue
			}

			spaces = append(spaces, space)
		}
	}

	return spaces
}
