package swarm

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
)

func (u unit) Path() string {
	if u.parent == nil {
		return ""
	}

	return u.parent.Path
}

func (u unit) CanModify(data map[string]interface{}) ([]string, bool) {
	if len(u.parent.KeySets) == 0 {
		return nil, true
	}

	can := true
	keys := make([]string, 0, len(u.parent.KeySets))

	for key := range data {
		if !u.parent.KeySets[key] {
			keys = append(keys, key)
			can = false
		}
	}

	return keys, can
}

func (u *unit) Merge(data map[string]interface{}) error {
	if keys, ok := u.CanModify(data); !ok {

		return fmt.Errorf("Keys cannot set new value,%s", keys)
	}

	if u.parent.ContentMap == nil {
		u.content = data
		return nil
	}

	if u.content == nil {
		u.content = make(map[string]interface{})
	}

	for key, val := range data {
		u.content[key] = val
	}

	return nil
}

func (u unit) Verify(data map[string]interface{}) error {
	if len(data) > 0 {
		if err := u.verify(data); err != nil {
			return err
		}
	}

	if len(u.content) > 0 {
		if err := u.verify(u.content); err != nil {
			return err
		}
	}

	return nil
}

func (u *unit) Set(key string, val interface{}) error {
	if !u.parent.KeySets[key] {
		return fmt.Errorf("%s cannot Set new Value", key)
	}

	u.content[key] = val

	return nil
}

func (u *unit) Marshal() ([]byte, error) {

	return json.Marshal(u.content)
}

func (u *unit) SaveToDisk() (string, error) {
	config := database.UnitConfig{
		ID:         utils.Generate64UUID(),
		ImageID:    u.ImageID,
		Version:    u.parent.Version + 1,
		ParentID:   u.parent.ID,
		ContentMap: u.content,
		KeySets:    u.parent.KeySets,
		Path:       u.Path(),
		CreateAt:   time.Now(),
	}

	u.Unit.ConfigID = config.ID

	err := database.SaveUnitConfigToDisk(&u.Unit, config)
	if err != nil {
		return "", err
	}

	return config.ID, nil
}

type mysqlCmd struct {
	unit *database.Unit
}

func NewMysqlCmd(unit *database.Unit) *mysqlCmd {
	return &mysqlCmd{
		unit: unit,
	}
}

func (mysqlCmd) StartContainerCmd() []string     { return nil }
func (mysqlCmd) StartServiceCmd() []string       { return nil }
func (mysqlCmd) StopServiceCmd() []string        { return nil }
func (mysqlCmd) RecoverCmd(file string) []string { return nil }
func (mysqlCmd) BackupCmd() []string             { return nil }

func verifyMysqlConfig(data map[string]interface{}) error {
	return nil
}
