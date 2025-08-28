package main

import (
	"bufio"
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
		Tail:       "all", // Show all historical logs
	}

	logs, err := ds.client.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return err
	}
	defer logs.Close()

	go func() {
		defer close(logCh)
		reader := bufio.NewReader(logs)
		
		for {
			select {
			case <-ctx.Done():
				return
			default:
				line, err := reader.ReadString('\n')
				if err != nil {
					if err == io.EOF {
						time.Sleep(100 * time.Millisecond)
						continue
					}
					return
				}
				
				entry := parseLogEntry(containerID, line)
				if entry.Message != "" {
					logCh <- entry
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
	line = strings.TrimSpace(line)
	if len(line) == 0 {
		return LogEntry{}
	}

	// Docker logs may have an 8-byte header (stream type + length)
	// Check if this looks like a Docker log header by examining the first byte
	if len(line) >= 8 {
		firstByte := line[0]
		// Docker log headers start with 0, 1, or 2 (stdin, stdout, stderr)
		if firstByte <= 2 {
			line = line[8:]
		}
	}
	
	parts := strings.SplitN(line, " ", 2)
	var timestamp time.Time
	var message string
	
	if len(parts) >= 2 {
		if ts, err := time.Parse(time.RFC3339Nano, parts[0]); err == nil {
			timestamp = ts
			message = parts[1]
		} else {
			timestamp = time.Now()
			message = line
		}
	} else {
		timestamp = time.Now()
		message = line
	}

	return LogEntry{
		ContainerID: containerID,
		Timestamp:   timestamp,
		Message:     message,
		Stream:      "stdout",
	}
}