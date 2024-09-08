package main

import (
	"fmt"
	"log/syslog"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/docker/go-plugins-helpers/volume"
)

type btrfsDriver struct {
	home    string
	volumes map[string]*vol
	count   map[string]int
	mu      sync.RWMutex
	logger  *syslog.Writer
}

type vol struct {
	Name       string `json:"name"`
	MountPoint string `json:"mountpoint"`
	Type       string `json:"type"`
	Source     string `json:"source"`
}

func getVolumeCreationDateTime(volume string) (time.Time, error) {
    // Implement or use the actual function logic
    return time.Now(), nil
}

func (d *btrfsDriver) Capabilities() *volume.CapabilitiesResponse {
	return &volume.CapabilitiesResponse{
        Capabilities: volume.Capability{
            Scope: "local",  
        },
    }
}

func newDriver(home string) (*btrfsDriver, error) {
	logger, err := syslog.New(syslog.LOG_ERR, "docker-btrfs-plugin")
	if err != nil {
		return nil, err
	}

	return &btrfsDriver{
		home:    home,
		volumes: make(map[string]*vol),
		count:   make(map[string]int),
		logger:  logger,
	}, nil
}

func (d *btrfsDriver) Create(req *volume.CreateRequest) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.volumes[req.Name]; exists {
		return nil
	}

	mp := getMountpoint(d.home, req.Name)

	// Check if snapshot option is provided
	snap, ok := req.Options["snapshot"]
	isSnapshot := ok && snap != ""

	// Ensure the parent directory exists
	err := os.MkdirAll(d.home, 0700)
	if err != nil {
		return err
	}

	if isSnapshot {
		// Create a BTRFS snapshot
		snapshotSource := getMountpoint(d.home, snap)
		cmd := exec.Command("btrfs", "subvolume", "snapshot", snapshotSource, mp)
		if out, err := cmd.CombinedOutput(); err != nil {
			d.logger.Err(fmt.Sprintf("Create: btrfs snapshot error: %s output %s", err, string(out)))
			return fmt.Errorf("Error creating snapshot volume")
		}
	} else {
		// Create a new BTRFS subvolume
		cmd := exec.Command("btrfs", "subvolume", "create", mp)
		if out, err := cmd.CombinedOutput(); err != nil {
			d.logger.Err(fmt.Sprintf("Create: btrfs subvolume create error: %s output %s", err, string(out)))
			return fmt.Errorf("Error creating volume")
		}
	}

	// Save the volume info
	v := &vol{Name: req.Name, MountPoint: mp}
	if isSnapshot {
		v.Type = "Snapshot"
		v.Source = snap
	}
	d.volumes[v.Name] = v
	d.count[v.Name] = 0

	err = saveToDisk(d.volumes, d.count)
	if err != nil {
		return err
	}
	return nil
}


func (d *btrfsDriver) List() (*volume.ListResponse, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var ls []*volume.Volume
	for _, vol := range d.volumes {
		v := &volume.Volume{
			Name:       vol.Name,
			Mountpoint: vol.MountPoint,
		}
		ls = append(ls, v)
	}
	return &volume.ListResponse{Volumes: ls}, nil
}

func (d *btrfsDriver) Get(req *volume.GetRequest) (*volume.GetResponse, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	v, exists := d.volumes[req.Name]
	if !exists {
		return &volume.GetResponse{}, fmt.Errorf("No such volume")
	}

	createdAt, err := getVolumeCreationDateTime(v.MountPoint)
	if err != nil {
		d.logger.Err(fmt.Sprintf("Get: %v", err))
		return nil, err
	}

	var res volume.GetResponse
	res.Volume = &volume.Volume{
		Name:       v.Name,
		Mountpoint: v.MountPoint,
		CreatedAt:  fmt.Sprintf(createdAt.Format(time.RFC3339)),
	}
	return &res, nil
}

func (d *btrfsDriver) Remove(req *volume.RemoveRequest) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	vol, exists := d.volumes[req.Name]
	if !exists {
		return fmt.Errorf("Unknown volume %s", req.Name)
	}

	// Check if the volume is mounted by any container
	if d.count[req.Name] > 0 {
		return fmt.Errorf("Error removing volume: %s is still mounted by a container", req.Name)
	}

	// Check if the volume is a snapshot
	if vol.Type == "Snapshot" {
		snapshotPath := getMountpoint(d.home, req.Name)
		if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
			// Log the missing snapshot and clean up metadata
			d.logger.Info(fmt.Sprintf("Snapshot %s does not exist, cleaning up metadata", req.Name))
			delete(d.volumes, req.Name)
			delete(d.count, req.Name)
			if err := saveToDisk(d.volumes, d.count); err != nil {
				return fmt.Errorf("Error saving metadata for volume %s: %v", req.Name, err)
			}
			return nil // Successfully handled missing snapshot, exit here
		}

		// Try to remove the BTRFS snapshot
		cmd := exec.Command("btrfs", "subvolume", "delete", snapshotPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			d.logger.Err(fmt.Sprintf("Error removing BTRFS snapshot %s: %s", req.Name, string(out)))
			return fmt.Errorf("Error removing BTRFS snapshot %s: %v", req.Name, err)
		}

		// Clean up metadata after successfully removing the snapshot
		delete(d.count, req.Name)
		delete(d.volumes, req.Name)
		if err := saveToDisk(d.volumes, d.count); err != nil {
			return fmt.Errorf("Error saving metadata for volume %s: %v", req.Name, err)
		}

		return nil // Exit after handling the snapshot deletion
	}

	// Check if the volume is an origin subvolume with snapshots
	isOrigin := func() bool {
		for _, v := range d.volumes {
			if v.Name == req.Name {
				continue
			}
			if v.Type == "Snapshot" && v.Source == req.Name {
				return true
			}
		}
		return false
	}()

	if isOrigin {
		return fmt.Errorf("Error removing volume %s: snapshots must be removed first", req.Name)
	}

	// Try to remove the original subvolume (non-snapshot)
	cmd := exec.Command("btrfs", "subvolume", "delete", vol.MountPoint)
	if out, err := cmd.CombinedOutput(); err != nil {
		d.logger.Err(fmt.Sprintf("Error removing BTRFS subvolume %s: %s", req.Name, string(out)))
		return fmt.Errorf("Error removing BTRFS subvolume %s: %v", req.Name, err)
	}

	// Clean up metadata after successfully removing the subvolume
	delete(d.count, req.Name)
	delete(d.volumes, req.Name)
	if err := saveToDisk(d.volumes, d.count); err != nil {
		return fmt.Errorf("Error saving metadata for volume %s: %v", req.Name, err)
	}

	return nil
}

func (d *btrfsDriver) Path(req *volume.PathRequest) (*volume.PathResponse, error) {
	return &volume.PathResponse{Mountpoint: getMountpoint(d.home, req.Name)}, nil
}

func (l *btrfsDriver) Mount(req *volume.MountRequest) (*volume.MountResponse, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Check if the volume exists
	if _, exists := l.volumes[req.Name]; !exists {
		return &volume.MountResponse{}, fmt.Errorf("Unknown volume %s", req.Name)
	}

	// Initialize the mount count if it doesn't exist
	if _, ok := l.count[req.Name]; !ok {
		l.count[req.Name] = 0
	}

	// Increment the mount count
	l.count[req.Name]++

	// Save the updated mount count to disk
	if err := saveToDisk(l.volumes, l.count); err != nil {
		return &volume.MountResponse{}, err
	}

	// Return the mountpoint for the volume
	return &volume.MountResponse{Mountpoint: getMountpoint(l.home, req.Name)}, nil
}

func (l *btrfsDriver) Unmount(req *volume.UnmountRequest) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Decrement the mount count
	if _, ok := l.count[req.Name]; !ok {
		return fmt.Errorf("Unknown volume %s", req.Name)
	}

	l.count[req.Name]--
	if l.count[req.Name] < 0 {
		l.count[req.Name] = 0
	}

	if err := saveToDisk(l.volumes, l.count); err != nil {
		return err
	}

	return nil
}
