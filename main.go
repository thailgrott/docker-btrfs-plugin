package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/docker/go-plugins-helpers/volume"
)

const (
	btrfsHome              = "/var/lib/docker-btrfs-plugin"
	btrfsVolumesConfigPath = "/var/lib/docker-btrfs-plugin/btrfsVolumesConfig.json"
	btrfsCountConfigPath   = "/var/lib/docker-btrfs-plugin/btrfsCountConfig.json"
)

var (
	flVersion *bool
	flDebug   *bool
)

func init() {
	flVersion = flag.Bool("version", false, "Print version information and quit")
	flDebug = flag.Bool("debug", false, "Enable debug logging")
}

func main() {
	flag.Parse()

	if *flVersion {
		fmt.Fprint(os.Stdout, "docker btrfs plugin version: 0.0.2a\n")
		return
	}

	if *flDebug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	if _, err := os.Stat(btrfsHome); err != nil {
		if !os.IsNotExist(err) {
			logrus.Fatal(err)
		}
		logrus.Debugf("Created home dir at %s", btrfsHome)
		if err := os.MkdirAll(btrfsHome, 0700); err != nil {
			logrus.Fatal(err)
		}
	}

	btrfsDriver, err := newDriver(btrfsHome)
	if err != nil {
		logrus.Fatalf("Error initializing btrfsDriver %v", err)
	}

	// Call loadFromDisk only if config file exists.
	if _, err := os.Stat(btrfsVolumesConfigPath); err == nil {
		if err := loadFromDisk(btrfsDriver); err != nil {
			logrus.Fatal(err)
		}
	}

	h := volume.NewHandler(btrfsDriver)
	if err := h.ServeUnix("btrfs", 0); err != nil {
		logrus.Fatal(err)
	}
}
