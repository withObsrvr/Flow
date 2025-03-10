FROM golang:1.23.4 AS builder

# Install build dependencies
RUN apt-get update && apt-get install -y git build-essential

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the binaries
RUN go build -buildmode=pie -o bin/flow cmd/flow/main.go
RUN go build -buildmode=pie -o bin/schema-registry cmd/schema-registry/main.go
RUN go build -buildmode=pie -o bin/graphql-api cmd/graphql-api/main.go

# Create a smaller runtime image
FROM ubuntu:22.04

# Install runtime dependencies
RUN apt-get update && apt-get install -y ca-certificates sqlite3 curl netcat-openbsd && rm -rf /var/lib/apt/lists/*

# Set working directory
WORKDIR /app

# Copy binaries from builder stage
COPY --from=builder /app/bin /app/bin

# Copy plugins directory
COPY --from=builder /app/plugins /app/plugins

# Copy examples directory
COPY --from=builder /app/examples /app/examples

# Create a directory for data
RUN mkdir -p /app/data

# Set environment variables
ENV PATH="/app/bin:${PATH}"

# Expose ports
EXPOSE 8080 8081

# Set entrypoint
ENTRYPOINT ["/app/bin/flow"]

# Default command
CMD ["--help"] 