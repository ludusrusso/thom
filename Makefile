.PHONY: build install test fmt clean

BIN_DIR ?= $(HOME)/.local/bin

build:
	go build -o bin/thomctl ./cmd/thomctl

install: build
	mkdir -p $(BIN_DIR)
	install -m 0755 bin/thomctl $(BIN_DIR)/thomctl
	@echo "installed: $(BIN_DIR)/thomctl"

test:
	go test ./...

fmt:
	gofmt -w .

clean:
	rm -rf bin
