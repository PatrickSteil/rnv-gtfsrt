BIN_DIR := bin

.PHONY: all build server clean

all: build

build: server

server:
	go build -o $(BIN_DIR)/server ./cmd/server

clean:
	rm -rf $(BIN_DIR)
