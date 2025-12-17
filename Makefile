.PHONY: build test clean lint run

BINARY_NAME=omblego
BUILD_DIR=bin

build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/omblego

test:
	go test ./...

test-verbose:
	go test -v ./...

clean:
	rm -rf $(BUILD_DIR)
	go clean

lint:
	golangci-lint run

run: build
	./$(BUILD_DIR)/$(BINARY_NAME)

install:
	go install ./cmd/omblego
