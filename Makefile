# Go 1.27 is pinned in go.mod; older local toolchains download it automatically.
GO ?= go
CONFIG ?= configs/modhak.local.yaml

.PHONY: build test vet fmt check run up down reset logs

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
