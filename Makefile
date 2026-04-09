BIN_DIR := bin

.PHONY: all build server inspect clean

all: build

build: server inspect

server:
	go mod tidy
	go build -o $(BIN_DIR)/server ./cmd/server

inspect:
	go mod tidy
	go build -o $(BIN_DIR)/inspect ./cmd/inspect

clean:
	rm -rf $(BIN_DIR)
