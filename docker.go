package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
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

type DockerEndpoint struct {
	Name        string
	Description string
	Host        string
	IsDefault   bool
	Available   bool
}

func NewDockerService() (*DockerService, error) {
	return NewDockerServiceWithSelection(true)
}

func NewDockerServiceWithSelection(interactive bool) (*DockerService, error) {
	endpoints := discoverDockerEndpoints()
	
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no Docker endpoints found")
	}
	
	// Filter only available endpoints
	var availableEndpoints []DockerEndpoint
	for _, endpoint := range endpoints {
		if endpoint.Available {
			availableEndpoints = append(availableEndpoints, endpoint)
		}
	}
	
	if len(availableEndpoints) == 0 {
		return nil, fmt.Errorf("no available Docker endpoints found")
	}
	
	var selectedEndpoint DockerEndpoint
	
	if interactive && len(availableEndpoints) > 1 {
		selectedEndpoint = selectDockerEndpoint(availableEndpoints)
	} else {
		// Use the first available endpoint (prefer default if available)
		selectedEndpoint = availableEndpoints[0]
		for _, endpoint := range availableEndpoints {
			if endpoint.IsDefault {
				selectedEndpoint = endpoint
				break
			}
		}
	}
	
	return connectToDockerEndpoint(selectedEndpoint)
}

func discoverDockerEndpoints() []DockerEndpoint {
	var endpoints []DockerEndpoint
	
	// Get current Docker context
	currentContext := getCurrentDockerContext()
	
	// Get Docker contexts from `docker context ls`
	contextEndpoints := getDockerContexts()
	endpoints = append(endpoints, contextEndpoints...)
	
	// Add common socket paths that might not be in contexts
	socketPaths := []struct {
		name        string
		description string
		path        string
	}{
		{"orbstack", "OrbStack", os.Getenv("HOME") + "/.orbstack/run/docker.sock"},
		{"docker-desktop", "Docker Desktop", os.Getenv("HOME") + "/.docker/run/docker.sock"},
		{"docker-daemon", "Standard Docker", "/var/run/docker.sock"},
	}
	
	// Check if socket paths are already covered by contexts
	existingHosts := make(map[string]bool)
	for _, endpoint := range endpoints {
		existingHosts[endpoint.Host] = true
	}
	
	for _, socket := range socketPaths {
		host := "unix://" + socket.path
		if existingHosts[host] {
			continue // Already covered by context
		}
		
		if _, err := os.Stat(socket.path); err == nil {
			endpoint := DockerEndpoint{
				Name:        socket.name,
				Description: socket.description,
				Host:        host,
				IsDefault:   socket.name == currentContext,
				Available:   testDockerConnection(host),
			}
			endpoints = append(endpoints, endpoint)
		}
	}
	
	return endpoints
}

func getCurrentDockerContext() string {
	cmd := exec.Command("docker", "context", "show")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func getDockerContexts() []DockerEndpoint {
	var endpoints []DockerEndpoint
	
	cmd := exec.Command("docker", "context", "ls", "--format", "{{.Name}}\t{{.Description}}\t{{.DockerEndpoint}}\t{{.Current}}")
	output, err := cmd.Output()
	if err != nil {
		return endpoints
	}
	
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		parts := strings.Split(line, "\t")
		if len(parts) < 4 {
			continue
		}
		
		name := parts[0]
		description := parts[1]
		host := parts[2]
		isCurrent := strings.Contains(parts[3], "*")
		
		endpoint := DockerEndpoint{
			Name:        name,
			Description: description,
			Host:        host,
			IsDefault:   isCurrent,
			Available:   testDockerConnection(host),
		}
		endpoints = append(endpoints, endpoint)
	}
	
	return endpoints
}

func testDockerConnection(host string) bool {
	cli, err := client.NewClientWithOpts(
		client.WithHost(host),
		client.WithAPIVersionNegotiation(),
		client.WithTimeout(2*time.Second),
	)
	if err != nil {
		return false
	}
	defer cli.Close()
	
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	_, err = cli.Ping(ctx)
	return err == nil
}

func selectDockerEndpoint(endpoints []DockerEndpoint) DockerEndpoint {
	fmt.Println("\nMultiple Docker endpoints found:")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	
	for i, endpoint := range endpoints {
		status := "✓ Available"
		if !endpoint.Available {
			status = "✗ Unavailable"
		}
		
		defaultMarker := ""
		if endpoint.IsDefault {
			defaultMarker = " (current context)"
		}
		
		fmt.Printf("%d. %s%s\n", i+1, endpoint.Name, defaultMarker)
		fmt.Printf("   %s - %s\n", endpoint.Description, status)
		fmt.Printf("   Host: %s\n", endpoint.Host)
		fmt.Println()
	}
	
	fmt.Print("Select Docker endpoint (1-", len(endpoints), ") [default: 1]: ")
	
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			return endpoints[0]
		}
		
		if choice, err := strconv.Atoi(input); err == nil && choice >= 1 && choice <= len(endpoints) {
			return endpoints[choice-1]
		}
	}
	
	// Default to first endpoint if invalid input
	return endpoints[0]
}

func connectToDockerEndpoint(endpoint DockerEndpoint) (*DockerService, error) {
	cli, err := client.NewClientWithOpts(
		client.WithHost(endpoint.Host),
		client.WithAPIVersionNegotiation(),
		client.WithTimeout(5*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client for %s: %w", endpoint.Name, err)
	}
	
	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if _, err := cli.Ping(ctx); err != nil {
		cli.Close()
		return nil, fmt.Errorf("failed to connect to Docker endpoint %s: %w", endpoint.Name, err)
	}
	
	fmt.Printf("✓ Connected to Docker via %s (%s)\n", endpoint.Name, endpoint.Description)
	return &DockerService{client: cli}, nil
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
	// Use docker command directly - we know this works!
	cmd := exec.Command("docker", "logs", "-f", "--timestamps", "--tail", "100", containerID)
	
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	
	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		defer close(logCh)
		defer stdout.Close()
		defer cmd.Process.Kill()
		
		scanner := bufio.NewScanner(stdout)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
				line := scanner.Text()
				if line != "" {
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