package structs

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"sort"

	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type ServiceLink struct {
	priority int
	Spec     *ServiceSpec

	Arch Arch     `json:"architecture"`
	ID   string   `json:"id"`
	Deps []string `json:"deps"`
}

type ServicesLink struct {
	Mode     string
	NameOrID string // service id or name,unit id or name or containerID
	Links    []*ServiceLink
}

func (sl ServicesLink) Less(i, j int) bool {
	return sl.Links[i].priority < sl.Links[j].priority
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
	ServiceID     string
	ConfigFile    string
	ConfigContent string
	Commands      []string
	Request       *HTTPRequest `json:"request,omitempty"`
}

type ServiceLinkResponse struct {
	Links   []UnitLink
	Compose []string
}

type HTTPRequest struct {
	Method string
	URL    string `json:"url"`
	Body   []byte
	Header map[string][]string
}

// Send send request to remote server
func (r HTTPRequest) Send(ctx context.Context) error {
	req, err := http.NewRequest(r.Method, r.URL, bytes.NewReader(r.Body))
	if err != nil {
		return err
	}

	if ctx != nil {
		req = req.WithContext(ctx)
	}

	for key, val := range r.Header {
		for i := range val {
			req.Header.Add(key, val[i])
		}
	}

	resp, err := requireOK(http.DefaultClient.Do(req))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	io.CopyN(ioutil.Discard, resp.Body, 512)

	return err
}

func requireOK(resp *http.Response, e error) (*http.Response, error) {
	if e != nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		return nil, e
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		buf := bytes.NewBuffer(nil)

		io.Copy(buf, resp.Body)
		resp.Body.Close()

		return nil, errors.Errorf("%s,Unexpected response code: %d (%s)", resp.Request.URL.String(), resp.StatusCode, buf.Bytes())
	}

	return resp, nil
}
