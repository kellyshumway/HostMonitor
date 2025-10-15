# Stage 1: Build the Go application
FROM golang:1.21-alpine AS builder

# Set necessary environment variables
ENV GO111MODULE=on

# Create app directory
WORKDIR /app

# Copy the Go source code
COPY host_monitor.go .

# Build the executable
# CGO_ENABLED=0 creates a statically linked binary (no libc dependency), ideal for alpine.
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /host_monitor host_monitor.go

# Stage 2: Create a minimal production image
FROM alpine:latest

# Install ping utility (optional, but good for testing/debugging within the final image)
RUN apk --no-cache add iputils

# Set environment
WORKDIR /root/
EXPOSE 8080

# Copy the compiled executable from the builder stage
COPY --from=builder /host_monitor .

# Define the entry point and default command with flags
# These defaults can be overridden when running the container
ENTRYPOINT ["./host_monitor"]
CMD ["-hosts", "actiontarget.com, ksl.com, github.com", "-port", "8080", "-interval", "3000"]
