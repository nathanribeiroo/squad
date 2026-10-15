VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null)
LDFLAGS := -s -w -X github.com/nathanribeiroo/squad/internal/version.Version=$(VERSION) -X github.com/nathanribeiroo/squad/internal/version.Commit=$(COMMIT)

.PHONY: build build-windows test vet check web icons skill-zip clean

build:
	go build -ldflags "$(LDFLAGS)" -o bin/squad ./cmd/squad

build-windows:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/squad.exe ./cmd/squad

test:
	go test ./...

vet:
	go vet ./...
	GOOS=windows go vet ./...

# Everything that must pass before merging a stage into main.
check: vet test build build-windows

# Rebuilds the embedded UI (web/ → internal/webui/dist). The build output is
# committed so `go build` and `go install` ship the canvas without Node.
web:
	cd web && npm ci && npm run build

# Regenerates the icon font subset after changing web/src/lib/icons.ts.
# Needs Python with fontTools and brotli (pip install fonttools brotli).
icons:
	test -d web/node_modules || (cd web && npm ci)
	cd web && python3 scripts/subset-icons.py

# Packs the skill that `squad install-skill` writes into squad-skill.zip
# (folder squad/ with SKILL.md), ready to copy or upload.
skill-zip: build
	rm -rf bin/skill squad-skill.zip
	bin/squad install-skill -dir bin/skill
	cd bin/skill && zip -qrX ../../squad-skill.zip squad

clean:
	rm -rf bin
