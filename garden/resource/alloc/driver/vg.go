package driver

import (
	"github.com/docker/swarm/garden/database"
	"github.com/pkg/errors"
)

type vgIface interface {
	ActivateVG(v database.Volume) error
	DeactivateVG(v database.Volume) error
	createVG(lvs []database.Volume) error
	expandVG(luns []database.LUN) error
}

type unsupportSAN struct{}

var errUnsupportSAN = errors.New("unsupport SAN error")

func (unsupportSAN) ActivateVG(v database.Volume) error {
	return errUnsupportSAN
}

func (unsupportSAN) DeactivateVG(v database.Volume) error {
	return errUnsupportSAN
}

func (unsupportSAN) createVG(v []database.Volume) error {
	return nil
}

func (unsupportSAN) expandVG(luns []database.LUN) error {
	return nil
}

func (unsupportSAN) updateVolume(v database.Volume) error {
	return nil
}
