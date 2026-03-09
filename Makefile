.PHONY: build run test lint clean install

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -s -w \
  -X main.version=$(VERSION) \
  -X main.commit=$(COMMIT) \
  -X main.date=$(DATE)

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/releasewave ./cmd/releasewave

run:
	CGO_ENABLED=0 go run -ldflags "$(LDFLAGS)" ./cmd/releasewave $(ARGS)

test:
	go test -race -cover ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/ dist/

install: build
	cp bin/releasewave $(GOPATH)/bin/releasewave 2>/dev/null || \
	cp bin/releasewave /usr/local/bin/releasewave
