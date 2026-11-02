# Go 1.27 is pinned in go.mod; older local toolchains download it automatically.
GO ?= go
CONFIG ?= configs/modhak.local.yaml

.PHONY: build test vet fmt check run serve dist up down reset logs

build:
	CGO_ENABLED=0 $(GO) build -o bin/modhak ./cmd/modhak

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	@test -z "$$(gofmt -l .)" || (gofmt -l .; echo "gofmt needed"; exit 1)

check: fmt vet test

# make run ARGS="capabilities --config configs/modhak.local.yaml"
run:
	$(GO) run ./cmd/modhak $(ARGS)

# Build release archives for all platforms into dist/ (same script CI uses).
# make dist VERSION=v0.1.0
VERSION ?= dev
dist:
	scripts/release-build.sh $(VERSION) dist

# Web UI on http://127.0.0.1:8080 (no auth yet: loopback only).
serve:
	$(GO) run ./cmd/modhak serve --config $(CONFIG)

up:
	@test -f $(CONFIG) || cp configs/modhak.example.yaml $(CONFIG)
	docker compose up -d --wait

down:
	docker compose --profile bare --profile matrix --profile app down

# Drop containers and data (the init scripts only run on an empty volume).
reset:
	docker compose --profile bare --profile matrix --profile app down -v

logs:
	docker compose logs -f --tail=50
