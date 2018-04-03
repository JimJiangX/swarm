package driver

import (
	"testing"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/structs"
)

var _ localVolumeIface = &localVolumeMap{}

func TestParseSize(t *testing.T) {
	type testSize struct {
		size string
		val  int64
		want bool
	}

	var tests = []testSize{
		{" ", 0, true}, {"1", 1, true},
		{"1 b", 1, true}, {"1B", 1, true},
		{"1 k", 1 << 10, true}, {"1K", 1 << 10, true},
		{"1 m", 1 << 20, true}, {"1M", 1 << 20, true},
		{"1 g", 1 << 30, true}, {"1G", 1 << 30, true},
		{"497491 m", 497491 << 20, true}, {" 2784991248 ", 2784991248, true},
		{"497491 m", 497491 << 20, true}, {"2784991248", 2784991248, true},
		{"497 491 m", 0, false}, {"278499 1248", 0, false},
	}

	for i := range tests {

		n, err := parseSize(tests[i].size)
		if err != nil {
			if tests[i].want {
				t.Error(i, err)
			}
		} else if n != tests[i].val {
			t.Errorf("%d: %s ,expect %d but got %d", i, tests[i].size, tests[i].val, n)

		}
	}
}

func testLocalVolumeMap(lvm *localVolumeMap, t *testing.T) {
	base := lvm.len()
	hdd, err := lvm.ListVolumeByEngine("_HDDVGLabel")
	if err != nil {
		t.Error(err)
	}
	ssd, err := lvm.ListVolumeByEngine("_SSDVGLabel")
	if err != nil {
		t.Error(err)
	}

	{
		// test InsertVolume
		err = lvm.InsertVolume(database.Volume{
			ID:   "lv0001",
			Name: "lv0001",
			VG:   "_HDDVGLabel",
			Size: 2 << 30,
		})
		if err != nil {
			t.Error(err)
		}

		err = lvm.InsertVolume(database.Volume{
			ID:   "lv0002",
			Name: "lv0001", // duplicate name
			VG:   "_HDDVGLabel",
			Size: 3 << 30,
		})
		if err == nil {
			t.Error(err)
		}

		err = lvm.InsertVolume(database.Volume{
			ID:   "lv0003",
			Name: "lv0003",
			VG:   "_SSDVGLabel",
			Size: 4 << 30,
		})
		if err != nil {
			t.Error(err)
		}

		err = lvm.InsertVolume(database.Volume{
			ID:   "lv0004",
			Name: "lv00004",
			VG:   "_HDDVGLabel",
			Size: 5 << 30,
		})
		if err != nil {
			t.Error(err)
		}

		if lvm.len() != base+3 {
			t.Errorf("expected %d but got %d", base+3, lvm.len())
		}

		// test ListVolumeByVG
		hdd1, err := lvm.ListVolumeByEngine("_HDDVGLabel")
		if err != nil {
			t.Error(err)
		}
		ssd1, err := lvm.ListVolumeByEngine("_SSDVGLabel")
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
				_HDDVGSizeLabel: "20G",
				_SSDVGLabel:     _SSDVGLabel,
				_SSDVGSizeLabel: "10G",
			},
		}

		drivers, err := localVolumeDrivers(e1, lvm, 0)
		if err != nil {
			t.Error(err)
		}
		if want, got := 1, len(drivers); got != want {
			t.Errorf("expected %d but got %d", want, got)
		}

		if d := VolumeDrivers(drivers).Get(_HDD); d != nil {
			t.Error("expected nil value")
		}

		if d := VolumeDrivers(drivers).Get(_SSD); d == nil {
			t.Error("expected non-nil value")
		} else {
			space, err := d.Space()
			if err != nil {
				t.Error(err)
			}
			if space.Free != 10<<30 {
				t.Errorf("expected %d but got %d", 10<<30, space.Free)
			}
		}
	}

	{
		e2 := &cluster.Engine{
			ID:   "engineID002",
			Name: "engineName002",
			Labels: map[string]string{
				_HDDVGLabel:     _HDDVGLabel,
				_HDDVGSizeLabel: "200g",
				_SSDVGLabel:     _SSDVGLabel,
				_SSDVGSizeLabel: "50G",
			},
		}

		drivers, err := localVolumeDrivers(e2, lvm, 0)
		if err != nil {
			t.Error(err)
		}
		if want, got := 2, len(drivers); got != want {
			t.Errorf("expected %d but got %d", want, got)
		}

		if d := VolumeDrivers(drivers).Get(_HDD); d == nil {
			t.Error("expected non-nil value")
		} else {
			space, err := d.Space()
			if err != nil {
				t.Error(err)
			}
			if space.Free != 200<<30 {
				t.Errorf("expected %d but got %d", 200<<30, space.Free)
			}
		}

		if d := VolumeDrivers(drivers).Get(_SSD); d == nil {
			t.Error("expected non-nil value")
		} else {
			space, err := d.Space()
			if err != nil {
				t.Error(err)
			}
			if space.Free != 50<<30 {
				t.Errorf("expected %d but got %d", 50<<30, space.Free)
			}
		}

		testLocalVolumeMap(lvm, t) // insert []Volume

		if d := VolumeDrivers(drivers).Get(_HDD); d == nil {
			t.Error("expected non-nil value")
		} else {
			space, err := d.Space()
			if err != nil {
				t.Error(err)
			}
			if space.Free != 214748364800 {
				t.Errorf("expected %d but got %d", 214748364800, space.Free)
			}
		}

		if d := VolumeDrivers(drivers).Get(_SSD); d == nil {
			t.Error("expected non-nil value")
		} else {
			space, err := d.Space()
			if err != nil {
				t.Error(err)
			}
			if space.Free != 53687091200 {
				t.Errorf("expected %d but got %d", 53687091200, space.Free)
			}
		}
	}

	{
		e3 := &cluster.Engine{
			ID:   "engineID003",
			Name: "engineName003",
			Labels: map[string]string{
				_HDDVGLabel:     _HDDVGLabel,
				_HDDVGSizeLabel: "119410y9840184", // wrong number
				_SSDVGLabel:     _SSDVGLabel,
				_SSDVGSizeLabel: "20G",
			},
		}

		drivers, err := localVolumeDrivers(e3, lvm, 0)
		if err != nil {
			t.Error(err)
		}
		if want, got := 1, len(drivers); got != want {
			t.Errorf("expected %d but got %d", want, got)
		}
	}
}

func TestLocalVolumeAlloc(t *testing.T) {
	lvm := &localVolumeMap{}
	testLocalVolumeMap(lvm, t)

	requires := []structs.VolumeRequire{
		{
			Name: "LOG",
			Type: _HDD,
			Size: 5 << 30,
		},
		{
			Name: "DAT1",
			Type: _HDD,
			Size: 5 << 30,
		},
		{
			Name: "DAT2",
			Type: _HDD,
			Size: 5 << 30,
		},

		{
			Name: "DAT",
			Type: _SSD,
			Size: 3 << 30,
		},
	}

	e := &cluster.Engine{
		ID:   "engineID002",
		Name: "engineName002",
		Labels: map[string]string{
			_HDDVGLabel:     _HDDVGLabel,
			_HDDVGSizeLabel: "50G",
			_SSDVGLabel:     _SSDVGLabel,
			_SSDVGSizeLabel: "10G",
		},
	}

	drivers, err := localVolumeDrivers(e, lvm, 0)
	if err != nil {
		t.Error(err)
	}

	vds := VolumeDrivers(drivers)

	{
		config := &cluster.ContainerConfig{}
		lvs, err := vds.AllocVolumes(config, "unitxxxxx0001", requires)
		if err != nil {
			t.Error(err)
		} else if len(lvs) != len(requires) {
			t.Errorf("expected %d but got %d", len(requires), len(lvs))
		}
	}

	{
		config := &cluster.ContainerConfig{}
		lvs, err := vds.AllocVolumes(config, "unit2xxxxx0002", requires)
		if err != nil {
			t.Error(err)
		} else if len(lvs) != len(requires) {
			t.Errorf("expected %d but got %d", len(requires), len(lvs))
		}
	}

	{
		config := &cluster.ContainerConfig{}
		lvs, err := vds.AllocVolumes(config, "unit3xxxxx0002", requires)
		if err != nil {
			t.Error(err)
		} else if len(lvs) != len(requires) {
			t.Errorf("expected %d but got %d", len(requires), len(lvs))
		}
	}

	{
		// not enough _SSD
		config := &cluster.ContainerConfig{}
		_, err := vds.AllocVolumes(config, "unit3xxxxx0002", requires)
		if err == nil {
			t.Error("error expected")
		}
	}

	{
		config := &cluster.ContainerConfig{}
		lvs, err := vds.AllocVolumes(config, "unit4xxxxx0002", requires[:1])
		if err != nil {
			t.Error(err)
		} else if len(lvs) != 1 {
			t.Errorf("expected %d but got %d", len(requires), 1)
		}
	}
}
