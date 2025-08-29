# Colog MCP Integration with Cursor

This guide shows how to connect the Colog MCP server directly to Cursor IDE using the stdio transport (Option 2).

## 🚀 Quick Setup

### 1. Build Colog
```bash
cd /path/to/colog
go build -o colog .
```

### 2. Test MCP Functionality
```bash
# Test initialization
echo '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}}}' | ./colog -m stdio

# Test tool listing  
echo '{"jsonrpc": "2.0", "id": 2, "method": "tools/list"}' | ./colog -m stdio

# Test container listing
echo '{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "list_containers", "arguments": {"all": false}}}' | ./colog -m stdio
```

### 3. Configure Cursor

Create or update your Cursor MCP configuration file:

**Location**: `~/.cursor/mcp_servers.json` (or wherever Cursor stores MCP config)

**Configuration**:
```json
{
  "mcpServers": {
    "colog-docker": {
      "command": "/Users/berkantay/Projects/berkant/colog/colog",
      "args": ["-m", "stdio"],
      "env": {
        "PATH": "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin",
        "HOME": "/Users/berkantay"
      }
    }
  }
}
```

**Important PATH Notes**:
- Include `/opt/homebrew/bin` for Homebrew Docker on Apple Silicon
- Include `/usr/local/bin` for Intel Homebrew or Docker Desktop
- Replace `/Users/berkantay` with your actual home directory path

**Important**: Update the `command` path to point to your actual colog binary location.

### 4. Restart Cursor

After adding the configuration, restart Cursor to load the new MCP server.

## 🔧 Available Tools

Once connected, you'll have access to these 4 MCP tools in Cursor:

### 1. `list_containers`
Lists all Docker containers (running by default, or all with `all: true`)

**Usage in Cursor**: *"Show me all running Docker containers"*

### 2. `get_container_logs` 
Gets logs from a specific container

**Parameters**:
- `container_id` (required): Container ID or name
- `tail` (optional): Number of recent lines (default: 50)
- `since` (optional): Show logs since timestamp

**Usage in Cursor**: *"Get the latest 100 logs from the postgres container"*

### 3. `export_logs_llm`
Exports container logs in markdown format optimized for LLM analysis

**Parameters**:
- `tail` (optional): Lines per container (default: 50)
- `containers` (optional): Specific container list (default: all running)

**Usage in Cursor**: *"Export all container logs for debugging analysis"*

### 4. `filter_containers`
Filters containers by various criteria

**Parameters**:
- `status` (optional): Filter by status (running, exited, etc.)
- `image` (optional): Filter by image name
- `name` (optional): Filter by container name

**Usage in Cursor**: *"Find all containers running postgres image"*

## 🐳 Docker Support

The MCP server supports multiple Docker environments:

- **OrbStack** (`~/.orbstack/run/docker.sock`) ✅
- **Docker Desktop** (`~/.docker/run/docker.sock`) ✅  
- **Standard Docker** (`/var/run/docker.sock`) ✅

The server automatically discovers and connects to the best available Docker endpoint.

## 💡 Example Cursor Interactions

Once configured, you can ask Cursor things like:

- *"What containers are currently running?"*
- *"Show me the logs from the web-server container"*
- *"Find any containers that are unhealthy"*
- *"Export logs from all my microservices for debugging"*
- *"Get the latest 200 log lines from the database container"*

## 🛠️ Troubleshooting

### MCP Server Not Connecting
1. Verify the colog binary path in the configuration
2. Test the MCP server manually with the test commands above
3. Check Cursor's MCP logs for error messages

### No Docker Containers Found
1. Ensure Docker is running
2. Verify your user has Docker permissions
3. Test with: `docker ps` to confirm containers are visible

### Permission Issues
1. Ensure colog binary is executable: `chmod +x ./colog`
2. Verify Docker socket permissions
3. On some systems, add your user to the docker group

## 🎉 Success!

You should now have full Docker container log access directly within Cursor through the MCP protocol!

The integration provides real-time access to:
- Container listings and status
- Live container logs
- Formatted log exports for LLM analysis
- Container filtering and search

Perfect for debugging, monitoring, and analyzing your containerized applications right from your IDE.