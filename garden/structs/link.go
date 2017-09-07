package structs

import "sort"

type ServiceLink struct {
	priority int
	Spec     *ServiceSpec `json:"-"`

	Arch Arch     `json:"architecture"`
	ID   string   `json:"from_service_name"`
	Deps []string `json:"to_services_name"`
}

type ServicesLink struct {
	Mode  string
	Links []*ServiceLink
}

func (sl ServicesLink) Less(i, j int) bool {
	return sl.Links[i].priority > sl.Links[j].priority
}

// Len is the number of elements in the collection.
func (sl ServicesLink) Len() int {
	return len(sl.Links)
}

// Swap swaps the elements with indexes i and j.
func (sl ServicesLink) Swap(i, j int) {
	sl.Links[i], sl.Links[j] = sl.Links[j], sl.Links[i]
}

// https://play.golang.org/p/1tkv9z4DtC
func (sl ServicesLink) Sort() {
	deps := make(map[string]int, len(sl.Links))

	for i := range sl.Links {
		deps[sl.Links[i].ID] = len(sl.Links[i].Deps)
	}

	for i := len(sl.Links) - 1; i > 0; i-- {
		for _, s := range sl.Links {

			max := 0

			for _, id := range s.Deps {
				if n := deps[id]; n > max {
					max = n + 1
				}
			}
			if max > 0 {
				deps[s.ID] = max
			}
		}
	}

	for i := range sl.Links {
		sl.Links[i].priority = deps[sl.Links[i].ID]
	}

	sort.Sort(sl)
}

func (sl ServicesLink) LinkIDs() []string {
	l := make([]string, 0, len(sl.Links)*2)
	for i := range sl.Links {
		l = append(l, sl.Links[i].ID)
		l = append(l, sl.Links[i].Deps...)
	}

	ids := make([]string, 0, len(l))

	for i := range l {
		ok := false
		for c := range ids {
			if ids[c] == l[i] {
				ok = true
				break
			}
		}
		if !ok {
			ids = append(ids, l[i])
		}
	}

	return ids
}

type UnitLink struct {
	NameOrID      string
	ConfigFile    string
	ConfigContent string
	Commands      []string
}

type ServiceLinkResponse struct {
	Links   []UnitLink
	Compose []string
}
