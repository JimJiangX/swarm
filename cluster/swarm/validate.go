package swarm

import (
	"bytes"
	"fmt"
	"net"
	"strconv"

	"github.com/docker/engine-api/types/container"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/storage"
	"github.com/pkg/errors"
)

func validateContainerConfig(config *cluster.ContainerConfig) error {
	// validate config
	buf := bytes.NewBuffer(nil)

	swarmID := config.SwarmID()
	if swarmID != "" {
		buf.WriteString("SwarmID to the container have created\n")
	}

	if config.HostConfig.CPUShares != 0 {
		buf.WriteString("CPUShares > 0,CPUShares should be 0\n")
	}

	if config.HostConfig.CpusetCpus == "" {
		buf.WriteString("CpusetCpus is null,CpusetCpus should not be null\n")
	}

	_, err := parseCpuset(config.HostConfig.CpusetCpus)
	if err != nil {
		buf.WriteString(err.Error())
		buf.WriteByte('\n')
	}

	if err := config.Validate(); err != nil {
		buf.WriteString(err.Error())
		buf.WriteByte('\n')
	}

	if buf.Len() == 0 {
		return nil
	}

	return errors.New(buf.String())
}

func validateContainerUpdateConfig(config container.UpdateConfig) error {
	buf := bytes.NewBuffer(nil)

	if config.Resources.CPUShares != 0 {
		buf.WriteString("CPUShares > 0,CPUShares should be 0\n")
	}

	if config.Resources.CpusetCpus != "" {
		n, err := strconv.Atoi(config.Resources.CpusetCpus)
		if err != nil {
			buf.WriteString(err.Error())
			buf.WriteByte('\n')
		} else if n == 0 {
			buf.WriteString(fmt.Sprintf("CpusetCpus is '%s',should > 0\n", config.Resources.CpusetCpus))
		}
	}

	if buf.Len() == 0 {
		return nil
	}

	return errors.New(buf.String())
}

func ValidDatacenter(req structs.PostClusterRequest) error {
	buf := bytes.NewBuffer(nil)

	if req.Name == "" {
		buf.WriteString("'name' is null\n")
	}

	if !isStringExist(req.StorageType, supportedStoreTypes) {
		buf.WriteString("unsupported '" + req.StorageType + "' yet\n")
	}

	if !storage.IsLocalStore(req.StorageType) && req.StorageID == "" {
		buf.WriteString("missing 'StorageID' while 'StorageType' != 'local'\n")
	}

	if buf.Len() == 0 {
		return nil
	}

	return errors.New(buf.String())
}

func ValidService(req structs.PostServiceRequest) error {
	buf := bytes.NewBuffer(nil)

	if req.Name == "" {
		buf.WriteString("Service name should not be null\n")
	}

	_, err := database.GetService(req.Name)
	if err == nil {
		buf.WriteString("Service name '" + req.Name + "' exist\n")
	}

	arch, _, err := parseServiceArch(req.Architecture)
	if err != nil {
		buf.WriteString(err.Error())
		buf.WriteByte('\n')
	}

	for _, module := range req.Modules {
		if _, _, err := initialize(module.Name, module.Version); err != nil {
			buf.WriteString(err.Error())
			buf.WriteByte('\n')
		}

		//if !isStringExist(module.Type, supportedServiceTypes) {
		//	buf.WriteString("Unsupported " + module.Type)
		//}
		if module.Config.Image == "" {
			image, err := database.GetImage(module.Name, module.Version)
			if err != nil {
				buf.WriteString("not found Image:" + err.Error() + "\n")
			}
			if !image.Enabled {
				buf.WriteString(fmt.Sprintf("Image '%s:%s' disabled\n", module.Name, module.Version))
			}
		} else {
			image, err := database.GetImageByID(module.Config.Image)
			if err != nil {
				buf.WriteString("not found Image:" + err.Error() + "\n")
			}
			if !image.Enabled {
				buf.WriteString("Image:'" + module.Config.Image + "' disabled\n")
			}
		}
		_, num, err := parseServiceArch(module.Arch)
		if err != nil {
			buf.WriteString(err.Error())
			buf.WriteByte('\n')
		}

		if arch[module.Type] != num {
			buf.WriteString(fmt.Sprintf("%s nodeNum  unequal Architecture,(%s)\n", module.Type, module.Arch))
		}

		hostConfig := container.HostConfig{
			Resources: container.Resources{
				Memory:     module.HostConfig.Memory,
				CpusetCpus: module.HostConfig.CpusetCpus,
			},
		}

		config := cluster.BuildContainerConfig(module.Config, hostConfig, module.NetworkingConfig)
		err = validateContainerConfig(config)
		if err != nil {
			buf.WriteString(err.Error())
			buf.WriteByte('\n')
		}

		lvNames := make([]string, 0, len(module.Stores))
		for _, ds := range module.Stores {
			if isStringExist(ds.Name, lvNames) {
				buf.WriteString(fmt.Sprintf("Storage Name '%s' duplicate in one module:'%s'\n", ds.Name, module.Name))
			} else {
				lvNames = append(lvNames, ds.Name)
			}

			if !isStringExist(ds.Name, supportedStoreNames) {
				buf.WriteString(fmt.Sprintf("unsupported Storage Name '%s' yet,should be one of %s\n", ds.Name, supportedStoreNames))
			}

			if !isStringExist(ds.Type, supportedStoreTypes) {
				buf.WriteString(fmt.Sprintf("unsupported Storage Type '%s' yet,should be one of %s\n", ds.Type, supportedStoreTypes))
			}
		}
	}

	if buf.Len() == 0 {
		return nil
	}

	return errors.New(buf.String())
}

func validateServiceScale(svc *Service, scale structs.PostServiceScaledRequest) error {
	buf := bytes.NewBuffer(nil)

	if scale.UpdateConfig != nil {
		err := validateContainerUpdateConfig(*scale.UpdateConfig)
		if err != nil {
			buf.WriteString(err.Error())
			buf.WriteByte('\n')
		}
	}

	svc.RLock()

	units, err := svc.getUnitByType(scale.Type)
	if err != nil {
		buf.WriteString(err.Error())
		buf.WriteByte('\n')
	}
	for _, u := range units {
		if u.engine == nil || (u.config == nil && u.container == nil) {
			buf.WriteString(fmt.Sprintf("unit odd,%+v\n", u))
		}
	}

	des, err := svc.getServiceDescription()
	if err != nil {
		buf.WriteString(err.Error())
		buf.WriteByte('\n')
	}

	svc.RUnlock()

	m, found := 0, false
	for index := range des.Modules {
		if scale.Type == des.Modules[index].Type {
			m, found = index, true
			break
		}
	}
	if !found {
		buf.WriteString(fmt.Sprintf("not found Service by Type'%s'\n", scale.Type))
		return errors.New(buf.String())
	}

	for ext := range scale.Extensions {
		if scale.Extensions[ext].Type == "nfs" || scale.Extensions[ext].Type == "NFS" {
			buf.WriteString("found Type 'NFS',unsupported 'NFS' expension\n")
			continue
		}
		found = false
		for ds := range des.Modules[m].Stores {
			if scale.Extensions[ext].Name == des.Modules[m].Stores[ds].Name {
				// Completion Store Type
				scale.Extensions[ext].Type = des.Modules[m].Stores[ds].Type
				des.Modules[m].Stores[ds].Size += scale.Extensions[ext].Size
				found = true
				break
			}
		}
		if !found {
			buf.WriteString(fmt.Sprintf("not found '%s':'%s' storage\n",
				scale.Extensions[ext].Name, scale.Extensions[ext].Type))
			continue
		}
	}

	if buf.Len() == 0 {
		return nil
	}

	return errors.New(buf.String())
}

func ValidateIPAddress(prefix int, addrs ...string) error {
	buf := bytes.NewBuffer(nil)

	if prefix < 1 || prefix > 31 {
		buf.WriteString(fmt.Sprintf("'%d' is out of range 1~32\n", prefix))
	}

	for i := range addrs {
		ip := net.ParseIP(addrs[i])
		if ip == nil {
			buf.WriteString(addrs[i] + " isnot an IP\n")
		}
	}

	if buf.Len() == 0 {
		return nil
	}

	return errors.New(buf.String())
}
