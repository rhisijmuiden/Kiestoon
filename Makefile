# Kiestoon build. `make` builds the web UI and a single binary with the UI embedded.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo v0.0.0-dev)
# Package versions must not start with "v".
DEB_VERSION ?= $(patsubst v%,%,$(VERSION))
COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GOARCH ?= amd64
LDFLAGS := -s -w \
	-X github.com/rhisijmuiden/kiestoon/server/internal/version.Version=$(VERSION) \
	-X github.com/rhisijmuiden/kiestoon/server/internal/version.Commit=$(COMMIT) \
	-X github.com/rhisijmuiden/kiestoon/server/internal/version.BuildDate=$(BUILD_DATE)

.PHONY: all web build test deb lxc edge edge-deb edge-lxc edge-pi push push-deb clean run

all: build

web:
	cd web && npm ci && npm run i18n:check && npm run build
	rm -rf server/internal/web/dist && mkdir -p server/internal/web/dist
	cp -r web/dist/. server/internal/web/dist/
	touch server/internal/web/dist/.gitkeep

build: web
	mkdir -p build
	cd server && CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) go build -trimpath -ldflags "$(LDFLAGS)" -o ../build/kiestoon-linux-$(GOARCH) ./cmd/kiestoon

test:
	cd server && go vet ./... && go test -race ./...
	cd web && npm run i18n:check && npm run typecheck

# Needs nfpm: go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest
deb: build
	mkdir -p dist build/softphone
	cp build/kiestoon-linux-$(GOARCH) build/kiestoon
	VERSION=$(DEB_VERSION) ARCH=$(GOARCH) nfpm package --config packaging/nfpm.yaml --packager deb --target dist/kiestoon_$(DEB_VERSION)_$(GOARCH).deb

# Kiestoon Edge: the satellite for branch offices (no web UI build needed).
edge:
	mkdir -p build
	cd server && CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) go build -trimpath -ldflags "$(LDFLAGS)" -o ../build/kiestoon-edge-linux-$(GOARCH) ./cmd/kiestoon-edge

edge-deb: edge
	mkdir -p dist
	cp build/kiestoon-edge-linux-$(GOARCH) build/kiestoon-edge
	VERSION=$(DEB_VERSION) ARCH=$(GOARCH) nfpm package --config packaging/edge/nfpm.yaml --packager deb --target dist/kiestoon-edge_$(DEB_VERSION)_$(GOARCH).deb

# The push relay of the mobile app (run by the publisher of the app, not by phone systems).
push:
	mkdir -p build
	cd server && CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) go build -trimpath -ldflags "$(LDFLAGS)" -o ../build/kiestoon-push-linux-$(GOARCH) ./cmd/kiestoon-push

push-deb: push
	mkdir -p dist
	cp build/kiestoon-push-linux-$(GOARCH) build/kiestoon-push
	VERSION=$(DEB_VERSION) ARCH=$(GOARCH) nfpm package --config packaging/push/nfpm.yaml --packager deb --target dist/kiestoon-push_$(DEB_VERSION)_$(GOARCH).deb

edge-lxc: edge-deb
	sudo ./lxc/build-lxc.sh dist/kiestoon-edge_$(DEB_VERSION)_$(GOARCH).deb dist/

# Raspberry Pi image (arm64): Raspberry Pi OS Lite with Kiestoon Edge (needs root, qemu-user-static on amd64).
edge-pi:
	GOARCH=arm64 $(MAKE) edge-deb
	sudo ./packaging/edge/pi-image.sh dist/kiestoon-edge_$(DEB_VERSION)_arm64.deb dist/

# Builds a Proxmox LXC template (needs root and debootstrap).
lxc: deb
	sudo ./lxc/build-lxc.sh dist/kiestoon_$(DEB_VERSION)_$(GOARCH).deb dist/

# Local development: API on :8080 with a throwaway data dir. Run `cd web && npm run dev` alongside.
run:
	cd server && KIESTOON_DATA_DIR=../build/dev-data go run ./cmd/kiestoon

clean:
	rm -rf build dist web/dist server/internal/web/dist/*
