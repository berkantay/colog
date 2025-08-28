# Colog SDK Documentation

The Colog SDK provides programmatic access to Docker container logs and information, making it easy to extract, filter, and format container data for analysis or integration with other systems, including LLM services.

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Core Concepts](#core-concepts)
- [API Reference](#api-reference)
- [Examples](#examples)
- [LLM Integration](#llm-integration)
- [Advanced Usage](#advanced-usage)

## Installation

The SDK is part of the main Colog package. To use it in your Go application:

```bash
go get github.com/berkantay/colog
```

Or if building from source:
```bash
git clone https://github.com/berkantay/colog.git
cd colog
go mod tidy
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"
)

func main() {
    // Initialize the SDK
    ctx := context.Background()
    sdk, err := NewSDK(ctx)
    if err != nil {
        log.Fatalf("Failed to initialize SDK: %v", err)
    }
    defer sdk.Close()

    // List running containers
    containers, err := sdk.ListRunningContainers()
    if err != nil {
        log.Fatalf("Failed to list containers: %v", err)
    }

    fmt.Printf("Found %d running containers\n", len(containers))
    
    // Export logs for LLM analysis
    if len(containers) > 0 {
        var containerIDs []string
        for _, container := range containers {
            containerIDs = append(containerIDs, container.ID)
        }
        
        markdown, err := sdk.ExportLogsAsMarkdown(containerIDs, LogOptions{
            Tail: 50,
        })
        if err != nil {
            log.Fatalf("Failed to export logs: %v", err)
        }
        
        fmt.Println("Logs exported for LLM analysis")
    }
}
```

## Core Concepts

### SDK Instance
The SDK provides a single entry point for all Docker operations:
```go
sdk, err := NewSDK(context.Background())
```

### Container Information
Containers are represented with detailed information:
```go
type ContainerInfo struct {
    ID        string            `json:"id"`
    Name      string            `json:"name"`
    Image     string            `json:"image"`
    ImageID   string            `json:"image_id"`
    Status    string            `json:"status"`
    State     string            `json:"state"`
    Created   time.Time         `json:"created"`
    Labels    map[string]string `json:"labels"`
    Ports     []PortMapping     `json:"ports"`
    Mounts    []MountInfo       `json:"mounts"`
    NetworkID string            `json:"network_id"`
}
```

### Log Options
Configure log retrieval behavior:
```go
type LogOptions struct {
    Follow     bool      `json:"follow"`      // Stream logs in real-time
    Tail       int       `json:"tail"`        // Number of recent log lines
    Since      time.Time `json:"since"`       // Show logs since this time
    Until      time.Time `json:"until"`       // Show logs until this time
    Timestamps bool      `json:"timestamps"`  // Include timestamps
}
```

### Container Filtering
Filter containers based on various criteria:
```go
type ContainerFilter struct {
    Name     string            `json:"name"`     // Container name pattern
    Image    string            `json:"image"`    // Image name pattern
    ImageID  string            `json:"image_id"` // Image ID pattern
    Status   string            `json:"status"`   // Status pattern
    Labels   map[string]string `json:"labels"`   // Label key-value pairs
    Networks []string          `json:"networks"` // Network names
}
```

## API Reference

### Container Management

#### `ListRunningContainers() ([]ContainerInfo, error)`
Returns all currently running containers.

#### `ListAllContainers() ([]ContainerInfo, error)`
Returns all containers (running and stopped).

#### `GetContainerByName(name string) (*ContainerInfo, error)`
Finds a specific container by name.

#### `GetContainerByID(id string) (*ContainerInfo, error)`
Finds a specific container by ID (supports both full and short IDs).

#### `FilterContainers(filter ContainerFilter) ([]ContainerInfo, error)`
Filters containers based on specified criteria.

#### `GetContainersByImage(image string) ([]ContainerInfo, error)`
Returns all containers using a specific image.

#### `GetContainersByImageID(imageID string) ([]ContainerInfo, error)`
Returns all containers using a specific image ID.

### Log Operations

#### `GetContainerLogs(containerID string, options LogOptions) ([]LogEntry, error)`
Retrieves logs from a single container.

#### `GetMultipleContainerLogs(containerIDs []string, options LogOptions) (map[string][]LogEntry, error)`
Retrieves logs from multiple containers simultaneously.

### LLM-Friendly Export

#### `ExportLogsForLLM(containerIDs []string, options LogOptions) (*LogsOutput, error)`
Exports logs in a structured format optimized for LLM analysis.

#### `ExportLogsAsJSON(containerIDs []string, options LogOptions) (string, error)`
Exports logs as formatted JSON string.

#### `ExportLogsAsMarkdown(containerIDs []string, options LogOptions) (string, error)`
Exports logs as formatted Markdown string.

## Examples

### Basic Container Listing
```go
containers, err := sdk.ListRunningContainers()
if err != nil {
    return err
}

for _, container := range containers {
    fmt.Printf("Container: %s (Image: %s, Status: %s)\n", 
        container.Name, container.Image, container.Status)
}
```

### Log Retrieval with Time Range
```go
since := time.Now().Add(-1 * time.Hour)
logs, err := sdk.GetContainerLogs("container_id", LogOptions{
    Since:      since,
    Timestamps: true,
    Tail:       100,
})
```

### Container Filtering
```go
filter := ContainerFilter{
    Image:  "nginx",
    Status: "running",
    Labels: map[string]string{
        "env": "production",
    },
}

containers, err := sdk.FilterContainers(filter)
```

### Bulk Log Export
```go
containerIDs := []string{"container1", "container2", "container3"}
logsMap, err := sdk.GetMultipleContainerLogs(containerIDs, LogOptions{
    Tail: 50,
})

for containerID, logs := range logsMap {
    fmt.Printf("Container %s has %d log entries\n", containerID, len(logs))
}
```

## LLM Integration

The SDK provides specialized functions for LLM integration:

### Structured Export
```go
output, err := sdk.ExportLogsForLLM(containerIDs, LogOptions{Tail: 100})
if err != nil {
    return err
}

// output contains:
// - GeneratedAt: timestamp
// - Containers: detailed container info with logs
// - Summary: aggregate statistics (error count, time range, etc.)
```

### Markdown Format for LLM Prompts
```go
markdown, err := sdk.ExportLogsAsMarkdown(containerIDs, LogOptions{Tail: 50})
if err != nil {
    return err
}

prompt := `Analyze these Docker logs and identify any issues:

` + markdown

// Send prompt to your LLM service
response := sendToLLM(prompt)
```

### JSON Format for Structured Analysis
```go
jsonData, err := sdk.ExportLogsAsJSON(containerIDs, LogOptions{Tail: 100})
if err != nil {
    return err
}

// Parse and send structured data to LLM
// The JSON includes metadata, summaries, and formatted logs
```

### Sample LLM Prompt Structure

The SDK generates markdown output optimized for LLM analysis:

```markdown
# Docker Container Logs Analysis

**Generated:** 2024-01-15 14:30:25 UTC
**Total Containers:** 3
**Total Log Entries:** 150
**Error Count:** 5
**Top Images:** nginx, redis, postgresql

---

## Container: web-server

- **ID:** abc123def456
- **Image:** nginx:latest
- **Status:** running
- **Log Entries:** 50

### Logs

```
[2024-01-15 14:25:15] Starting nginx server
[2024-01-15 14:25:16] Listening on port 80
...
```
```

## Advanced Usage

### Real-time Monitoring
```go
func monitorContainers(ctx context.Context, sdk *SDK) {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            containers, _ := sdk.ListRunningContainers()
            
            var containerIDs []string
            for _, c := range containers {
                containerIDs = append(containerIDs, c.ID)
            }
            
            output, err := sdk.ExportLogsForLLM(containerIDs, LogOptions{
                Tail: 100,
            })
            if err != nil {
                continue
            }
            
            // Send to monitoring system or LLM for analysis
            if output.Summary.ErrorCount > threshold {
                alertSystem(output)
            }
        }
    }
}
```

### Custom Log Analysis
```go
func analyzeErrors(logs []LogEntry) map[string]int {
    errorPatterns := make(map[string]int)
    
    for _, log := range logs {
        message := strings.ToLower(log.Message)
        if strings.Contains(message, "error") {
            errorPatterns["error"]++
        }
        if strings.Contains(message, "exception") {
            errorPatterns["exception"]++
        }
        if strings.Contains(message, "timeout") {
            errorPatterns["timeout"]++
        }
    }
    
    return errorPatterns
}
```

### Batch Processing
```go
func processAllContainers(sdk *SDK) error {
    containers, err := sdk.ListAllContainers()
    if err != nil {
        return err
    }

    // Process containers in batches of 10
    batchSize := 10
    for i := 0; i < len(containers); i += batchSize {
        end := i + batchSize
        if end > len(containers) {
            end = len(containers)
        }
        
        batch := containers[i:end]
        var batchIDs []string
        for _, c := range batch {
            batchIDs = append(batchIDs, c.ID)
        }
        
        logsMap, err := sdk.GetMultipleContainerLogs(batchIDs, LogOptions{
            Tail: 100,
        })
        if err != nil {
            log.Printf("Failed to process batch: %v", err)
            continue
        }
        
        // Process batch results
        processBatch(logsMap)
    }
    
    return nil
}
```

## Error Handling

The SDK provides comprehensive error handling:

```go
logs, err := sdk.GetContainerLogs(containerID, options)
if err != nil {
    if strings.Contains(err.Error(), "not found") {
        // Container doesn't exist
        log.Printf("Container %s not found", containerID)
    } else if strings.Contains(err.Error(), "permission denied") {
        // Docker access issues
        log.Printf("Permission denied accessing Docker")
    } else {
        // Other errors
        log.Printf("Unexpected error: %v", err)
    }
}
```

## Performance Considerations

- **Batch Operations**: Use `GetMultipleContainerLogs()` for better performance when retrieving logs from multiple containers
- **Log Limits**: Use the `Tail` option to limit log retrieval and avoid memory issues
- **Context Cancellation**: Always use context for cancellation support in long-running operations
- **Resource Cleanup**: Always call `sdk.Close()` to release Docker client connections

## License

This SDK is part of Colog and is released under the MIT License.