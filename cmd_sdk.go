package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Command-line interface for the SDK
func runSDKCommand(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("SDK command required. Use 'colog sdk --help' for usage")
	}

	command := args[0]
	
	switch command {
	case "--help", "-h", "help":
		printSDKHelp()
		return nil
	case "list":
		return runListCommand(args[1:])
	case "logs":
		return runLogsCommand(args[1:])
	case "export":
		return runExportCommand(args[1:])
	case "filter":
		return runFilterCommand(args[1:])
	default:
		return fmt.Errorf("unknown SDK command: %s", command)
	}
}

func printSDKHelp() {
	fmt.Println(`Colog SDK - Programmatic Docker Container Log Access

USAGE:
    colog sdk <COMMAND> [OPTIONS]

COMMANDS:
    list              List containers
    logs              Get logs from containers
    export            Export logs for LLM analysis
    filter            Filter containers by criteria
    help              Show this help message

EXAMPLES:
    colog sdk list                              # List all running containers
    colog sdk list --all                        # List all containers
    colog sdk logs <container_id> --tail 50     # Get last 50 log lines
    colog sdk export --format json --tail 100  # Export logs as JSON
    colog sdk filter --image nginx              # Filter containers by image

For detailed usage of each command, use:
    colog sdk <command> --help`)
}

func runListCommand(args []string) error {
	ctx := context.Background()
	sdk, err := NewColog(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize SDK: %w", err)
	}
	defer sdk.Close()

	showAll := false
	for _, arg := range args {
		if arg == "--all" || arg == "-a" {
			showAll = true
		} else if arg == "--help" || arg == "-h" {
			fmt.Println(`List containers

USAGE:
    colog sdk list [OPTIONS]

OPTIONS:
    --all, -a         List all containers (including stopped)
    --help, -h        Show this help message

EXAMPLES:
    colog sdk list                # List running containers
    colog sdk list --all          # List all containers`)
			return nil
		}
	}

	var containers []ContainerInfo
	if showAll {
		containers, err = sdk.ListAllContainers()
	} else {
		containers, err = sdk.ListRunningContainers()
	}

	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	if len(containers) == 0 {
		fmt.Println("No containers found")
		return nil
	}

	fmt.Printf("%-12s %-20s %-30s %-15s\n", "ID", "NAME", "IMAGE", "STATUS")
	fmt.Println(strings.Repeat("-", 80))
	
	for _, container := range containers {
		id := container.ID
		if len(id) > 12 {
			id = id[:12]
		}
		name := container.Name
		if len(name) > 20 {
			name = name[:17] + "..."
		}
		image := container.Image
		if len(image) > 30 {
			image = image[:27] + "..."
		}
		status := container.Status
		if len(status) > 15 {
			status = status[:12] + "..."
		}
		
		fmt.Printf("%-12s %-20s %-30s %-15s\n", id, name, image, status)
	}

	return nil
}

func runLogsCommand(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("container ID required")
	}

	containerID := args[0]
	
	// Parse options
	options := LogOptions{
		Tail:       50,
		Follow:     false,
		Timestamps: true,
	}

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			fmt.Println(`Get logs from a container

USAGE:
    colog sdk logs <container_id> [OPTIONS]

OPTIONS:
    --tail <n>        Number of log lines to retrieve (default: 50)
    --follow, -f      Follow log output
    --since <time>    Show logs since timestamp (RFC3339 format)
    --until <time>    Show logs until timestamp (RFC3339 format)
    --no-timestamps   Don't show timestamps
    --help, -h        Show this help message

EXAMPLES:
    colog sdk logs abc123 --tail 100           # Get last 100 log lines
    colog sdk logs abc123 --follow             # Follow logs in real-time
    colog sdk logs abc123 --since 2024-01-01T10:00:00Z`)
			return nil
		case "--tail":
			if i+1 < len(args) {
				if tail, err := strconv.Atoi(args[i+1]); err == nil {
					options.Tail = tail
					i++ // Skip the next argument
				}
			}
		case "--follow", "-f":
			options.Follow = true
		case "--since":
			if i+1 < len(args) {
				if since, err := time.Parse(time.RFC3339, args[i+1]); err == nil {
					options.Since = since
					i++ // Skip the next argument
				}
			}
		case "--until":
			if i+1 < len(args) {
				if until, err := time.Parse(time.RFC3339, args[i+1]); err == nil {
					options.Until = until
					i++ // Skip the next argument
				}
			}
		case "--no-timestamps":
			options.Timestamps = false
		}
	}

	ctx := context.Background()
	sdk, err := NewColog(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize SDK: %w", err)
	}
	defer sdk.Close()

	// Get container info first
	container, err := sdk.GetContainerByID(containerID)
	if err != nil {
		return fmt.Errorf("container not found: %w", err)
	}

	fmt.Printf("Getting logs from container: %s (%s)\n", container.Name, container.ID[:12])
	fmt.Println(strings.Repeat("-", 60))

	logs, err := sdk.GetContainerLogs(container.ID, options)
	if err != nil {
		return fmt.Errorf("failed to get logs: %w", err)
	}

	if len(logs) == 0 {
		fmt.Println("No logs found")
		return nil
	}

	for _, logEntry := range logs {
		if options.Timestamps {
			fmt.Printf("[%s] %s\n", logEntry.Timestamp.Format("2006-01-02 15:04:05"), logEntry.Message)
		} else {
			fmt.Println(logEntry.Message)
		}
	}

	return nil
}

func runExportCommand(args []string) error {
	format := "markdown"
	outputFile := ""
	options := LogOptions{
		Tail:       100,
		Follow:     false,
		Timestamps: true,
	}
	var containerIDs []string

	// Parse arguments
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			fmt.Println(`Export logs for LLM analysis

USAGE:
    colog sdk export [OPTIONS]

OPTIONS:
    --format <format>     Output format: json, markdown (default: markdown)
    --output <file>       Output file (default: stdout)
    --tail <n>           Number of log lines per container (default: 100)
    --containers <ids>   Comma-separated container IDs (default: all running)
    --help, -h           Show this help message

EXAMPLES:
    colog sdk export --format json --output logs.json
    colog sdk export --containers abc123,def456 --tail 50
    colog sdk export --format markdown > analysis.md`)
			return nil
		case "--format":
			if i+1 < len(args) {
				format = args[i+1]
				i++
			}
		case "--output":
			if i+1 < len(args) {
				outputFile = args[i+1]
				i++
			}
		case "--tail":
			if i+1 < len(args) {
				if tail, err := strconv.Atoi(args[i+1]); err == nil {
					options.Tail = tail
					i++
				}
			}
		case "--containers":
			if i+1 < len(args) {
				containerIDs = strings.Split(args[i+1], ",")
				i++
			}
		}
	}

	ctx := context.Background()
	sdk, err := NewColog(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize SDK: %w", err)
	}
	defer sdk.Close()

	// If no specific containers specified, get all running containers
	if len(containerIDs) == 0 {
		containers, err := sdk.ListRunningContainers()
		if err != nil {
			return fmt.Errorf("failed to list containers: %w", err)
		}
		
		for _, container := range containers {
			containerIDs = append(containerIDs, container.ID)
		}
	}

	if len(containerIDs) == 0 {
		return fmt.Errorf("no containers found to export")
	}

	var output string
	switch strings.ToLower(format) {
	case "json":
		output, err = sdk.ExportLogsAsJSON(containerIDs, options)
	case "markdown", "md":
		output, err = sdk.ExportLogsAsMarkdown(containerIDs, options)
	default:
		return fmt.Errorf("unsupported format: %s (supported: json, markdown)", format)
	}

	if err != nil {
		return fmt.Errorf("failed to export logs: %w", err)
	}

	// Output to file or stdout
	if outputFile != "" {
		err = os.WriteFile(outputFile, []byte(output), 0644)
		if err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		fmt.Printf("Logs exported to %s (%s format, %d characters)\n", 
			outputFile, format, len(output))
	} else {
		fmt.Println(output)
	}

	return nil
}

func runFilterCommand(args []string) error {
	filter := ContainerFilter{}
	format := "table"

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			fmt.Println(`Filter containers by criteria

USAGE:
    colog sdk filter [OPTIONS]

OPTIONS:
    --name <pattern>      Filter by container name pattern
    --image <pattern>     Filter by image name pattern
    --image-id <id>       Filter by image ID
    --status <status>     Filter by container status
    --format <format>     Output format: table, json (default: table)
    --help, -h           Show this help message

EXAMPLES:
    colog sdk filter --image nginx            # Find nginx containers
    colog sdk filter --name web --status running
    colog sdk filter --format json`)
			return nil
		case "--name":
			if i+1 < len(args) {
				filter.Name = args[i+1]
				i++
			}
		case "--image":
			if i+1 < len(args) {
				filter.Image = args[i+1]
				i++
			}
		case "--image-id":
			if i+1 < len(args) {
				filter.ImageID = args[i+1]
				i++
			}
		case "--status":
			if i+1 < len(args) {
				filter.Status = args[i+1]
				i++
			}
		case "--format":
			if i+1 < len(args) {
				format = args[i+1]
				i++
			}
		}
	}

	ctx := context.Background()
	sdk, err := NewColog(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize SDK: %w", err)
	}
	defer sdk.Close()

	containers, err := sdk.FilterContainers(filter)
	if err != nil {
		return fmt.Errorf("failed to filter containers: %w", err)
	}

	if len(containers) == 0 {
		fmt.Println("No containers match the filter criteria")
		return nil
	}

	switch strings.ToLower(format) {
	case "json":
		jsonData, err := json.MarshalIndent(containers, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(jsonData))
	case "table":
		fmt.Printf("Found %d containers matching filter:\n\n", len(containers))
		fmt.Printf("%-12s %-20s %-30s %-15s\n", "ID", "NAME", "IMAGE", "STATUS")
		fmt.Println(strings.Repeat("-", 80))
		
		for _, container := range containers {
			id := container.ID
			if len(id) > 12 {
				id = id[:12]
			}
			name := container.Name
			if len(name) > 20 {
				name = name[:17] + "..."
			}
			image := container.Image
			if len(image) > 30 {
				image = image[:27] + "..."
			}
			status := container.Status
			if len(status) > 15 {
				status = status[:12] + "..."
			}
			
			fmt.Printf("%-12s %-20s %-30s %-15s\n", id, name, image, status)
		}
	default:
		return fmt.Errorf("unsupported format: %s (supported: table, json)", format)
	}

	return nil
}