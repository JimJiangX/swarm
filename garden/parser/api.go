package parser

const (
	InitServiceCmd  = "init_service_cmd"
	StartServiceCmd = "start_service_cmd"
	StopServiceCmd  = "stop_service_cmd"
	HealthCheckCmd  = "health_check_cmd"
)

type ConfigCmds struct {
	Update  bool
	ID      string
	Path    string
	Context string

	Cmds CmdsMap

	HealthCheck struct{}
}

type CmdsMap map[string][]string

type Commands map[string]CmdsMap

type ConfigsMap map[string]ConfigCmds

func (c CmdsMap) Get(typ string) []string {
	if c == nil {
		return nil
	}

	return c[typ]
}

func (c ConfigCmds) GetCmd(typ string) []string {
	if c.Cmds == nil {
		return nil
	}

	return c.Cmds.Get(typ)
}

func (c Commands) GetCmd(key, typ string) []string {
	if cmds, ok := c[key]; ok {
		return cmds.Get(typ)
	}

	return nil
}

func (m ConfigsMap) Get(key string) (ConfigCmds, bool) {
	if m == nil {
		return ConfigCmds{}, false
	}

	val, ok := m[key]

	return val, ok
}

func (m ConfigsMap) GetCmd(key, typ string) []string {
	val, ok := m.Get(key)
	if ok {
		return val.GetCmd(typ)
	}

	return nil
}
