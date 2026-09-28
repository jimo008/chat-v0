.PHONY: build test compose-up compose-down

build:
	go build ./cmd/support-api
	go build ./cmd/support-worker

test:
	go test ./...

compose-up:
	./supportctl up

compose-down:
	./supportctl down
