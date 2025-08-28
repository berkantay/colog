# Colog 🐳

A powerful Docker container log viewer with both interactive TUI and programmatic SDK for monitoring, analysis, and LLM integration.

![Colog Demo](https://img.shields.io/badge/made%20with-Go-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/license-MIT-green)
![Docker](https://img.shields.io/badge/docker-required-blue?style=flat&logo=docker)

## ✨ Features

### 🖥️ Interactive TUI Mode
- **Live Log Streaming**: Real-time logs from all running Docker containers
- **Grid Layout**: Beautiful, organized grid view with automatic container arrangement
- **Color-Coded Containers**: Each container gets a unique color for easy identification
- **Minimal & Clean**: Focus on logs with a distraction-free interface
- **Keyboard Controls**: Simple and intuitive navigation
- **No Configuration**: Works out of the box with your Docker setup

### 🔧 SDK Mode
- **Programmatic Access**: Extract container logs and information via Go SDK
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

- Docker installed and running
- Docker daemon accessible (usually via `/var/run/docker.sock`)
- At least one running Docker container

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
| `q` | Quit application | Cleanly exit Colog and return to terminal |
| `g` | Export logs | Export last 50 log lines from each container for LLM analysis |
| `Tab` | Navigate containers | Switch focus between different container log panels |
| `Ctrl+C` | Force quit | Immediately terminate the application |

### Navigation Tips
- **Container Focus**: Use `Tab` to cycle through containers and highlight the active panel
- **Log Export**: Press `g` to quickly export recent logs for analysis or sharing
- **Clean Exit**: Always use `q` for a proper shutdown that ensures all resources are cleaned up

## 🏗️ How It Works

1. **Container Discovery**: Connects to Docker daemon and lists all running containers
2. **Grid Layout**: Automatically arranges containers in an optimal grid layout
3. **Live Streaming**: Opens log streams for each container using Docker API
4. **Real-time Updates**: Continuously displays new log entries with timestamps
5. **Color Coding**: Assigns unique colors to container borders and titles

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
- Check Docker daemon is running: `systemctl status docker`
- Verify Docker socket permissions: `ls -la /var/run/docker.sock`
- On macOS, ensure Docker Desktop is running

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
    sdk, err := NewColog(ctx)
    if err != nil {
        log.Fatal(err)
    }
    defer sdk.Close()

    // Get all running containers
    containers, _ := sdk.ListRunningContainers()
    
    // Extract container IDs
    var containerIDs []string
    for _, c := range containers {
        containerIDs = append(containerIDs, c.ID)
    }
    
    // Export logs for LLM analysis
    markdown, err := sdk.ExportLogsAsMarkdown(containerIDs, LogOptions{
        Tail: 100,
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Send to your LLM service
    fmt.Println("Ready for LLM analysis:", len(markdown), "characters")
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