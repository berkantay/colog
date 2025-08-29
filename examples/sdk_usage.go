package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/berkantay/colog/pkg/colog"
)

// This example demonstrates how to use the Colog SDK
func main() {
	// Create a context
	ctx := context.Background()

	// Initialize the SDK with automatic endpoint selection
	dockerService, err := colog.NewDockerService()
	if err != nil {
		log.Fatalf("Failed to initialize SDK: %v", err)
	}
	defer dockerService.Close()

	// Example 1: List all running containers
	fmt.Println("=== Example 1: List Running Containers ===")
	containers, err := dockerService.ListRunningContainers(ctx)
	if err != nil {
		log.Printf("Failed to list containers: %v", err)
		return
	}

	fmt.Printf("Found %d running containers:\n", len(containers))
	for _, container := range containers {
		fmt.Printf("- %s (%s) - %s\n", container.Name, container.ID, container.Status)
	}

	if len(containers) == 0 {
		fmt.Println("No running containers found. Start a container first:")
		fmt.Println("  docker run -d --name test-container nginx")
		return
	}

	// Example 2: Stream logs from the first container
	fmt.Printf("\n=== Example 2: Stream Logs from %s ===\n", containers[0].Name)
	
	logCh := make(chan colog.LogEntry, 100)
	go func() {
		err := dockerService.StreamLogs(ctx, containers[0].ID, logCh)
		if err != nil {
			fmt.Printf("Error streaming logs: %v\n", err)
		}
	}()

	// Read logs for 5 seconds
	timeout := time.After(5 * time.Second)
	logCount := 0
	maxLogs := 10

LogLoop:
	for {
		select {
		case entry, ok := <-logCh:
			if !ok {
				fmt.Println("Log stream closed")
				break LogLoop
			}
			fmt.Printf("[%s] %s\n", entry.Timestamp.Format("15:04:05"), entry.Message)
			logCount++
			if logCount >= maxLogs {
				fmt.Printf("... (showing first %d logs)\n", maxLogs)
				break LogLoop
			}
		case <-timeout:
			fmt.Println("Timeout reached, stopping log stream...")
			break LogLoop
		}
	}

	fmt.Println("\n✅ SDK example completed successfully!")
	fmt.Println("\nFor interactive endpoint selection, use:")
	fmt.Println("  dockerService, err := colog.NewDockerServiceInteractive()")
}