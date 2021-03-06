package vars

import (
	"errors"
)

var errUserInvalid = errors.New("invalid user")

type User struct {
	Role     string
	User     string
	Password string
	//	Privilege string
}

var (
	root     = ""
	root_pwd = ""
	// root_name=cup_dba
	// root_password=123.com
	// root_privilege="ALL"
	Root = User{
		Role: "root",
		//	Privilege: "ALL",
	}

	mon     = ""
	mon_pwd = ""
	// mon_name=mon
	// mon_password=111111
	// mon_privilege="SELECT,PROCESS,REPLICATION CLENT"
	Monitor = User{
		Role: "monitor",
		//	Privilege: "SELECT,PROCESS,REPLICATION CLENT",
	}

	repl     = ""
	repl_pwd = ""
	// repl_name=repl
	// repl_password=111111
	// repl_privilege="REPLICATION SLAVE"
	Replication = User{
		Role: "replication",
		//	Privilege: "REPLICATION SLAVE",
	}

	check     = ""
	check_pwd = ""
	Check     = User{
		Role: "check",
		//	Privilege: "ALL",
	}
)

func init() {
	Root.User = root
	Root.Password = root_pwd

	Monitor.User = mon
	Monitor.Password = mon_pwd

	Replication.User = repl
	Replication.Password = repl_pwd

	Check.User = check
	Check.Password = check_pwd
}

func Validate() error {
	if Root.User == "" || Root.Password == "" ||
		Check.User == "" || Check.Password == "" ||
		Monitor.User == "" || Monitor.Password == "" ||
		Replication.User == "" || Replication.Password == "" {

		return errUserInvalid
	}

	return nil
}
