# Colog v2.0.0 Release Notes

## 🎉 Major Release: Smart Docker Connection System

This release introduces a completely redesigned Docker connection system that automatically detects and connects to your preferred Docker setup, making Colog work seamlessly across Docker Desktop, OrbStack, and standard Docker installations.

## 🚀 What's New

### Smart Docker Connection System
- **Automatic Detection**: Colog now automatically discovers all available Docker endpoints on your system
- **Interactive Selection**: When multiple Docker systems are available, Colog presents a clean selection menu
- **Zero Configuration**: No manual setup required - works out of the box with any Docker setup
- **Improved Reliability**: Added connection testing, retries, and timeout handling to eliminate connection issues

### Enhanced Navigation & User Experience
- **Vim-style Navigation**: Use familiar `hjkl` keys to navigate between containers
- **Fullscreen Mode**: Press `Space` to focus on a single container, perfect for debugging
- **Smart Log Export**: Press `y` to copy recent logs to clipboard in markdown format for LLM analysis
- **Better Visual Feedback**: Clear indicators for the currently selected container

### SDK Improvements
- **Dual Modes**: Choose between automatic (`NewDockerService()`) or interactive (`NewDockerServiceInteractive()`) endpoint selection
- **Same Smart Connection**: SDK benefits from the same intelligent endpoint detection as the CLI
- **Better Error Handling**: More descriptive error messages and connection status feedback

## 🔧 Installation

### Option 1: Install with Go
```bash
go install github.com/berkantay/colog@v2.0.0
```

### Option 2: Download Binary
Download from [GitHub Releases](https://github.com/berkantay/colog/releases/tag/v2.0.0)

### Option 3: Build from Source
```bash
git clone https://github.com/berkantay/colog.git
cd colog
git checkout v2.0.0
go build -o colog .
```

## ⌨️ New Keyboard Controls

| Key | Action | Description |
|-----|--------|-------------|
| `h,j,k,l` | Vim navigation | Navigate between containers |
| `Space` | Toggle fullscreen | Focus on single container |
| `y` | Export logs | Copy logs to clipboard |
| `q` | Quit | Clean exit |
| `Ctrl+C` | Force quit | Emergency exit |

## 🐳 Docker Compatibility

Colog v2.0.0 automatically works with:
- **Docker Desktop** (macOS, Windows, Linux)
- **OrbStack** (macOS)
- **Standard Docker** (Linux servers, WSL)
- **Remote Docker** (via Docker contexts)

No configuration required - Colog detects and connects automatically!

## 💻 SDK Usage Examples

### Automatic Connection (Recommended)
```go
dockerService, err := colog.NewDockerService()
if err != nil {
    log.Fatal(err)
}
defer dockerService.Close()
```

### Interactive Selection
```go
dockerService, err := colog.NewDockerServiceInteractive()
if err != nil {
    log.Fatal(err)
}
defer dockerService.Close()
```

## 🐛 Bug Fixes & Improvements

- ✅ Fixed intermittent Docker connection failures
- ✅ Resolved Docker context detection issues  
- ✅ Improved resource cleanup and connection management
- ✅ Enhanced error messages with actionable suggestions
- ✅ Better handling of Docker daemon disconnection
- ✅ Added proper timeouts to prevent hanging

## 🔄 Migration from v1.x

No breaking changes! Your existing usage patterns continue to work:
- CLI usage: `colog` works exactly the same, but with better connection reliability
- SDK usage: Existing `NewDockerService()` calls work unchanged

## 🙏 Acknowledgments

Special thanks to the community for reporting connection issues and providing feedback. This release addresses the most common pain points and makes Colog truly "zero-configuration."

---

**Download Colog v2.0.0 today and experience hassle-free Docker log monitoring!**