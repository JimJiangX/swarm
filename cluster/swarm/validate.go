package swarm

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/store"
)

func validateContainerConfig(config *cluster.ContainerConfig) error {
	// validate config
	msg := make([]string, 0, 5)
	swarmID := config.SwarmID()
	if swarmID != "" {
		msg = append(msg, "Swarm ID to the container have created")
	}

	if config.HostConfig.CPUShares != 0 {
		msg = append(msg, "CPUShares > 0,CPUShares should be 0")
	}

	if config.HostConfig.CpusetCpus == "" {
		msg = append(msg, "CpusetCpus is null,CpusetCpus should not be null")
	}

	_, err := parseCpuset(config.HostConfig.CpusetCpus)
	if err != nil {
		msg = append(msg, err.Error())
	}

	if err := config.Validate(); err != nil {
		msg = append(msg, err.Error())
	}

	if len(msg) == 0 {
		return nil
	}

	return fmt.Errorf("Errors:%s", msg)
}

func validateContainerUpdateConfig(config container.UpdateConfig) error {
	msg := make([]string, 0, 5)
	if config.Resources.CPUShares != 0 {
		msg = append(msg, "CPUShares > 0,CPUShares should be 0")
	}

	if config.Resources.CpusetCpus != "" {
		n, err := strconv.Atoi(config.Resources.CpusetCpus)
		if err != nil {
			msg = append(msg, err.Error())
		} else if n == 0 {
			msg = append(msg, fmt.Sprintf("CpusetCpus is '%s',should >0", config.Resources.CpusetCpus))
		}
	}

	if len(msg) == 0 {
		return nil
	}

	return fmt.Errorf("Errors:%s", msg)
}

func ValidDatacenter(req structs.PostClusterRequest) string {
	warnings := make([]string, 0, 5)
	if req.Name == "" {
		warnings = append(warnings, "'name' is null")
	}

	if !isStringExist(req.StorageType, supportedStoreTypes) {
		warnings = append(warnings, fmt.Sprintf("Unsupported '%s' Yet", req.StorageType))
	}

	if !store.IsLocalStore(req.StorageType) && req.StorageID == "" {
		warnings = append(warnings, "missing 'StorageID' while 'StorageType' isnot 'local'")
	}

	if len(warnings) == 0 {
		return ""
	}

	return strings.Join(warnings, ",")
}

func ValidService(req structs.PostServiceRequest) []string {
	warnings := make([]string, 0, 10)
	if req.Name == "" {
		warnings = append(warnings, "Service Name should not be null")
	}

	_, err := database.GetService(req.Name)
	if err == nil {
		warnings = append(warnings, fmt.Sprintf("Service Name %s exist", req.Name))
	}

	arch, _, err := getServiceArch(req.Architecture)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("Parse 'Architecture' Failed,%s", err))
	}

	for _, module := range req.Modules {
		if _, _, err := initialize(module.Type); err != nil {
			warnings = append(warnings, err.Error())
		}

		//if !isStringExist(module.Type, supportedServiceTypes) {
		//	warnings = append(warnings, fmt.Sprintf("Unsupported '%s' Yet", module.Type))
		//}
		if module.Config.Image == "" {
			image, err := database.QueryImage(module.Name, module.Version)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("Not Found Image:%s:%s,Error%s", module.Name, module.Version, err))
			}
			if !image.Enabled {
				warnings = append(warnings, fmt.Sprintf("Image: %s:%s is Disabled", module.Name, module.Version))
			}
		} else {
			image, err := database.QueryImageByID(module.Config.Image)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("Not Found Image:%s,Error%s", module.Config.Image, err))
			}
			if !image.Enabled {
				warnings = append(warnings, fmt.Sprintf("Image:%s is Disabled", module.Config.Image))
			}
		}
		_, num, err := getServiceArch(module.Arch)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s,%s", module.Arch, err))
		}

		if arch[module.Type] != num {
			warnings = append(warnings, fmt.Sprintf("%s nodeNum  unequal Architecture,(%s)", module.Type, module.Arch))
		}

		config := cluster.BuildContainerConfig(module.Config, module.HostConfig, module.NetworkingConfig)
		err = validateContainerConfig(config)
		if err != nil {
			warnings = append(warnings, err.Error())
		}

		lvNames := make([]string, 0, len(module.Stores))
		for _, ds := range module.Stores {
			if isStringExist(ds.Name, lvNames) {
				warnings = append(warnings, fmt.Sprintf("Storage Name '%s' Duplicate in one Module:%s", ds.Name, module.Name))
			} else {
				lvNames = append(lvNames, ds.Name)
			}

			if !isStringExist(ds.Name, supportedStoreNames) {
				warnings = append(warnings, fmt.Sprintf("Unsupported Storage Name '%s' Yet,should be one of %s", ds.Name, supportedStoreNames))
			}

			if !isStringExist(ds.Type, supportedStoreTypes) {
				warnings = append(warnings, fmt.Sprintf("Unsupported Storage Type '%s' Yet,should be one of %s", ds.Type, supportedStoreTypes))
			}
		}
	}

	if len(warnings) == 0 {
		return nil
	}

	logrus.Warnf("Service Valid warning:%s", warnings)

	return warnings
}

func ValidateServiceScale(svc *Service, scale structs.PostServiceScaledRequest) error {
	warns := make([]string, 0, 10)

	_, _, err := initialize(scale.Type)
	if err != nil {
		warns = append(warns, err.Error())
	}

	if scale.UpdateConfig != nil {
		err := validateContainerUpdateConfig(*scale.UpdateConfig)
		if err != nil {
			warns = append(warns, err.Error())
		}
	}

	svc.RLock()

	units := svc.getUnitByType(scale.Type)
	if len(units) == 0 {
		warns = append(warns, fmt.Sprintf("Not Found unit '%s' In Service %s", scale.Type, svc.Name))
	}
	for _, u := range units {
		if u.engine == nil || (u.config == nil && u.container == nil) {
			warns = append(warns, fmt.Sprintf("unit odd,%+v", u))
		}
	}

	des, err := svc.getServiceDescription()

	svc.RUnlock()

	if err != nil {
		warns = append(warns, err.Error())
	}
	if err != nil || des == nil {
		return fmt.Errorf("Warnings:%s", warns)
	}

	m, found := 0, false
	for index := range des.Modules {
		if scale.Type == des.Modules[index].Type {
			m, found = index, true
			break
		}
	}
	if !found {
		warns = append(warns, fmt.Sprintf("Not Found '%s' service", scale.Type))
		return fmt.Errorf("Warnings:%s", warns)
	}

	for ext := range scale.Extensions {
		if scale.Extensions[ext].Type == "nfs" || scale.Extensions[ext].Type == "NFS" {
			warns = append(warns, "Found Type 'NFS',Unsupported 'NFS' Expension")
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
			warns = append(warns, fmt.Sprintf("Not Found '%s':'%s' storage",
				scale.Extensions[ext].Name, scale.Extensions[ext].Type))
			continue
		}
	}
	if len(warns) == 0 {
		return nil
	}

	return fmt.Errorf("Warnings:%s", warns)
}

func ValidateIPAddress(prefix int, addrs ...string) error {
	warns := make([]string, 0, len(addrs))

	if prefix < 1 || prefix > 31 {
		warns = append(warns, fmt.Sprintf("'%d' is out of range 1~32", prefix))
	}

	for i := range addrs {
		ip := net.ParseIP(addrs[i])
		if ip == nil {
			warns = append(warns, fmt.Sprintf("'%s' isnot an IP", addrs[i]))
		}
	}

	if len(warns) > 0 {
		return fmt.Errorf("errors:%s", warns)
	}

	return nil
}
