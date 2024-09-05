# Docker BTRFS Volume Plugin

## NAME
docker-btrfs-plugin - Docker Volume Driver for BTRFS volumes

## SYNOPSIS
`docker-btrfs-plugin` [ `-debug` ] [ `-version` ]

## OVERVIEW
`docker-btrfs-plugin` is a Docker volume plugin designed to use BTRFS subvolumes as the local backend storage for each persistent named volume. It supports volume creation, removal, listing, mounting, and unmounting.

## FEATURES

- **Efficiency:** Designed to be simple, fast, and have low memory requirements.
- **Snapshot Management:** Create, list, delete, and restore snapshots.
- **Subvolume Management:** Each Docker persistent named volume is bound to a nested BTRFS subvolume
- **Logging & Exception Handling:** Built-in mechanisms for logging and error handling.

## Build Prerequisites

- **go:** Make sure Golang build tools are installed to build the plugin.
- **pandoc:** Make sure pandoc is installed to create a man page from source. 

## Runtime Requirements

- **btrfs-progs:** Make sure the btrfs commandline utility programs are installed for the volume plugin to operate correctly.

## SETUP

1. Clone the repository:
   ```bash
   git clone git@github.com:thailgrott/docker-btrfs-plugin.git
   ```
   Alternatively, use HTTPS:
   ```bash
   git clone https://github.com/thailgrott/docker-btrfs-plugin.git
   ```
2. Change into the directory:
   ```bash
   cd docker-btrfs-plugin
   ```
3. Enable Go modules:
   ```bash
   export GO111MODULE=on
   ```
4. Build the plugin:
   ```bash
   make
   ```
5. Install the plugin:
   ```bash
   sudo make install
   ```

## USAGE

### Prepare The Selected BTRFS Bucket Volume
The plugin requires a BTRFS subvolume mounted at the `/var/lib/docker-btrfs-plugin` directory which is where the plugin stores volumes and snapshots as BTRFS subvolumes. 

1. Creating a Systemd Mount Unit
   Create a file named `var-lib-docker-btrfs-plugin.mount` under `/etc/systemd/system/`. You can use your preferred text editor to create and edit this file:
   ```bash
   sudo nano /etc/systemd/system/var-lib-docker-btrfs-plugin.mount
   ```
2. Add the Following Configuration:
   ```ini
   [Unit]
   Description=Mount BTRFS Subvolume for Docker BTRFS Plugin
   Documentation=man:docker-btrfs-plugin(8)
   After=local-fs.target

   [Mount]
   What=/btrfs_pool/volume_store
   Where=/var/lib/docker-btrfs-plugin
   Type=btrfs
   Options=subvol=/volume_store

   [Install]
   WantedBy=multi-user.target
   ```
3. Reload Systemd Configuration:
After creating the mount unit file, reload the systemd configuration to recognize the new unit:
   ```bash
   sudo systemctl daemon-reload
   ```
4. Enable and Start the Mount Unit:
To ensure that the mount unit is automatically started on boot and to start it immediately, use the following commands:
   Enable on next boot:
   ```bash
   sudo systemctl enable var-lib-docker-btrfs-plugin.mount
   ```
   Immediately mount: 
   ```bash
   sudo systemctl start var-lib-docker-btrfs-plugin.mount
   ```
5. Verify the Mount:
   Check the status of the mount unit to ensure it is properly mounted:
   ```bash
   systemctl status var-lib-docker-btrfs-plugin.mount
   ```
   You can also check the mount point directly to verify that the subvolume is mounted:
   ```bash
   df -h /var/lib/docker-btrfs-plugin
   ```

### Starting the Plugin
Start the Docker daemon before starting the docker-btrfs-plugin daemon:
```bash
sudo systemctl start docker
```
Once Docker is up and running, start the plugin:
```bash
sudo systemctl start docker-btrfs-plugin
```
The docker-btrfs-plugin daemon is on-demand socket-activated. Running the docker volume ls command will automatically start the daemon.

### VOLUME CREATION
The docker volume create command supports the creation of BTRFS subvolumes and snapshots.

Usage:
```
docker volume create [OPTIONS]
```

Options:
```
-d, --driver string : Specify volume driver name (default "local")
--label list : Set metadata for a volume (default [])
--name string : Specify volume name
-o, --opt map : Set driver-specific options (default map[])
```
BTRFS Options:
```
--opt snapshot
```

Examples:

Create a BTRFS volume named foobar with a default subvolume:
```bash
docker volume create -d btrfs --name foobar
```

Create a BTRFS snapshot of the foobar volume named foobar_snapshot:
```bash
docker volume create -d btrfs --opt snapshot=foobar --name foobar_snapshot
```

### VOLUME LIST
List all Docker volumes created by all drivers:
```bash
docker volume ls
```

### VOLUME INSPECT
Inspect a specific volume:
```bash
docker volume inspect foobar
```

Example output:
```bash
[
    {
        "Driver": "btrfs",
        "Labels": {},
        "Mountpoint": "/var/lib/docker-btrfs-plugin/foobar",
        "Name": "foobar",
        "Options": {},
        "Scope": "local"
    }
]
```

### VOLUME REMOVAL
Remove a BTRFS volume:
```bash
docker volume rm foobar
```

### ACCESS VOLUME INSIDE THE CONTAINER
Bind mount the BTRFS volume inside a container:
```bash
docker run -it -v foobar:/home fedora /bin/bash
```
This will bind mount the logical volume foobar into the /home directory of the container.

## SUPPORTED ENVIRONMENTS
Currently supported environments include Fedora, Rocky, SuSE and Ubuntu 

## AUTHOR
The docker-btrfs-plugin was developed by Thailgrott.
