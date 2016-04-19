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
		ID:            "test1",
		ImageID:       "image1",
		Path:          "/root/abc",
		Version:       0,
		ParentID:      "",
		Content:       "",
		ConfigKeySets: "",
		KeySets:       map[string]bool{"abc": false, "def": true},
		CreatedAt:     time.Now(),
	}

	err = TXInsertUnitConfig(tx, config)
	if err != nil {
		t.Fatal(err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatal(err)
	}

	err = DeleteUnitConfig(config.ID)
	if err != nil {
		t.Fatal(err)
	}
}
