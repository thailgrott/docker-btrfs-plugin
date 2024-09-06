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

	isOrigin := func() bool {
		for _, vol := range d.volumes {
			if vol.Name == req.Name {
				continue
			}
			if vol.Type == "Snapshot" && vol.Source == req.Name {
				return true
			}
		}
		return false
	}()

	if isOrigin {
		return fmt.Errorf("Error removing volume, all snapshot destinations must be removed before removing the original volume")
	}

	if err := os.RemoveAll(getMountpoint(d.home, req.Name)); err != nil {
		return err
	}

	cmd := exec.Command("btrfs", "subvolume", "delete", vol.MountPoint)
	if out, err := cmd.CombinedOutput(); err != nil {
		d.logger.Err(fmt.Sprintf("Remove: btrfs subvolume delete error %s output %s", err, string(out)))
		return fmt.Errorf("Error removing volume")
	}

	delete(d.count, req.Name)
	delete(d.volumes, req.Name)
	if err := saveToDisk(d.volumes, d.count); err != nil {
		return err
	}
	return nil
}

func (d *btrfsDriver) Path(req *volume.PathRequest) (*volume.PathResponse, error) {
	return &volume.PathResponse{Mountpoint: getMountpoint(d.home, req.Name)}, nil
}

func (d *btrfsDriver) Mount(req *volume.MountRequest) (*volume.MountResponse, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	vol, exists := d.volumes[req.Name]
	if !exists {
		return &volume.MountResponse{}, fmt.Errorf("Unknown volume %s", req.Name)
	}

	if d.count[req.Name] == 0 {
		device := vol.MountPoint
		mountArgs := []string{device, vol.MountPoint}

		cmd := exec.Command("mount", mountArgs...)
		if out, err := cmd.CombinedOutput(); err != nil {
			d.logger.Err(fmt.Sprintf("Mount: mount error: %s output %s", err, string(out)))
			return &volume.MountResponse{}, fmt.Errorf("Error mounting volume")
		}
	}
	d.count[req.Name]++
	if err := saveToDisk(d.volumes, d.count); err != nil {
		return &volume.MountResponse{}, err
	}
	return &volume.MountResponse{Mountpoint: getMountpoint(d.home, req.Name)}, nil
}

func (d *btrfsDriver) Unmount(req *volume.UnmountRequest) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	vol, exists := d.volumes[req.Name]
	if !exists {
		return fmt.Errorf("Unknown volume %s", req.Name)
	}

	if d.count[req.Name] == 1 {
		cmd := exec.Command("umount", vol.MountPoint)
		if out, err := cmd.CombinedOutput(); err != nil {
			d.logger.Err(fmt.Sprintf("Unmount: umount error: %s output %s", err, string(out)))
			return fmt.Errorf("Error unmounting volume")
		}
	}
	d.count[req.Name]--
	if err := saveToDisk(d.volumes, d.count); err != nil {
		return err
	}
	return nil
}
