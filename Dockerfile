# Multi-stage build for Colog Docker container log viewer
FROM golang:1.21-alpine AS builder

# Install git and ca-certificates (needed for go modules)
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o colog .

# Final stage: minimal image
FROM scratch

# Copy ca-certificates from builder (needed for HTTPS)
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary
COPY --from=builder /app/colog /colog

# Create a non-root user (even though scratch has no users, this sets metadata)
USER 65534

# Expose no ports (this tool accesses Docker socket)
# Note: You'll need to mount /var/run/docker.sock when running

# Default entrypoint
ENTRYPOINT ["/colog"]

# Default command
CMD ["--help"]

# Labels for metadata
LABEL maintainer="berkantay" \
      description="Docker container log viewer with SDK support" \
      version="1.2.0" \
      org.opencontainers.image.title="Colog" \
      org.opencontainers.image.description="Docker container log viewer with TUI and SDK" \
      org.opencontainers.image.vendor="berkantay" \
      org.opencontainers.image.licenses="MIT"