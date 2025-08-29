# Colog MCP Server Documentation

The Colog MCP (Model Context Protocol) Server provides LLMs with direct access to Docker container logs and information through a standardized protocol. It supports both local and remote deployment with SSE (Server-Sent Events) for real-time streaming.

## Table of Contents

- [Overview](#overview)
- [Features](#features)
- [Quick Start](#quick-start)
- [Installation & Deployment](#installation--deployment)
- [MCP Tools](#mcp-tools)
- [Configuration](#configuration)
- [Remote Access](#remote-access)
- [Security](#security)
- [Examples](#examples)

## Overview

The MCP server implements the Model Context Protocol specification, enabling LLMs to:
- **Access Docker containers** and their logs programmatically
- **Stream real-time logs** via Server-Sent Events (SSE)
- **Export formatted logs** optimized for AI analysis
- **Filter containers** by various criteria
- **Operate remotely** with authentication and CORS support

## Features

### 🔧 MCP Protocol Support
- **Streamable HTTP Transport** with SSE streaming
- **Standard MCP Tools** for container operations
- **Real-time Notifications** via SSE
- **Session Management** with persistent connections
- **Error Handling** with standard MCP error codes

### 🐳 Docker Integration
- **Container Discovery** - List running and stopped containers
- **Log Streaming** - Real-time and historical log access
- **Filtering** - Find containers by name, image, status, labels
- **Batch Operations** - Process multiple containers simultaneously

### 🤖 LLM Optimization
- **Structured Responses** with metadata and context
- **Export Formats** - JSON and Markdown for AI consumption
- **Error Analysis** - Automatic error detection in logs
- **Time Ranges** - Historical log queries

### 🔒 Security & Deployment
- **Authentication** - API key support
- **CORS Configuration** - Remote access control
- **Docker Socket** - Secure container access
- **Health Monitoring** - Built-in health checks

## Quick Start

### Docker Compose (Recommended)

```bash
# Clone the repository
git clone https://github.com/berkantay/colog.git
cd colog

# Start with Docker Compose
docker-compose -f docker-compose.mcp.yml up -d

# Test the server
curl http://localhost:8080/health
```

### Docker Run

```bash
# Run the MCP server
docker run -d \
  --name colog-mcp \
  -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -e MCP_PORT=8080 \
  -e MCP_HOST=0.0.0.0 \
  berkantay/colog-mcp:latest

# Check health
curl http://localhost:8080/health
```

### Local Development

```bash
cd colog/mcp
go mod tidy
go run server.go
```

## Installation & Deployment

### 1. Docker Sidecar Pattern

Deploy alongside your applications:

```yaml
# docker-compose.yml
version: '3.8'
services:
  app:
    image: your-app:latest
    # your app config

  colog-mcp:
    image: berkantay/colog-mcp:latest
    ports:
      - "8080:8080"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    environment:
      - MCP_API_KEY=your-secret-key
```

### 2. Kubernetes Deployment

```bash
# Apply Kubernetes manifests
kubectl apply -f k8s/mcp-deployment.yaml

# Check status
kubectl get pods -n colog-mcp
```

### 3. Remote Server

For remote LLM access:

```bash
# Deploy to cloud with SSL
docker run -d \
  --name colog-mcp \
  -p 443:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -e MCP_HOST=0.0.0.0 \
  -e MCP_API_KEY=your-secure-api-key \
  -e MCP_ALLOWED_ORIGINS=https://your-llm-client.com \
  berkantay/colog-mcp:latest
```

## MCP Tools

The server provides these standardized MCP tools:

### `list_containers`

Lists Docker containers with optional filtering.

**Parameters:**
- `all` (boolean, optional) - Include stopped containers (default: false)

**Example:**
```json
{
  "name": "list_containers",
  "arguments": {
    "all": true
  }
}
```

### `get_container_logs`

Retrieves logs from a specific container.

**Parameters:**
- `container_id` (string, required) - Container ID or name
- `tail` (number, optional) - Number of log lines (default: 50)
- `follow` (boolean, optional) - Follow log output (default: false)

**Example:**
```json
{
  "name": "get_container_logs",
  "arguments": {
    "container_id": "abc123",
    "tail": 100,
    "follow": false
  }
}
```

### `export_logs_llm`

Exports logs in LLM-optimized format.

**Parameters:**
- `container_ids` (array, required) - List of container IDs
- `format` (string, optional) - "json" or "markdown" (default: "markdown")
- `tail` (number, optional) - Log lines per container (default: 100)

**Example:**
```json
{
  "name": "export_logs_llm",
  "arguments": {
    "container_ids": ["abc123", "def456"],
    "format": "markdown",
    "tail": 200
  }
}
```

### `filter_containers`

Filters containers by criteria.

**Parameters:**
- `name` (string, optional) - Container name pattern
- `image` (string, optional) - Image name pattern
- `status` (string, optional) - Container status

**Example:**
```json
{
  "name": "filter_containers",
  "arguments": {
    "image": "nginx",
    "status": "running"
  }
}
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `MCP_PORT` | Server port | `8080` |
| `MCP_HOST` | Bind address | `0.0.0.0` |
| `MCP_API_KEY` | Authentication key | None |
| `MCP_ALLOWED_ORIGINS` | CORS origins (comma-separated) | `*` |
| `LOG_LEVEL` | Logging level | `info` |

### Docker Socket Access

The server requires access to the Docker socket:

```bash
# Unix socket (most common)
-v /var/run/docker.sock:/var/run/docker.sock:ro

# TCP socket (remote Docker)
-e DOCKER_HOST=tcp://docker-host:2376
```

### Security Configuration

```bash
# Enable authentication
-e MCP_API_KEY=your-secret-api-key

# Restrict origins
-e MCP_ALLOWED_ORIGINS=https://claude.ai,https://your-app.com

# Read-only Docker socket
-v /var/run/docker.sock:/var/run/docker.sock:ro
```

## Remote Access

### SSE Connection

Connect to the MCP server via Server-Sent Events:

```javascript
const eventSource = new EventSource('http://localhost:8080/mcp?api_key=your-key');

eventSource.onmessage = function(event) {
    const data = JSON.parse(event.data);
    console.log('MCP message:', data);
};
```

### HTTP Requests

Send MCP requests via HTTP POST:

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-key" \
  -d '{
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "list_containers",
      "arguments": {"all": true}
    }
  }'
```

### Claude Desktop Integration

Configure Claude Desktop to use the MCP server:

```json
{
  "mcpServers": {
    "colog": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "-v", "/var/run/docker.sock:/var/run/docker.sock",
        "berkantay/colog-mcp:latest"
      ]
    }
  }
}
```

### Remote MCP Client

For remote servers, use the streamable HTTP transport:

```json
{
  "mcpServers": {
    "colog-remote": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/client-http"],
      "env": {
        "MCP_HTTP_URL": "https://your-server.com/mcp",
        "MCP_API_KEY": "your-api-key"
      }
    }
  }
}
```

## Security

### Authentication

The server supports API key authentication:

```bash
# Set API key
export MCP_API_KEY=your-secure-random-key

# Include in requests
curl -H "X-API-Key: your-secure-random-key" http://localhost:8080/mcp
```

### CORS Configuration

Control cross-origin access:

```bash
# Allow specific origins
export MCP_ALLOWED_ORIGINS=https://claude.ai,https://your-app.com

# Allow all origins (development only)
export MCP_ALLOWED_ORIGINS=*
```

### Docker Security

- **Read-only socket**: Mount Docker socket as read-only
- **Non-root user**: Container runs as user 1001
- **Minimal image**: Based on Alpine Linux scratch
- **Resource limits**: Configure CPU and memory limits

### Network Security

- **Bind locally**: Use `MCP_HOST=127.0.0.1` for local-only access
- **Use HTTPS**: Deploy behind reverse proxy with SSL
- **Firewall**: Restrict port access to authorized clients

## Examples

### Basic Container Monitoring

```javascript
const client = new CologMCPClient('http://localhost:8080');

// List all containers
const containers = await client.listContainers(true);
console.log('Containers:', containers);

// Get logs from nginx containers
const nginx = await client.filterContainers({ image: 'nginx' });
if (nginx.content[1].resource.containers.length > 0) {
    const logs = await client.getContainerLogs(
        nginx.content[1].resource.containers[0].id, 
        100
    );
    console.log('Nginx logs:', logs);
}
```

### LLM Integration

```javascript
// Export logs for AI analysis
const containerIds = ['abc123', 'def456'];
const exported = await client.exportLogsLLM(
    containerIds, 
    'markdown', 
    200
);

// Send to LLM
const prompt = `
Analyze these Docker container logs and identify any issues:

${exported.content[1].resource.data}

Please identify:
1. Error patterns
2. Performance issues
3. Security concerns
4. Recommendations
`;

// Use with your LLM service
const analysis = await llm.complete(prompt);
```

### Real-time Monitoring

```javascript
// Initialize SSE connection
await client.initSSE();

// Monitor containers every minute
setInterval(async () => {
    const containers = await client.listContainers();
    const unhealthy = containers.content[1].resource.containers.filter(
        c => c.status.includes('unhealthy')
    );
    
    if (unhealthy.length > 0) {
        console.log('Unhealthy containers detected:', unhealthy);
        // Send alert or export logs for analysis
    }
}, 60000);
```

### Health Check Integration

```bash
#!/bin/bash
# health-check.sh

# Check MCP server health
HEALTH=$(curl -s http://localhost:8080/health | jq -r '.status')

if [ "$HEALTH" != "healthy" ]; then
    echo "MCP server unhealthy: $HEALTH"
    exit 1
fi

# Check container count
CONTAINERS=$(curl -s http://localhost:8080/mcp \
  -X POST \
  -H "Content-Type: application/json" \
  -d '{"id":1,"method":"tools/call","params":{"name":"list_containers"}}' \
  | jq -r '.result.content[1].resource.count')

echo "MCP server healthy, monitoring $CONTAINERS containers"
```

## API Reference

### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/mcp` | Initialize SSE connection |
| POST | `/mcp` | Send MCP requests |
| GET | `/health` | Health check |
| GET | `/capabilities` | Server capabilities |

### Response Format

All MCP responses follow the standard format:

```json
{
  "id": 1,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "Human-readable description"
      },
      {
        "type": "resource",
        "resource": {
          "type": "containers",
          "data": {},
          "generated_at": "2024-01-01T00:00:00Z"
        }
      }
    ]
  }
}
```

### Error Codes

| Code | Description |
|------|-------------|
| -32700 | Parse error |
| -32600 | Invalid request |
| -32601 | Method not found |
| -32602 | Invalid params |
| -32603 | Internal error |

## Troubleshooting

### Common Issues

1. **Docker socket permission denied**
   ```bash
   # Add user to docker group
   sudo usermod -aG docker $USER
   # Or run with appropriate permissions
   ```

2. **CORS errors**
   ```bash
   # Set allowed origins
   export MCP_ALLOWED_ORIGINS=*
   ```

3. **Connection refused**
   ```bash
   # Check server is running
   curl http://localhost:8080/health
   ```

4. **Authentication failed**
   ```bash
   # Verify API key
   curl -H "X-API-Key: $MCP_API_KEY" http://localhost:8080/health
   ```

### Debugging

Enable debug logging:
```bash
export LOG_LEVEL=debug
```

Check server logs:
```bash
docker logs colog-mcp
```

Test MCP tools directly:
```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"id":1,"method":"tools/list"}'
```

## Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feature-name`
3. Make changes and test with Docker
4. Update documentation
5. Submit a pull request

## License

MIT License - see [LICENSE](LICENSE) file for details.

---

**Made with ❤️ for the MCP ecosystem**

*Bringing Docker container logs to the age of AI*