# Makefile for nixinit client and server

# Variables
GO := go
GOFLAGS :=
LDFLAGS := -ldflags '-extldflags "-static" -w -s'
BUILD_DIR := build
SERVER_BINARY := nixinit-server
CLIENT_BINARY := nixinit
SERVER_SRC := ./cmd/nixinit-server
CLIENT_SRC := ./cmd/nixinit

# Phony targets
.PHONY: all clean build run-server run-client test server client

# Default target
all: server client

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)

tidy:
	go mod tidy
	go mod vendor

# Build both server and client
build: server client

# Build the nixinit-server
server: .FORCE
	@echo "Building $(SERVER_BINARY)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -a -trimpath $(LDFLAGS) -o $(BUILD_DIR)/$(SERVER_BINARY) $(SERVER_SRC)

# Build the nixinit-client
client: .FORCE
	@echo "Building $(CLIENT_BINARY)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -a -trimpath $(LDFLAGS) -o $(BUILD_DIR)/$(CLIENT_BINARY) $(CLIENT_SRC)

# Run the server
run-server: server
	@echo "Running $(SERVER_BINARY)..."
	@$(BUILD_DIR)/$(SERVER_BINARY)

# Run the client
run-client: client
	@echo "Running $(CLIENT_BINARY)..."
	@$(BUILD_DIR)/$(CLIENT_BINARY)

# Run tests
test:
	@echo "Running tests..."
	$(GO) test ./...

# Force target to always run
.FORCE:
