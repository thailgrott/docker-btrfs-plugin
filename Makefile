# Installation Directories
SYSCONFDIR ?=$(DESTDIR)/etc/docker
SYSTEMDIR ?=$(DESTDIR)/usr/lib/systemd/system
GOLANG=go
BINARY ?= docker-btrfs-plugin
MANPAGE_SRC ?= man/docker-btrfs-plugin.8.md
MANPAGE_DST ?= docker-btrfs-plugin.8
MANPAGE_DIR ?= $(DESTDIR)/usr/share/man/man8
BINDIR ?=$(DESTDIR)/usr/libexec/docker

# Go environment settings
export GO111MODULE=on
export GO15VENDOREXPERIMENT=1
export GOOS=linux

# Build the manual and plugin binary
all: prepare-man btrfs-build

# Convert the Markdown file to man page format using pandoc
.PHONY: prepare-man
prepare-man:
	pandoc -s -t man $(MANPAGE_SRC) -o $(MANPAGE_DST)

# Install the man page to the system's man directory
.PHONY: install-man
install-man: prepare-man
	install -d $(MANPAGE_DIR)
	install -m 644 $(MANPAGE_DST) $(MANPAGE_DIR)/$(MANPAGE_DST)


.PHONY: btrfs-build
btrfs-build: main.go go.mod
	$(GOLANG) mod tidy
	$(GOLANG) build -o $(BINARY) .

.PHONY: install
install:
	# Install the systemd service and socket files
	install -D -m 644 systemd/docker-btrfs-plugin.service $(SYSTEMDIR)/docker-btrfs-plugin.service
	install -D -m 644 systemd/docker-btrfs-plugin.socket $(SYSTEMDIR)/docker-btrfs-plugin.socket
	
	# Install the plugin binary
	install -D -m 755 $(BINARY) $(BINDIR)/$(BINARY)
	
	# Install the man page
	install -D -m 644 docker-btrfs-plugin.8 $(MANINSTALLDIR)/man8/docker-btrfs-plugin.8

.PHONY: circleci
circleci:
	# Placeholder for CircleCI setup, replace with actual command if needed
	echo "CircleCI setup not implemented"

.PHONY: test
test:
	# Placeholder for running tests, replace with actual command if needed
	echo "Test execution not implemented"

.PHONY: clean
clean:
	rm -f $(BINARY)
	rm -f btrfs.8

.PHONY: all prepare-man install-man btrfs-build install clean
