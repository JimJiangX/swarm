package parser

import (
	"testing"
)

func TestRegister(t *testing.T) {
	err := register("switch_manager", "1.0", &switchManagerConfig{})
	if err != nil {
		t.Error(err)
	}
	err = register("switch_manager", "1.1.19", &switchManagerConfigV1119{})
	if err != nil {
		t.Error(err)
	}
	err = register("switch_manager", "1.1.23", &switchManagerConfigV1123{})
	if err != nil {
		t.Error(err)
	}
	err = register("switch_manager", "1.1.47", &switchManagerConfigV1147{})
	if err != nil {
		t.Error(err)
	}
	err = register("switch_manager", "1.2.0", &switchManagerConfigV120{})
	if err != nil {
		t.Error(err)
	}
	err = register("switch_manager", "1.3.0", &switchManagerConfigV120{})
	if err != nil {
		t.Error(err)
	}

	err = register("switch_manager", "2.0", &switchManagerConfigV120{})
	if err != nil {
		t.Error(err)
	}
}

func TestFactory(t *testing.T) {
	pr, err := factory("switch_manager:2.0.6.5")
	if err != nil || pr == nil {
		t.Error(pr == nil, err)
	}

}
