package driver

import (
	"testing"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
)

func testLocalVolumeMap(lvm *localVolumeMap, t *testing.T) {
	base := lvm.len()
	hdd, err := lvm.ListVolumeByVG("_HDDVGLabel")
	if err != nil {
		t.Error(err)
	}
	ssd, err := lvm.ListVolumeByVG("_SSDVGLabel")
	if err != nil {
		t.Error(err)
	}

	{
		// test InsertVolume
		err = lvm.InsertVolume(database.Volume{
			ID:   "lv0001",
			Name: "lv0001",
			VG:   "_HDDVGLabel",
			Size: 3247441,
		})
		if err != nil {
			t.Error(err)
		}

		err = lvm.InsertVolume(database.Volume{
			ID:   "lv0002",
			Name: "lv0001", // duplicate name
			VG:   "_HDDVGLabel",
			Size: 3247441,
		})
		if err == nil {
			t.Error(err)
		}

		err = lvm.InsertVolume(database.Volume{
			ID:   "lv0003",
			Name: "lv0003",
			VG:   "_SSDVGLabel",
			Size: 3247341,
		})
		if err != nil {
			t.Error(err)
		}

		err = lvm.InsertVolume(database.Volume{
			ID:   "lv0004",
			Name: "lv00004",
			VG:   "_HDDVGLabel",
			Size: 3247441,
		})
		if err != nil {
			t.Error(err)
		}

		if lvm.len() != base+3 {
			t.Errorf("expected %d but got %d", base+3, lvm.len())
		}

		// test ListVolumeByVG
		hdd1, err := lvm.ListVolumeByVG("_HDDVGLabel")
		if err != nil {
			t.Error(err)
		}
		ssd1, err := lvm.ListVolumeByVG("_SSDVGLabel")
		if err != nil {
			t.Error(err)
		}

		if len(hdd1) != len(hdd)+2 {
			t.Errorf("expected %d but got %d", len(hdd)+2, len(hdd1))
		}
		if len(ssd1) != len(ssd)+1 {
			t.Errorf("expected %d but got %d", len(ssd)+1, len(ssd1))
		}
	}

	{
		// test GetVolume
		_, err = lvm.GetVolume("lv00004")
		if err != nil {
			t.Error(err)
		}
	}

	{
		// test DelVolume
		err = lvm.DelVolume("lv00004")
		if err != nil {
			t.Error(err)
		}

		if lvm.len() != base+2 {
			t.Errorf("expected %d but got %d", base+2, lvm.len())
		}
	}
}

func TestLocalVolumeMap(t *testing.T) {
	var lvm localVolumeMap

	testLocalVolumeMap(&lvm, t)
}

func TestLocalVolumeDrivers(t *testing.T) {
	lvm := &localVolumeMap{}

	{
		e1 := &cluster.Engine{
			ID:   "engineID001",
			Name: "engineName001",
			Labels: map[string]string{
				_HDDVGLabel:     "", // nil string
				_HDDVGSizeLabel: "11941040184",
				_SSDVGLabel:     "_SSDVGLabel",
				_SSDVGSizeLabel: "81084012324",
			},
		}

		drivers, err := localVolumeDrivers(e1, lvm)
		if err != nil {
			t.Error(err)
		}
		if want, got := 1, len(drivers); got != want {
			t.Errorf("expected %d but got %d", want, got)
		}

		if d := VolumeDrivers(drivers).get(_HDD); d != nil {
			t.Error("expected nil value")
		}

		if d := VolumeDrivers(drivers).get(_SSD); d == nil {
			t.Error("expected non-nil value")
		} else {
			space, err := d.Space()
			if err != nil {
				t.Error(err)
			}
			if space.Free != 81084012324 {
				t.Errorf("expected %d but got %d", 81084012324, space.Free)
			}
		}
	}

	{
		e2 := &cluster.Engine{
			ID:   "engineID002",
			Name: "engineName002",
			Labels: map[string]string{
				_HDDVGLabel:     "_HDDVGLabel",
				_HDDVGSizeLabel: "1194109840184",
				_SSDVGLabel:     "_SSDVGLabel",
				_SSDVGSizeLabel: "81084012324",
			},
		}

		drivers, err := localVolumeDrivers(e2, lvm)
		if err != nil {
			t.Error(err)
		}
		if want, got := 2, len(drivers); got != want {
			t.Errorf("expected %d but got %d", want, got)
		}

		if d := VolumeDrivers(drivers).get(_HDD); d == nil {
			t.Error("expected non-nil value")
		} else {
			space, err := d.Space()
			if err != nil {
				t.Error(err)
			}
			if space.Free != 1194109840184 {
				t.Errorf("expected %d but got %d", 1194109840184, space.Free)
			}
		}

		if d := VolumeDrivers(drivers).get(_SSD); d == nil {
			t.Error("expected non-nil value")
		} else {
			space, err := d.Space()
			if err != nil {
				t.Error(err)
			}
			if space.Free != 81084012324 {
				t.Errorf("expected %d but got %d", 81084012324, space.Free)
			}
		}

		testLocalVolumeMap(lvm, t) // insert []Volume

		if d := VolumeDrivers(drivers).get(_HDD); d == nil {
			t.Error("expected non-nil value")
		} else {
			space, err := d.Space()
			if err != nil {
				t.Error(err)
			}
			if space.Free != 1194106592743 {
				t.Errorf("expected %d but got %d", 1194106592743, space.Free)
			}
		}

		if d := VolumeDrivers(drivers).get(_SSD); d == nil {
			t.Error("expected non-nil value")
		} else {
			space, err := d.Space()
			if err != nil {
				t.Error(err)
			}
			if space.Free != 81080764983 {
				t.Errorf("expected %d but got %d", 81080764983, space.Free)
			}
		}
	}

	{
		e3 := &cluster.Engine{
			ID:   "engineID003",
			Name: "engineName003",
			Labels: map[string]string{
				_HDDVGLabel:     "_HDDVGLabel",
				_HDDVGSizeLabel: "119410y9840184", // wrong number
				_SSDVGLabel:     "_SSDVGLabel",
				_SSDVGSizeLabel: "81084012324",
			},
		}

		drivers, err := localVolumeDrivers(e3, lvm)
		if err != nil {
			t.Error(err)
		}
		if want, got := 1, len(drivers); got != want {
			t.Errorf("expected %d but got %d", want, got)
		}
	}
}

func TestLocalVolumeAlloc(t *testing.T) {

}
