package main

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

type Container struct {
	ID     string
	Name   string
	Image  string
	Status string
}

type DockerService struct {
	client *client.Client
}

func NewDockerService() (*DockerService, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	
	return &DockerService{
		client: cli,
	}, nil
}

func (ds *DockerService) Close() error {
	return ds.client.Close()
}

func (ds *DockerService) ListRunningContainers(ctx context.Context) ([]Container, error) {
	containers, err := ds.client.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return nil, err
	}

	var result []Container
	for _, ctr := range containers {
		name := strings.TrimPrefix(ctr.Names[0], "/")
		result = append(result, Container{
			ID:     ctr.ID[:12],
			Name:   name,
			Image:  ctr.Image,
			Status: ctr.Status,
		})
	}

	return result, nil
}

func (ds *DockerService) StreamLogs(ctx context.Context, containerID string, logCh chan<- LogEntry) error {
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: true,
		Tail:       "100", // Show last 100 lines of historical logs
	}

	logs, err := ds.client.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return err
	}

	go func() {
		defer close(logCh)
		defer logs.Close()
		
		// Use io.Reader directly instead of bufio.Reader to handle binary data better
		buf := make([]byte, 4096)
		
		for {
			select {
			case <-ctx.Done():
				return
			default:
				n, err := logs.Read(buf)
				if err != nil {
					if err == io.EOF {
						time.Sleep(100 * time.Millisecond)
						continue
					}
					return
				}
				
				if n > 0 {
					// Process the raw data
					data := string(buf[:n])
					lines := strings.Split(data, "\n")
					
					for _, line := range lines {
						if len(line) == 0 {
							continue
						}
						
						entry := parseLogEntry(containerID, line)
						if entry.Message != "" {
							select {
							case logCh <- entry:
							case <-ctx.Done():
								return
							}
						}
					}
				}
			}
		}
	}()

	return nil
}

type LogEntry struct {
	ContainerID string
	Timestamp   time.Time
	Message     string
	Stream      string
}

func parseLogEntry(containerID, line string) LogEntry {
	if len(line) == 0 {
		return LogEntry{}
	}

	// Docker logs have an 8-byte header when using the multiplexed format:
	// [0-3]: stream type (1=stdout, 2=stderr) + padding
	// [4-7]: payload length (big-endian uint32)
	// [8+]:  actual log content
	
	originalLine := line
	
	// Check if this line has the Docker multiplexed header
	if len(line) >= 8 {
		// First byte indicates stream type: 1 for stdout, 2 for stderr
		firstByte := byte(line[0])
		if firstByte == 1 || firstByte == 2 {
			// Skip the 8-byte header
			line = line[8:]
		}
	}
	
	// Trim whitespace after header removal
	line = strings.TrimSpace(line)
	if len(line) == 0 {
		return LogEntry{}
	}
	
	// Try to parse timestamp if present
	parts := strings.SplitN(line, " ", 2)
	var timestamp time.Time
	var message string
	
	if len(parts) >= 2 {
		// Try multiple timestamp formats
		timestampFormats := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02T15:04:05.000000000Z",
			"2006-01-02T15:04:05.000Z",
		}
		
		parsed := false
		for _, format := range timestampFormats {
			if ts, err := time.Parse(format, parts[0]); err == nil {
				timestamp = ts
				message = parts[1]
				parsed = true
				break
			}
		}
		
		if !parsed {
			// No valid timestamp found, treat entire line as message
			timestamp = time.Now()
			message = line
		}
	} else {
		timestamp = time.Now()
		message = line
	}

	// If message is still empty, use the original line as fallback
	if message == "" {
		message = strings.TrimSpace(originalLine)
	}

	return LogEntry{
		ContainerID: containerID,
		Timestamp:   timestamp,
		Message:     message,
		Stream:      "stdout",
	}
}