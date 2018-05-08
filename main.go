package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/leodotcloud/log"
	logserver "github.com/leodotcloud/log/server"
	"github.com/rancher/connectivity-check/checker"
	"github.com/rancher/connectivity-check/utils"
	"github.com/rancher/go-rancher-metadata/metadata"
	"github.com/urfave/cli"
)

// VERSION of the application, that can be defined during build time
var VERSION = "v0.0.0-dev"

const (
	appName             = "connectivity-check"
	metadataURLTemplate = "http://%v/2016-07-29"
)

func main() {
	app := cli.NewApp()
	app.Name = appName
	app.Version = VERSION
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "metadata-address",
			Value:  "169.254.169.250",
			EnvVar: "RANCHER_METADATA_ADDRESS",
		},
		cli.IntFlag{
			Name:   "connectivity-check-interval",
			Usage:  fmt.Sprintf("Customize the connectivity check interval in milliseconds (default: %v)", checker.DefaultCheckInterval),
			Value:  checker.DefaultCheckInterval,
			EnvVar: "CONNECTIVITY_CHECK_INTERVAL",
		},
		cli.IntFlag{
			Name:   "peer-connection-timeout",
			Usage:  fmt.Sprintf("Customize the peer connnection timeout in milliseconds (default: %v)", checker.DefaultPeerConnectionTimeoutInterval),
			Value:  checker.DefaultPeerConnectionTimeoutInterval,
			EnvVar: "PEER_CONNECTION_TIMEOUT",
		},
		cli.IntFlag{
			Name:  "port",
			Value: checker.DefaultServerPort,
		},
		cli.BoolFlag{
			Name:   "debug",
			Usage:  "Turn on debug logging",
			EnvVar: "RANCHER_DEBUG",
		},
	}
	app.Action = run
	app.Run(os.Args)
}

func run(c *cli.Context) error {
	logserver.StartServerWithDefaults()
	if c.Bool("debug") {
		log.SetLevelString("debug")
		log.Debugf("loglevel set to debug")
	}

	inputPort := c.Int("port")
	portToUse := checker.DefaultServerPort
	if utils.IsValidPort(inputPort) {
		portToUse = inputPort
	}

	metadataURL := fmt.Sprintf(metadataURLTemplate, c.String("metadata-address"))
	log.Infof("Waiting for metadata")
	mc, err := metadata.NewClientAndWait(metadataURL)
	if err != nil {
		log.Errorf("error creating metadata client: %v", err)
		return err
	}
	log.Infof("Successfully connected to metadata")

	cc, err := checker.New(portToUse, c.Int("connectivity-check-interval"), mc)
	if err != nil {
		log.Errorf("Error creating new checker: %v", err)
		return err
	}

	if err := cc.Start(); err != nil {
		log.Errorf("Failed to start: %v", err)
	}

	sCh := make(chan os.Signal, 2)
	signal.Notify(sCh, os.Interrupt, syscall.SIGTERM)
	<-sCh
	log.Infof("Got shutdown signal")

	if err := cc.Shutdown(); err != nil {
		log.Errorf("error shutting down: %v", err)
		return err
	}

	log.Infof("Program exiting successfully")
	return nil
}
