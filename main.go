package main

import (
	"fmt"
	"os"
)

func main() {
	// Check if this is an SDK command
	if len(os.Args) > 1 && os.Args[1] == "sdk" {
		if err := runSDKCommand(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "SDK Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fmt.Println("Colog - Docker Container Logs Viewer")
	
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		printHelp()
		return
	}
	
	app := NewApp()
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println(`Colog - Live Docker Container Logs Viewer

USAGE:
    colog [COMMAND] [OPTIONS]

COMMANDS:
    (default)      Start the interactive TUI log viewer
    sdk            Use SDK commands for programmatic access

OPTIONS:
    -h, --help     Show this help message

TUI CONTROLS:
    q              Quit the application
    g              Export last 50 log lines from each container for LLM analysis
    Tab            Navigate between containers
    Ctrl+C         Quit the application

SDK USAGE:
    colog sdk --help                           # Show SDK help
    colog sdk list                             # List running containers
    colog sdk logs <container_id> --tail 50    # Get container logs
    colog sdk export --format markdown         # Export logs for LLM

DESCRIPTION:
    Colog displays live logs from all running Docker containers in a clean,
    grid-based terminal interface. Each container gets its own pane with
    color-coded titles and real-time log streaming.

    The SDK mode provides programmatic access to container information and logs,
    perfect for integration with monitoring systems or LLM analysis workflows.`)
}