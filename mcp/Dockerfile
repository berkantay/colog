# Multi-stage build for Colog MCP Server
FROM golang:1.21-alpine AS builder

# Install git and ca-certificates
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy parent module files first
COPY ../go.mod ../go.sum ./
COPY ../*.go ./

# Copy MCP server files
COPY mcp/ ./mcp/

# Download dependencies
WORKDIR /app/mcp
RUN go mod download

# Build the MCP server
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o mcp-server .

# Final stage: minimal image
FROM alpine:3.18

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -g 1001 -S colog && \
    adduser -u 1001 -S colog -G colog

# Create necessary directories
RUN mkdir -p /app /var/log/colog && \
    chown -R colog:colog /app /var/log/colog

# Copy the binary
COPY --from=builder /app/mcp/mcp-server /app/mcp-server

# Switch to non-root user
USER colog

# Set working directory
WORKDIR /app

# Expose default port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Environment variables
ENV MCP_PORT=8080 \
    MCP_HOST=0.0.0.0 \
    GIN_MODE=release

# Default entrypoint
ENTRYPOINT ["/app/mcp-server"]

# Labels for metadata
LABEL maintainer="berkantay" \
      description="Colog MCP Server - Docker log access via Model Context Protocol" \
      version="1.2.0" \
      org.opencontainers.image.title="Colog MCP Server" \
      org.opencontainers.image.description="MCP server for Docker container log access with SSE support" \
      org.opencontainers.image.vendor="berkantay" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.source="https://github.com/berkantay/colog"