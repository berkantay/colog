# Colog 🐳

A powerful Docker container log viewer with both interactive TUI and programmatic SDK for monitoring, analysis, and LLM integration.

![Colog Demo](https://img.shields.io/badge/made%20with-Go-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/license-MIT-green)
![Docker](https://img.shields.io/badge/docker-required-blue?style=flat&logo=docker)

## ✨ Features

### 🖥️ Interactive TUI Mode
- **Live Log Streaming**: Real-time logs from all running Docker containers
- **Smart Docker Connection**: Automatic detection and selection of Docker endpoints (OrbStack, Docker Desktop, etc.)
- **Grid Layout**: Beautiful, organized grid view with automatic container arrangement
- **Vim-style Navigation**: Navigate containers with `hjkl` keys, fullscreen toggle with `Space`
- **Color-Coded Containers**: Each container gets a unique color for easy identification
- **Log Export**: Export logs for LLM analysis with `y` key
- **Minimal & Clean**: Focus on logs with a distraction-free interface
- **No Configuration**: Works out of the box with your Docker setup

### 🔧 SDK Mode
- **Programmatic Access**: Extract container logs and information via Go SDK
- **Smart Docker Connection**: Same intelligent endpoint detection for programmatic use
- **Interactive & Non-Interactive Modes**: Choose automatic or manual Docker endpoint selection
- **Batch Operations**: Process multiple containers simultaneously
- **Smart Filtering**: Filter containers by name, image, status, labels, and more
- **LLM Integration**: Export logs in JSON/Markdown formats optimized for AI analysis
- **Time-based Queries**: Retrieve logs within specific time ranges
- **Command-line Interface**: Use SDK features directly from the command line

## 🚀 Installation

### Option 1: Install with Go
```bash
go install github.com/berkantay/colog@latest
```

### Option 2: Build from Source
```bash
git clone https://github.com/berkantay/colog.git
cd colog
go build -o colog
sudo mv colog /usr/local/bin/
```

### Option 3: Download Binary
Download the latest release from [GitHub Releases](https://github.com/berkantay/colog/releases)

## 📋 Requirements

- Docker installed and running (Docker Desktop, OrbStack, or standard Docker daemon)
- At least one running Docker container
- **No manual configuration needed** - Colog automatically detects and connects to available Docker endpoints

## 🎮 Usage

Colog operates in two modes: **Interactive TUI** (default) and **SDK Mode** for programmatic access.

### 🖥️ Interactive TUI Mode

```bash
# Show logs from all running containers in TUI
colog

# Show help
colog --help
```

The TUI application will automatically:
1. **Discover** all running Docker containers
2. **Arrange** them in an optimal grid layout
3. **Stream** live logs from each container in real-time
4. **Color-code** each container with unique borders and titles

### 🔧 SDK Mode

```bash
# List all running containers
colog sdk list

# Get logs from a specific container
colog sdk logs abc123 --tail 50

# Export logs for LLM analysis
colog sdk export --format markdown --tail 100

# Filter containers by image
colog sdk filter --image nginx

# Show SDK help
colog sdk --help
```

## ⌨️ Keyboard Controls

| Key | Action | Description |
|-----|--------|-------------|
| `h,j,k,l` | Vim navigation | Navigate between containers using vim-style keys |
| `Space` | Toggle fullscreen | Fullscreen the selected container or return to grid view |
| `y` | Export logs | Export recent logs to clipboard in markdown format |
| `q` | Quit application | Cleanly exit Colog and return to terminal |
| `Ctrl+C` | Force quit | Immediately terminate the application |

### Navigation Tips
- **Container Focus**: Use vim-style `hjkl` keys to navigate between containers
- **Fullscreen Mode**: Press `Space` to focus on a single container, press again to return to grid
- **Log Export**: Press `y` to copy recent logs to clipboard for LLM analysis
- **Clean Exit**: Always use `q` for a proper shutdown that ensures all resources are cleaned up

## 🏗️ How It Works

1. **Smart Connection**: Automatically detects and connects to available Docker endpoints (OrbStack, Docker Desktop, standard Docker)
2. **Container Discovery**: Lists all running containers from the selected Docker endpoint
3. **Grid Layout**: Automatically arranges containers in an optimal grid layout
4. **Live Streaming**: Opens log streams for each container using Docker API
5. **Real-time Updates**: Continuously displays new log entries with timestamps
6. **Interactive Navigation**: Vim-style keyboard navigation with fullscreen support

## 🎨 Features in Detail

### Grid Layout
- Automatically calculates optimal rows/columns based on container count
- Square-ish layout for best screen utilization
- Each container gets equal space

### Color System
- 14 distinct colors cycle through containers
- Border and title colors match for easy identification
- Readable color combinations for all terminal themes

### Log Format
- Timestamps in `HH:MM:SS` format
- Clean log parsing that handles Docker's log format
- Scrollable view with automatic scroll-to-end

## 🔧 Development

### Prerequisites
- Go 1.21+
- Docker
- Terminal with color support

### Building
```bash
go mod tidy
go build -o colog
```

### Dependencies
- `github.com/rivo/tview` - Terminal UI framework
- `github.com/docker/docker` - Docker client library
- `github.com/gdamore/tcell/v2` - Terminal handling

## 🐛 Troubleshooting

### "No running containers found"
- Make sure Docker is running: `docker ps`
- Ensure you have running containers: `docker run -d nginx`

### "Failed to connect to Docker"
- **No worries!** Colog automatically detects and tries multiple Docker endpoints
- Ensure at least one Docker system is running:
  - **Docker Desktop**: Start Docker Desktop application
  - **OrbStack**: Start OrbStack application
  - **Standard Docker**: `systemctl status docker` (Linux)
- If multiple Docker systems are available, Colog will show a selection menu

### Permission Issues
```bash
# Add user to docker group (Linux)
sudo usermod -aG docker $USER
# Then log out and back in
```

## 🚀 SDK Integration & LLM Usage

For detailed SDK documentation and examples, see [SDK_README.md](SDK_README.md).

### Quick SDK Integration Example

```go
package main

import (
    "context"
    "fmt"
    "log"
)

func main() {
    ctx := context.Background()
    
    // Option 1: Automatic endpoint selection (recommended)
    dockerService, err := colog.NewDockerService()
    if err != nil {
        log.Fatal(err)
    }
    defer dockerService.Close()
    
    // Option 2: Interactive endpoint selection
    // dockerService, err := colog.NewDockerServiceInteractive()
    
    // Get all running containers
    containers, err := dockerService.ListRunningContainers(ctx)
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Found %d running containers\n", len(containers))
    
    // Stream logs from first container
    if len(containers) > 0 {
        logCh := make(chan colog.LogEntry, 100)
        go dockerService.StreamLogs(ctx, containers[0].ID, logCh)
        
        // Process logs...
    }
}
```

### LLM Integration Examples

```bash
# Export logs and pipe to LLM analysis tool
colog sdk export --format markdown --tail 100 | your-llm-tool

# Export as JSON for structured analysis
colog sdk export --format json --output logs.json

# Monitor and alert on high error counts
colog sdk export --format json | jq '.summary.error_count'
```

## 📝 License

MIT License - see [LICENSE](LICENSE) file for details.

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

### Development Setup
1. Fork the repository
2. Clone your fork: `git clone https://github.com/berkantay/colog.git`
3. Create a feature branch: `git checkout -b feature-name`
4. Make your changes and test thoroughly
5. Submit a pull request

### Ideas for Contributions
- Container filtering options
- Search functionality
- Custom color themes
- Keyboard navigation improvements
- Advanced SDK features
- Additional export formats
- LLM integration examples

## 🙏 Acknowledgments

- [tview](https://github.com/rivo/tview) - Excellent TUI framework
- [Docker](https://docker.com) - Container platform
- Inspired by tools like `k9s` and `lazydocker`

---

**Made with ❤️ and Go**

*Colog makes Docker log monitoring simple and beautiful.*