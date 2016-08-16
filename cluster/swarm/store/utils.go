package store

import (
	"bufio"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/docker/swarm/cluster/swarm/database"
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
		max int
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

type Space struct {
	Enable bool
	ID     int
	Total  int
	Free   int
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

		part := strings.Split(string(line), " ")

		if len(part) == 5 {
			var (
				space = Space{}
				err   error
			)
			space.ID, err = strconv.Atoi(part[0])
			if err != nil {
				continue
			}
			space.Total, err = strconv.Atoi(part[1])
			if err != nil {
				continue
			}
			space.Total = space.Total << 20

			space.Free, err = strconv.Atoi(part[2])
			if err != nil {
				continue
			}
			space.Free = space.Free << 20

			space.State = part[3]

			space.LunNum, err = strconv.Atoi(part[4])
			if err != nil {
				continue
			}

			spaces = append(spaces, space)
		}
	}

	return spaces
}
