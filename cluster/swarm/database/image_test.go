package database

import (
	"testing"
	"time"
)

func TestTXInsertUnitConfig(t *testing.T) {
	tx, err := GetTX()
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
		configKeySets: "",
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
