BIN_DIR := bin

.PHONY: all build server inspect tidy clean

all: build

build: tidy server inspect

tidy:
	go mod tidy

server:
	go build -o $(BIN_DIR)/server ./cmd/server

inspect:
	go build -o $(BIN_DIR)/inspect ./cmd/inspect

clean:
	rm -rf $(BIN_DIR)
