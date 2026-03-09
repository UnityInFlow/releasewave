.PHONY: build run test clean

build:
	CGO_ENABLED=0 go build -o bin/releasewave ./cmd/releasewave

run:
	CGO_ENABLED=0 go run ./cmd/releasewave $(ARGS)

test:
	go test ./...

clean:
	rm -rf bin/
