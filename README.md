# Colog 🐳

A sleek, minimal terminal UI for monitoring live Docker container logs in a beautiful grid layout.

![Colog Demo](https://img.shields.io/badge/made%20with-Go-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/license-MIT-green)
![Docker](https://img.shields.io/badge/docker-required-blue?style=flat&logo=docker)

## ✨ Features

- **Live Log Streaming**: Real-time logs from all running Docker containers
- **Grid Layout**: Beautiful, organized grid view with automatic container arrangement
- **Color-Coded Containers**: Each container gets a unique color for easy identification
- **Minimal & Clean**: Focus on logs with a distraction-free interface
- **Keyboard Controls**: Simple and intuitive navigation
- **No Configuration**: Works out of the box with your Docker setup

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

### Basic Usage
```bash
# Show logs from all running containers
colog
```

### Help
```bash
colog --help
```

## ⌨️ Keyboard Controls

| Key | Action |
|-----|--------|
| `q` | Quit application |
| `Ctrl+C` | Force quit |
| `Tab` | Navigate between containers (future feature) |

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
- Export logs to file
- Custom color themes
- Keyboard navigation improvements

## 🙏 Acknowledgments

- [tview](https://github.com/rivo/tview) - Excellent TUI framework
- [Docker](https://docker.com) - Container platform
- Inspired by tools like `k9s` and `lazydocker`

---

**Made with ❤️ and Go**

*Colog makes Docker log monitoring simple and beautiful.*