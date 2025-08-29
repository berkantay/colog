# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [2.0.0] - 2025-08-29

### 🚀 Major Features

#### Smart Docker Connection System
- **Automatic Docker Endpoint Detection**: Automatically detects and connects to available Docker endpoints (OrbStack, Docker Desktop, standard Docker daemon)
- **Interactive Endpoint Selection**: When multiple Docker systems are available, presents a user-friendly selection menu
- **Improved Connection Reliability**: Added connection testing, timeouts, and retry logic to resolve connection instability issues
- **Context-Aware**: Respects Docker context settings and prioritizes the current context

#### Enhanced Navigation & Controls
- **Vim-style Navigation**: Navigate between containers using `hjkl` keys for intuitive movement
- **Fullscreen Mode**: Press `Space` to toggle fullscreen view for focused container inspection
- **Improved Log Export**: Press `y` to export recent logs to clipboard in markdown format optimized for LLM analysis
- **Better Focus Management**: Clear visual indicators for the currently selected container

### 🔧 SDK Improvements
- **Dual SDK Modes**: 
  - `NewDockerService()` - Non-interactive mode for programmatic use
  - `NewDockerServiceInteractive()` - Interactive mode with endpoint selection
- **Enhanced Connection Options**: Same smart endpoint detection available in SDK
- **Better Error Handling**: More descriptive error messages and connection status feedback

### 🛠️ Technical Improvements
- **Connection Stability**: Resolved Docker daemon connection issues through intelligent endpoint detection
- **Better Error Messages**: Clear feedback when connections fail with suggestions for resolution
- **Timeout Handling**: Added proper timeouts for Docker API calls to prevent hanging
- **Resource Management**: Improved cleanup and connection management

### 🐛 Bug Fixes
- Fixed intermittent Docker connection failures
- Resolved issues with Docker context detection
- Fixed potential memory leaks in log streaming
- Improved handling of Docker daemon disconnection

### 📚 Documentation
- Updated README with new features and improved troubleshooting guide
- Added examples for both interactive and non-interactive SDK usage
- Enhanced keyboard controls documentation
- Improved installation and setup instructions

## [1.0.0] - Previous Release

### Features
- Interactive TUI mode with live log streaming
- Grid layout with automatic container arrangement  
- Color-coded containers
- SDK for programmatic access
- Basic Docker integration
- Log export functionality