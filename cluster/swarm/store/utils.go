package store

import (
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

func maxIdleSizeRG(m map[database.RaidGroup]space) database.RaidGroup {
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

type space struct {
	ID     int
	Total  int
	Free   int
	State  string
	LunNum int
}

func parseSpace(output string) []space {
	var (
		spaces []space
		lines  = strings.Split(output, "\n") // lines
	)

	for i := range lines {

		part := strings.Split(lines[i], " ")

		if len(part) == 5 {
			var (
				space = space{}
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
