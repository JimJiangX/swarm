package cli

import (
	"os"
	"path"
	"runtime/debug"
	"runtime/trace"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/swarm/experimental"
	"github.com/docker/swarm/version"
	"github.com/urfave/cli"
)

// Run the Swarm CLI.
func Run() {
	setProgramLimits()

	app := cli.NewApp()
	app.Name = path.Base(os.Args[0])
	app.Usage = "A Docker-native clustering system"
	app.Version = version.VERSION + " (" + version.GITCOMMIT + ")"

	app.Author = ""
	app.Email = ""

	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:   "debug",
			Usage:  "debug mode",
			EnvVar: "DEBUG",
		},

		cli.StringFlag{
			Name:  "log-level, l",
			Value: "info",
			Usage: "Log level (options: debug, info, warn, error, fatal, panic)",
		},

		cli.BoolFlag{
			Name:  "experimental",
			Usage: "enable experimental features",
		},
		cli.BoolFlag{
			Name:  "trace",
			Usage: "go tool trace",
		},
	}

	exist := make(chan struct{})
	defer close(exist)

	// logs
	app.Before = func(c *cli.Context) error {
		log.SetOutput(os.Stdout)
		level, err := log.ParseLevel(c.String("log-level"))
		if err != nil {
			log.Fatal(err.Error())
		}
		log.SetLevel(level)

		// If a log level wasn't specified and we are running in debug mode,
		// enforce log-level=debug.
		if !c.IsSet("log-level") && !c.IsSet("l") && c.Bool("debug") {
			log.SetLevel(log.DebugLevel)
		}

		experimental.ENABLED = c.Bool("experimental")

		if c.Bool("trace") {
			f, err := os.Create("trace.out")
			if err != nil {
				log.Fatal(err)
			}
			defer f.Close()

			err = trace.Start(f)
			if err != nil {
				log.Fatal(err)
			}

			go func() {
				<-exist
				trace.Stop()
			}()
		}

		return nil
	}

	app.Commands = commands

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func setProgramLimits() {
	// Swarm runnable threads could be large when the number of nodes is large
	// or under request bursts. Most threads are occupied by network connections.
	// Increase max thread count from 10k default to 50k to accommodate it.
	const maxThreadCount int = 50 * 1000
	debug.SetMaxThreads(maxThreadCount)
}
