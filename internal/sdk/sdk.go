package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/berkantay/colog/internal/docker"
)

// Colog provides programmatic access to Docker container logs and information
type Colog struct {
	dockerService *docker.DockerService
	ctx           context.Context
}

// ContainerInfo represents detailed container information
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

// PortMapping represents container port information
type PortMapping struct {
	ContainerPort int    `json:"container_port"`
	HostPort      int    `json:"host_port"`
	Type          string `json:"type"`
	HostIP        string `json:"host_ip"`
}

// MountInfo represents container mount information
type MountInfo struct {
	Type        string `json:"type"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Mode        string `json:"mode"`
	RW          bool   `json:"rw"`
}

// LogOptions configures log retrieval behavior
type LogOptions struct {
	Follow     bool      `json:"follow"`
	Tail       int       `json:"tail"`
	Since      time.Time `json:"since"`
	Until      time.Time `json:"until"`
	Timestamps bool      `json:"timestamps"`
}

// ContainerFilter defines criteria for filtering containers
type ContainerFilter struct {
	Name     string            `json:"name"`
	Image    string            `json:"image"`
	ImageID  string            `json:"image_id"`
	Status   string            `json:"status"`
	Labels   map[string]string `json:"labels"`
	Networks []string          `json:"networks"`
}

// LogsOutput represents formatted logs for LLM consumption
type LogsOutput struct {
	GeneratedAt time.Time                `json:"generated_at"`
	Containers  []ContainerLogCollection `json:"containers"`
	Summary     LogsSummary              `json:"summary"`
}

// ContainerLogCollection represents logs from a single container
type ContainerLogCollection struct {
	Container ContainerInfo `json:"container"`
	LogCount  int           `json:"log_count"`
	Logs      []docker.LogEntry    `json:"logs"`
	TimeRange TimeRange     `json:"time_range"`
}

// TimeRange represents the time span of logs
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// LogsSummary provides aggregate information about logs
type LogsSummary struct {
	TotalContainers int       `json:"total_containers"`
	TotalLogs       int       `json:"total_logs"`
	TimeRange       TimeRange `json:"time_range"`
	TopImages       []string  `json:"top_images"`
	ErrorCount      int       `json:"error_count"`
}

// NewColog creates a new Colog SDK instance
func NewColog(ctx context.Context) (*Colog, error) {
	dockerService, err := docker.NewDockerService()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Docker service: %w", err)
	}

	return &Colog{
		dockerService: dockerService,
		ctx:           ctx,
	}, nil
}

// Close releases Colog resources
func (c *Colog) Close() error {
	return c.dockerService.Close()
}

// ListAllContainers returns all containers (running and stopped)
func (c *Colog) ListAllContainers() ([]ContainerInfo, error) {
	return c.listContainers(true)
}

// ListRunningContainers returns only running containers
func (c *Colog) ListRunningContainers() ([]ContainerInfo, error) {
	return c.listContainers(false)
}

// GetContainerByName finds a container by name
func (c *Colog) GetContainerByName(name string) (*ContainerInfo, error) {
	containers, err := c.ListAllContainers()
	if err != nil {
		return nil, err
	}

	for _, container := range containers {
		if container.Name == name {
			return &container, nil
		}
	}

	return nil, fmt.Errorf("container with name '%s' not found", name)
}

// GetContainerByID finds a container by ID (full or short)
func (c *Colog) GetContainerByID(id string) (*ContainerInfo, error) {
	containers, err := c.ListAllContainers()
	if err != nil {
		return nil, err
	}

	for _, container := range containers {
		if container.ID == id || strings.HasPrefix(container.ID, id) {
			return &container, nil
		}
	}

	return nil, fmt.Errorf("container with ID '%s' not found", id)
}

// FilterContainers filters containers based on criteria
func (c *Colog) FilterContainers(filter ContainerFilter) ([]ContainerInfo, error) {
	containers, err := c.ListAllContainers()
	if err != nil {
		return nil, err
	}

	var filtered []ContainerInfo
	for _, container := range containers {
		if c.matchesFilter(container, filter) {
			filtered = append(filtered, container)
		}
	}

	return filtered, nil
}

// GetContainersByImage returns containers using a specific image
func (c *Colog) GetContainersByImage(image string) ([]ContainerInfo, error) {
	return c.FilterContainers(ContainerFilter{Image: image})
}

// GetContainersByImageID returns containers using a specific image ID
func (c *Colog) GetContainersByImageID(imageID string) ([]ContainerInfo, error) {
	return c.FilterContainers(ContainerFilter{ImageID: imageID})
}

// GetContainerLogs retrieves logs from a specific container
func (c *Colog) GetContainerLogs(containerID string, options LogOptions) ([]docker.LogEntry, error) {
	logCh := make(chan docker.LogEntry, 1000)
	logs := make([]docker.LogEntry, 0)

	// Create a context for log streaming
	ctx := c.ctx
	if !options.Follow {
		// For non-following requests, use a shorter timeout
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(c.ctx, 30*time.Second)
		defer cancel()
	}

	err := c.dockerService.StreamLogs(ctx, containerID, logCh)
	if err != nil {
		return nil, fmt.Errorf("failed to stream logs: %w", err)
	}

	// Collect logs
	for {
		select {
		case <-ctx.Done():
			return logs, nil
		case entry, ok := <-logCh:
			if !ok {
				return logs, nil
			}

			// Apply filters
			if !options.Since.IsZero() && entry.Timestamp.Before(options.Since) {
				continue
			}
			if !options.Until.IsZero() && entry.Timestamp.After(options.Until) {
				continue
			}

			logs = append(logs, entry)

			// Limit to tail count for non-following requests
			if !options.Follow && options.Tail > 0 && len(logs) >= options.Tail {
				// Keep only the last N entries
				if len(logs) > options.Tail {
					logs = logs[len(logs)-options.Tail:]
				}
				return logs, nil
			}
		}
	}
}

// GetMultipleContainerLogs retrieves logs from multiple containers
func (c *Colog) GetMultipleContainerLogs(containerIDs []string, options LogOptions) (map[string][]docker.LogEntry, error) {
	result := make(map[string][]docker.LogEntry)
	
	for _, containerID := range containerIDs {
		logs, err := c.GetContainerLogs(containerID, options)
		if err != nil {
			// Log error but continue with other containers
			result[containerID] = []docker.LogEntry{{
				ContainerID: containerID,
				Timestamp:   time.Now(),
				Message:     fmt.Sprintf("Error retrieving logs: %v", err),
				Stream:      "error",
			}}
			continue
		}
		result[containerID] = logs
	}

	return result, nil
}

// ExportLogsForLLM formats logs for LLM consumption
func (c *Colog) ExportLogsForLLM(containerIDs []string, options LogOptions) (*LogsOutput, error) {
	logsMap, err := c.GetMultipleContainerLogs(containerIDs, options)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve logs: %w", err)
	}

	containers, err := c.ListAllContainers()
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	// Create container lookup map
	containerLookup := make(map[string]ContainerInfo)
	for _, container := range containers {
		containerLookup[container.ID] = container
	}

	output := &LogsOutput{
		GeneratedAt: time.Now(),
		Containers:  make([]ContainerLogCollection, 0),
	}

	var allLogs []docker.LogEntry
	imageCount := make(map[string]int)
	errorCount := 0

	for containerID, logs := range logsMap {
		container, exists := containerLookup[containerID]
		if !exists {
			// Create minimal container info if not found
			container = ContainerInfo{
				ID:   containerID,
				Name: "unknown",
			}
		}

		var timeRange TimeRange
		if len(logs) > 0 {
			timeRange.Start = logs[0].Timestamp
			timeRange.End = logs[len(logs)-1].Timestamp
		}

		// Count errors in logs
		for _, log := range logs {
			if strings.Contains(strings.ToLower(log.Message), "error") ||
				strings.Contains(strings.ToLower(log.Message), "exception") ||
				strings.Contains(strings.ToLower(log.Message), "fail") {
				errorCount++
			}
		}

		collection := ContainerLogCollection{
			Container: container,
			LogCount:  len(logs),
			Logs:      logs,
			TimeRange: timeRange,
		}

		output.Containers = append(output.Containers, collection)
		allLogs = append(allLogs, logs...)
		imageCount[container.Image]++
	}

	// Generate summary
	var overallTimeRange TimeRange
	if len(allLogs) > 0 {
		// Sort logs by timestamp to find overall range
		sort.Slice(allLogs, func(i, j int) bool {
			return allLogs[i].Timestamp.Before(allLogs[j].Timestamp)
		})
		overallTimeRange.Start = allLogs[0].Timestamp
		overallTimeRange.End = allLogs[len(allLogs)-1].Timestamp
	}

	// Get top images
	type imageInfo struct {
		name  string
		count int
	}
	var imageInfos []imageInfo
	for image, count := range imageCount {
		imageInfos = append(imageInfos, imageInfo{image, count})
	}
	sort.Slice(imageInfos, func(i, j int) bool {
		return imageInfos[i].count > imageInfos[j].count
	})

	topImages := make([]string, 0)
	for i, info := range imageInfos {
		if i >= 5 { // Top 5 images
			break
		}
		topImages = append(topImages, info.name)
	}

	output.Summary = LogsSummary{
		TotalContainers: len(output.Containers),
		TotalLogs:       len(allLogs),
		TimeRange:       overallTimeRange,
		TopImages:       topImages,
		ErrorCount:      errorCount,
	}

	return output, nil
}

// ExportLogsAsJSON exports logs as JSON string
func (c *Colog) ExportLogsAsJSON(containerIDs []string, options LogOptions) (string, error) {
	output, err := c.ExportLogsForLLM(containerIDs, options)
	if err != nil {
		return "", err
	}

	jsonData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return string(jsonData), nil
}

// ExportLogsAsMarkdown exports logs as markdown string for LLM consumption
func (c *Colog) ExportLogsAsMarkdown(containerIDs []string, options LogOptions) (string, error) {
	output, err := c.ExportLogsForLLM(containerIDs, options)
	if err != nil {
		return "", err
	}

	var md strings.Builder
	
	md.WriteString("# Docker Container Logs Analysis\n\n")
	md.WriteString(fmt.Sprintf("**Generated:** %s\n", output.GeneratedAt.Format("2006-01-02 15:04:05 MST")))
	md.WriteString(fmt.Sprintf("**Total Containers:** %d\n", output.Summary.TotalContainers))
	md.WriteString(fmt.Sprintf("**Total Log Entries:** %d\n", output.Summary.TotalLogs))
	md.WriteString(fmt.Sprintf("**Error Count:** %d\n", output.Summary.ErrorCount))
	
	if len(output.Summary.TopImages) > 0 {
		md.WriteString(fmt.Sprintf("**Top Images:** %s\n", strings.Join(output.Summary.TopImages, ", ")))
	}
	
	if !output.Summary.TimeRange.Start.IsZero() {
		md.WriteString(fmt.Sprintf("**Time Range:** %s to %s\n", 
			output.Summary.TimeRange.Start.Format("2006-01-02 15:04:05"),
			output.Summary.TimeRange.End.Format("2006-01-02 15:04:05")))
	}
	
	md.WriteString("\n---\n\n")

	for _, collection := range output.Containers {
		md.WriteString(fmt.Sprintf("## Container: %s\n\n", collection.Container.Name))
		md.WriteString(fmt.Sprintf("- **ID:** %s\n", collection.Container.ID))
		md.WriteString(fmt.Sprintf("- **Image:** %s\n", collection.Container.Image))
		md.WriteString(fmt.Sprintf("- **Status:** %s\n", collection.Container.Status))
		md.WriteString(fmt.Sprintf("- **Log Entries:** %d\n", collection.LogCount))
		
		if !collection.TimeRange.Start.IsZero() {
			md.WriteString(fmt.Sprintf("- **Log Time Range:** %s to %s\n", 
				collection.TimeRange.Start.Format("2006-01-02 15:04:05"),
				collection.TimeRange.End.Format("2006-01-02 15:04:05")))
		}
		
		md.WriteString("\n### Logs\n\n```\n")
		for _, log := range collection.Logs {
			timestamp := log.Timestamp.Format("2006-01-02 15:04:05")
			md.WriteString(fmt.Sprintf("[%s] %s\n", timestamp, log.Message))
		}
		md.WriteString("```\n\n")
	}

	return md.String(), nil
}

// Helper methods

func (c *Colog) listContainers(all bool) ([]ContainerInfo, error) {
	// This is a simplified version - in a full implementation, you'd need to expand
	// the DockerService to provide detailed container information
	containers, err := c.dockerService.ListRunningContainers(c.ctx)
	if err != nil {
		return nil, err
	}

	var result []ContainerInfo
	for _, container := range containers {
		info := ContainerInfo{
			ID:     container.ID,
			Name:   container.Name,
			Image:  container.Image,
			Status: container.Status,
		}
		result = append(result, info)
	}

	return result, nil
}

func (c *Colog) matchesFilter(container ContainerInfo, filter ContainerFilter) bool {
	if filter.Name != "" && !strings.Contains(container.Name, filter.Name) {
		return false
	}
	if filter.Image != "" && !strings.Contains(container.Image, filter.Image) {
		return false
	}
	if filter.ImageID != "" && !strings.Contains(container.ImageID, filter.ImageID) {
		return false
	}
	if filter.Status != "" && !strings.Contains(container.Status, filter.Status) {
		return false
	}
	
	// Label matching
	for key, value := range filter.Labels {
		containerValue, exists := container.Labels[key]
		if !exists || containerValue != value {
			return false
		}
	}

	return true
}