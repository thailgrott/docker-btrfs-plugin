package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path"
	"fmt"
)

func removeBtrfsSubvolume(subvolPath string) error {
	cmd := exec.Command("btrfs", "subvolume", "delete", subvolPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("Error removing BTRFS subvolume: %v, output: %s", err, out)
	}
	return nil
}

func getMountpoint(home, name string) string {
	return path.Join(home, name)
}

func saveToDisk(volumes map[string]*vol, count map[string]int) error {
	// Save volume store metadata.
	fhVolumes, err := os.Create(btrfsVolumesConfigPath)
	if err != nil {
		return err
	}
	defer fhVolumes.Close()

	if err := json.NewEncoder(fhVolumes).Encode(&volumes); err != nil {
		return err
	}

	// Save count store metadata.
	fhCount, err := os.Create(btrfsCountConfigPath)
	if err != nil {
		return err
	}
	defer fhCount.Close()

	return json.NewEncoder(fhCount).Encode(&count)
}

func loadFromDisk(b *btrfsDriver) error {
	// Load volume store metadata
	jsonVolumes, err := os.Open(btrfsVolumesConfigPath)
	if err != nil {
		return err
	}
	defer jsonVolumes.Close()

	if err := json.NewDecoder(jsonVolumes).Decode(&b.volumes); err != nil {
		return err
	}

	// Load count store metadata
	jsonCount, err := os.Open(btrfsCountConfigPath)
	if err != nil {
		return err
	}
	defer jsonCount.Close()

	return json.NewDecoder(jsonCount).Decode(&b.count)
}
