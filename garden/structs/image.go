package structs

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

type ImageVersion struct {
	Name  string
	Major int
	Minor int
	Patch int
}

func (iv ImageVersion) Version() string {
	return fmt.Sprintf("%s:%d.%d.%d", iv.Name, iv.Major, iv.Minor, iv.Patch)
}

func NewImageVersion(name string, major, minor, patch int) ImageVersion {
	return ImageVersion{
		Name:  name,
		Major: major,
		Minor: minor,
		Patch: patch,
	}
}

func ParseImage(name string) (iv ImageVersion, err error) {
	slash := strings.IndexByte(name, '/')
	if slash < 0 {
		slash = 0
	}

	i := strings.LastIndexByte(name, ':')
	if i <= 0 {
		return iv, errors.New("parse image error,image:" + name)
	}

	iv.Name = name[slash:i]

	dots := strings.Split(name[i+1:], ".")
	if len(dots) >= 2 {
		iv.Major, err = strconv.Atoi(dots[0])
		if err != nil {
			return
		}

		iv.Minor, err = strconv.Atoi(dots[1])
		if err != nil {
			return
		}

		if len(dots) > 2 {
			iv.Patch, err = strconv.Atoi(string(dots[2]))
		}
	}

	return
}

func (iv ImageVersion) LessThan(v ImageVersion) (bool, error) {
	if iv.Name != v.Name {
		return false, fmt.Errorf("image name is different,'%s'!='%s'", iv.Name, v.Name)
	}

	if iv.Major != v.Major {
		return iv.Major < v.Major, nil
	}

	if iv.Minor != v.Minor {
		return iv.Minor < iv.Minor, nil
	}

	return iv.Patch < v.Patch, nil
}

type PostLoadImageRequest struct {
	ImageVersion
	Path    string
	Timeout int
	Labels  map[string]string
}

type Keyset struct {
	CanSet      bool `json:"can_set"`
	MustRestart bool `json:"must_restart"`
	Key         string
	Desc        string
	Range       string
}

type ConfigTemplate struct {
	Name      string
	Version   string
	Image     string
	Mount     string
	Content   []byte
	Keysets   []Keyset
	Timestamp int64
}
