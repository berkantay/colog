package main

import (
	"context"
	"fmt"
	"log"
	"time"
)

// This example demonstrates how to use the Colog SDK
func main() {
	// Create a context
	ctx := context.Background()

	// Initialize the SDK
	sdk, err := NewSDK(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize SDK: %v", err)
	}
	defer sdk.Close()

	// Example 1: List all running containers
	fmt.Println("=== Example 1: List Running Containers ===")
	containers, err := sdk.ListRunningContainers()
	if err != nil {
		log.Printf("Failed to list containers: %v", err)
		return
	}

	for _, container := range containers {
		fmt.Printf("Container: %s (ID: %s, Image: %s)\n", 
			container.Name, container.ID, container.Image)
	}

	if len(containers) == 0 {
		fmt.Println("No running containers found")
		return
	}

	// Example 2: Get containers by image
	fmt.Println("\n=== Example 2: Filter Containers by Image ===")
	nginxContainers, err := sdk.GetContainersByImage("nginx")
	if err != nil {
		log.Printf("Failed to get nginx containers: %v", err)
	} else {
		fmt.Printf("Found %d nginx containers\n", len(nginxContainers))
		for _, container := range nginxContainers {
			fmt.Printf("  - %s (%s)\n", container.Name, container.ID)
		}
	}

	// Example 3: Get logs from a specific container (last 10 entries)
	fmt.Println("\n=== Example 3: Get Container Logs ===")
	if len(containers) > 0 {
		firstContainer := containers[0]
		fmt.Printf("Getting logs from container: %s\n", firstContainer.Name)

		logs, err := sdk.GetContainerLogs(firstContainer.ID, LogOptions{
			Tail:       10,
			Follow:     false,
			Timestamps: true,
		})
		if err != nil {
			log.Printf("Failed to get logs: %v", err)
		} else {
			fmt.Printf("Retrieved %d log entries:\n", len(logs))
			for _, logEntry := range logs {
				fmt.Printf("  [%s] %s\n", 
					logEntry.Timestamp.Format("15:04:05"), logEntry.Message)
			}
		}
	}

	// Example 4: Get logs from multiple containers
	fmt.Println("\n=== Example 4: Get Logs from Multiple Containers ===")
	containerIDs := make([]string, 0)
	for i, container := range containers {
		if i < 3 { // Limit to first 3 containers
			containerIDs = append(containerIDs, container.ID)
		}
	}

	if len(containerIDs) > 0 {
		logsMap, err := sdk.GetMultipleContainerLogs(containerIDs, LogOptions{
			Tail:   5,
			Follow: false,
		})
		if err != nil {
			log.Printf("Failed to get multiple container logs: %v", err)
		} else {
			fmt.Printf("Retrieved logs from %d containers:\n", len(logsMap))
			for containerID, logs := range logsMap {
				fmt.Printf("  Container %s: %d log entries\n", containerID[:12], len(logs))
			}
		}
	}

	// Example 5: Export logs for LLM analysis (JSON format)
	fmt.Println("\n=== Example 5: Export Logs for LLM (JSON) ===")
	if len(containerIDs) > 0 {
		jsonLogs, err := sdk.ExportLogsAsJSON(containerIDs, LogOptions{
			Tail:   20,
			Follow: false,
		})
		if err != nil {
			log.Printf("Failed to export logs as JSON: %v", err)
		} else {
			fmt.Printf("JSON export size: %d characters\n", len(jsonLogs))
			// Optionally save to file or send to LLM
		}
	}

	// Example 6: Export logs for LLM analysis (Markdown format)
	fmt.Println("\n=== Example 6: Export Logs for LLM (Markdown) ===")
	if len(containerIDs) > 0 {
		markdownLogs, err := sdk.ExportLogsAsMarkdown(containerIDs, LogOptions{
			Tail:   15,
			Follow: false,
		})
		if err != nil {
			log.Printf("Failed to export logs as Markdown: %v", err)
		} else {
			fmt.Printf("Markdown export size: %d characters\n", len(markdownLogs))
			fmt.Println("Preview:")
			// Print first 500 characters as preview
			if len(markdownLogs) > 500 {
				fmt.Printf("%s...\n", markdownLogs[:500])
			} else {
				fmt.Println(markdownLogs)
			}
		}
	}

	// Example 7: Advanced filtering
	fmt.Println("\n=== Example 7: Advanced Container Filtering ===")
	filter := ContainerFilter{
		Status: "running",
		// You can add more filters like:
		// Labels: map[string]string{"env": "production"},
		// Image: "nginx",
	}

	filteredContainers, err := sdk.FilterContainers(filter)
	if err != nil {
		log.Printf("Failed to filter containers: %v", err)
	} else {
		fmt.Printf("Found %d containers matching filter\n", len(filteredContainers))
	}

	// Example 8: Get logs within a time range
	fmt.Println("\n=== Example 8: Get Logs within Time Range ===")
	if len(containers) > 0 {
		since := time.Now().Add(-1 * time.Hour) // Last hour
		logs, err := sdk.GetContainerLogs(containers[0].ID, LogOptions{
			Since:      since,
			Timestamps: true,
		})
		if err != nil {
			log.Printf("Failed to get time-filtered logs: %v", err)
		} else {
			fmt.Printf("Found %d log entries in the last hour\n", len(logs))
		}
	}

	fmt.Println("\n=== SDK Usage Examples Complete ===")
}

// Example of using the SDK in a monitoring application
func monitoringExample() {
	ctx := context.Background()
	sdk, err := NewSDK(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize SDK: %v", err)
	}
	defer sdk.Close()

	// Monitor all containers and export logs every 5 minutes for analysis
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			containers, err := sdk.ListRunningContainers()
			if err != nil {
				log.Printf("Failed to list containers: %v", err)
				continue
			}

			var containerIDs []string
			for _, container := range containers {
				containerIDs = append(containerIDs, container.ID)
			}

			// Export logs for LLM analysis
			logsOutput, err := sdk.ExportLogsForLLM(containerIDs, LogOptions{
				Tail:   50,
				Follow: false,
			})
			if err != nil {
				log.Printf("Failed to export logs: %v", err)
				continue
			}

			// Here you could send logsOutput to your LLM for analysis
			fmt.Printf("Exported logs from %d containers (%d total log entries)\n",
				logsOutput.Summary.TotalContainers,
				logsOutput.Summary.TotalLogs)

			// Check for high error counts and alert
			if logsOutput.Summary.ErrorCount > 10 {
				fmt.Printf("WARNING: High error count detected: %d errors\n",
					logsOutput.Summary.ErrorCount)
			}
		}
	}
}

// Example of integrating with an LLM service
func llmIntegrationExample() {
	ctx := context.Background()
	sdk, err := NewSDK(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize SDK: %v", err)
	}
	defer sdk.Close()

	// Get all running containers
	containers, err := sdk.ListRunningContainers()
	if err != nil {
		log.Printf("Failed to list containers: %v", err)
		return
	}

	var containerIDs []string
	for _, container := range containers {
		containerIDs = append(containerIDs, container.ID)
	}

	// Export logs in markdown format for LLM consumption
	markdownLogs, err := sdk.ExportLogsAsMarkdown(containerIDs, LogOptions{
		Tail:       100,
		Follow:     false,
		Timestamps: true,
	})
	if err != nil {
		log.Printf("Failed to export logs: %v", err)
		return
	}

	// Create a prompt for the LLM
	prompt := `Please analyze the following Docker container logs and provide insights:

1. Identify any errors or warning patterns
2. Suggest potential issues or optimizations
3. Summarize the overall health of the system
4. Highlight any security concerns

Here are the logs:

` + markdownLogs

	fmt.Printf("Generated prompt for LLM (%d characters):\n", len(prompt))
	fmt.Println("---")
	// In a real implementation, you would send this prompt to your LLM service
	// Example: response := sendToLLM(prompt)
	
	// For demonstration, just show a portion of the prompt
	if len(prompt) > 1000 {
		fmt.Printf("%s...\n", prompt[:1000])
	} else {
		fmt.Println(prompt)
	}
}