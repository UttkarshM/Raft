# Multi-stage build for optimized image size
FROM golang:1.23.4-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o raft-server ./cmd/server/main.go

# Final stage - minimal image
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/raft-server .

# Expose gRPC port
EXPOSE 5001

# Run the binary
CMD ["./raft-server"]
