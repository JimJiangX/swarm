package database

import (
	"testing"
	"time"
)

func TestTXInsertUnitConfig(t *testing.T) {
	db, err := GetDB(true)
	if err != nil {
		t.Fatal(err)
	}

	tx, err := db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	config := &UnitConfig{
		ID:       "test1",
		ImageID:  "image1",
		Path:     "/root/abc",
		Version:  0,
		ParentID: "",
		Content: `qwertazwk,ol.p;/['sxecrfvtgbyhn 
		ujmiyuiop[]\][as"""dfghjkl'';'zxcvbnm,./'"'''""`,
		ConfigKeySets: "",
		KeySets:       map[string]bool{"abc": false, "def": true},
		CreatedAt:     time.Now(),
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
