APP_NAME=host-monitor
SRC_FILE=host_monitor.go

# Default target executes 'build'
all: build

## Build the Go application
build:
	@echo "Building Host Monitor application: $(SRC_FILE)"
	# Compiles the source file into the specified binary name
	go build -o $(APP_NAME) $(SRC_FILE)
	@echo "Build successful. Binary created: ./$(APP_NAME)"

## Run the compiled application (requires prior 'build')
# Note: You can customize hosts/port using arguments like 'make run args="--hosts=example.com --port=8081"'
run: build
	@echo "Starting $(APP_NAME) dashboard (default port: 8080, interval: 5000ms)..."
	./$(APP_NAME) $(args)

## Clean up the compiled binary file
clean:
	@echo "Cleaning up binary..."
	rm -f $(APP_NAME)
	@echo "Cleanup complete."

.PHONY: all build run clean
