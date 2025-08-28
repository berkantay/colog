package main

import (
	"fmt"
	"os"
)

func main() {
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
    colog [OPTIONS]

OPTIONS:
    -h, --help     Show this help message

CONTROLS:
    q              Quit the application
    Tab            Navigate between containers
    Ctrl+C         Quit the application

DESCRIPTION:
    Colog displays live logs from all running Docker containers in a clean,
    grid-based terminal interface. Each container gets its own pane with
    color-coded titles and real-time log streaming.`)
}