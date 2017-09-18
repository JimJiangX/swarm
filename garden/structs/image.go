package structs

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

type ImageVersion struct {
	Name  string `json:"name"`
	Major int    `json:"major_version"`
	Minor int    `json:"minor_version"`
	Patch int    `json:"patch_version"`
	Build int    `json:"build_version"`
}

func (iv ImageVersion) Version() string {
	return fmt.Sprintf("%s:%d.%d.%d.%d", iv.Name, iv.Major, iv.Minor, iv.Patch, iv.Build)
}

func NewImageVersion(name string, major, minor, patch, build int) ImageVersion {
	return ImageVersion{
		Name:  name,
		Major: major,
		Minor: minor,
		Patch: patch,
		Build: build,
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
			return iv, errors.Wrap(err, "parse image error,image:"+name)
		}

		iv.Minor, err = strconv.Atoi(dots[1])
		if err != nil {
			return iv, errors.Wrap(err, "parse image error,image:"+name)

		}

		if len(dots) > 2 {
			iv.Patch, err = strconv.Atoi(dots[2])
			if err != nil {
				return iv, errors.Wrap(err, "parse image error,image:"+name)

			}
		}

		if len(dots) > 3 {
			iv.Build, err = strconv.Atoi(string(dots[3]))
		}
	}

	if err == nil {
		return iv, nil
	}

	return iv, errors.Wrap(err, "parse image error,image:"+name)
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

	if iv.Patch != v.Patch {
		return iv.Patch < iv.Patch, nil
	}

	return iv.Build < v.Build, nil
}

type PostLoadImageRequest struct {
	ImageVersion
	Path   string            `json:"image_path"`
	Labels map[string]string `json:"labels"`
}

type Keyset struct {
	CanSet      bool   `json:"can_set"`
	MustRestart bool   `json:"must_restart"`
	Key         string `json:"key"`
	Value       string `json:"value"`
	Default     string `json:"default"`
	Desc        string `json:"desc"`
	Range       string `json:"range"`
}

type ConfigTemplate struct {
	Image string `json:"image"`
	// Mount     string
	LogMount   string `json:"log_mount"`
	DataMount  string `json:"data_mount"`
	ConfigFile string `json:"config_file"`
	Content    string `json:"content"`

	Keysets   []Keyset `json:"keysets"`
	Timestamp int64    `json:"timestamp"`
}

type UnitConfigResponse struct {
	ID      string `json:"id"`
	Service string `json:"service"`
	ConfigTemplate
	Cmds CmdsMap `json:"cmds,omitempty"`
}

type ImageResponse struct {
	ImageVersion
	Size     int    `json:"size"`
	ID       string `json:"id"`
	ImageID  string `json:"docker_image_id"`
	Labels   string `json:"label"`
	UploadAt string `json:"upload_at"`
}

type GetImageResponse struct {
	ImageResponse
	Template ConfigTemplate `json:"config_template"`
}
