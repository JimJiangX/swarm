package database

import (
	"testing"
	"time"

	"github.com/pkg/errors"
)

func DeleteUnitConfig(id string) error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	query := "DELETE FROM tbl_dbaas_unit_config WHERE id=?"
	_, err = db.Exec(query, id)
	if err == nil {
		return nil
	}

	db, err = getDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, id)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Delete UnitConfig")
}

func TestTXInsertUnitConfig(t *testing.T) {
	tx, err := getTX()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	config := &UnitConfig{
		ID:       "test1",
		ImageID:  "image1",
		Mount:    "/root/abc",
		Version:  0,
		ParentID: "",
		Content: `qwertazwk,ol.p;/['sxecrfvtgbyhn 
		ujmiyuiop[]\][as"""dfghjkl'';'zxcvbnm,./'"'''""`,
		KeySets: map[string]KeysetParams{
			"abc": KeysetParams{
				Key:         "abc",
				CanSet:      true,
				MustRestart: false,
			},
			"def": KeysetParams{
				Key:         "def",
				CanSet:      false,
				MustRestart: true,
			},
		},
		CreatedAt: time.Now(),
	}

	err = TXInsertUnitConfig(tx, config)
	if err != nil {
		t.Fatal(err)
	}

	defer func(id string) {
		err := DeleteUnitConfig(id)
		if err != nil {
			t.Fatal(err)
		}
	}(config.ID)

	err = tx.Commit()
	if err != nil {
		t.Fatal(err)
	}
}
