package swarm

import (
	"fmt"
	"strings"
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

func (u unit) Verify(data map[string]interface{}) error {
	if len(data) > 0 {
		if err := u.Validate(data); err != nil {
			return err
		}
	}

	if len(u.content) > 0 {
		if err := u.Validate(u.content); err != nil {
			return err
		}
	}

	return nil
}

func (u *unit) Merge(data map[string]interface{}) error {
	if keys, ok := u.CanModify(data); !ok {

		return fmt.Errorf("Keys cannot set new value,%s", keys)
	}

	if u.content == nil {
		content, err := u.Parse(u.parent.Content)
		if err != nil {
			return err
		}

		if content == nil {
			u.content = data
			return nil
		}

		u.content = content
	}

	for key, val := range data {
		u.content[key] = val
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

func (u *unit) SaveToDisk(content string) (string, error) {
	if content == "" {
		data, err := u.Marshal(u.content)
		if err != nil {
			return "", err
		}

		content = string(data)
	}

	config := database.UnitConfig{
		ID:       utils.Generate64UUID(),
		ImageID:  u.ImageID,
		Version:  u.parent.Version + 1,
		ParentID: u.parent.ID,
		Content:  content,
		KeySets:  u.parent.KeySets,
		Path:     u.Path(),
		CreateAt: time.Now(),
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
func (mysqlCmd) BackupCmd() []string {
	return []string{"/root/upsql-backup.sh"}
}
func (mysqlCmd) CleanBackupFileCmd() []string { return nil }

type mysqlConfig struct{}

func (mysqlConfig) Validate(data map[string]interface{}) error {
	return nil
}

func (mysqlConfig) Parse(val string) (map[string]interface{}, error) {
	// ini/json/xml
	// convert to map[string]interface{}

	if strings.TrimSpace(val) == "" {
		return nil, nil
	}

	return nil, nil
}

func (mysqlConfig) Marshal(data map[string]interface{}) ([]byte, error) {
	// map[string]interface{} convert to  ini/json/xml
	// json.Marshal(data)

	return nil, nil
}
