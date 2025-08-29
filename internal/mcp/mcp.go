package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/berkantay/colog/internal/docker"
)

// MCP Protocol Types for stdio transport
type MCPRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

type MCPResponse struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id"`
	Result  map[string]interface{} `json:"result,omitempty"`
	Error   *MCPError              `json:"error,omitempty"`
}

type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type MCPStdioServer struct {
	dockerService *docker.DockerService
	ctx           context.Context
}

func NewMCPStdioServer() (*MCPStdioServer, error) {
	ctx := context.Background()
	
	return &MCPStdioServer{
		dockerService: nil, // Initialize lazily when needed
		ctx:           ctx,
	}, nil
}

func (s *MCPStdioServer) getDockerService() (*docker.DockerService, error) {
	if s.dockerService == nil {
		dockerService, err := docker.NewDockerService()
		if err != nil {
			return nil, fmt.Errorf("failed to connect to Docker: %w", err)
		}
		s.dockerService = dockerService
	}
	return s.dockerService, nil
}

func (s *MCPStdioServer) Start() error {
	scanner := bufio.NewScanner(os.Stdin)
	
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var req MCPRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			s.sendErrorResponse(req.ID, -32700, "Parse error", nil)
			continue
		}

		response := s.handleRequest(&req)
		s.sendResponse(response)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading stdin: %w", err)
	}

	return nil
}

func (s *MCPStdioServer) handleRequest(req *MCPRequest) MCPResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolCall(req)
	default:
		return MCPResponse{
			ID: req.ID,
			Error: &MCPError{
				Code:    -32601,
				Message: "Method not found",
			},
		}
	}
}

func (s *MCPStdioServer) handleInitialize(req *MCPRequest) MCPResponse {
	return MCPResponse{
		ID: req.ID,
		Result: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{
					"listChanged": false,
				},
				"logging":      map[string]interface{}{},
				"prompts":      map[string]interface{}{},
				"resources":    map[string]interface{}{},
				"experimental": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "colog-mcp",
				"version": "1.0.0",
			},
		},
	}
}

func (s *MCPStdioServer) handleToolsList(req *MCPRequest) MCPResponse {
	tools := []ToolDefinition{
		{
			Name:        "list_containers",
			Description: "List Docker containers with optional filtering",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"all": map[string]interface{}{
						"type":        "boolean",
						"description": "List all containers (including stopped ones)",
						"default":     false,
					},
				},
			},
		},
		{
			Name:        "get_container_logs",
			Description: "Get logs from a specific Docker container",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"container_id": map[string]interface{}{
						"type":        "string",
						"description": "Container ID or name",
					},
					"tail": map[string]interface{}{
						"type":        "integer",
						"description": "Number of recent log lines to retrieve (default: 50)",
						"default":     50,
					},
					"since": map[string]interface{}{
						"type":        "string",
						"description": "Show logs since timestamp (e.g. 2013-01-02T13:23:37Z)",
					},
				},
				"required": []string{"container_id"},
			},
		},
		{
			Name:        "export_logs_llm",
			Description: "Export container logs in markdown format optimized for LLM analysis",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"tail": map[string]interface{}{
						"type":        "integer",
						"description": "Number of recent log lines per container (default: 50)",
						"default":     50,
					},
					"containers": map[string]interface{}{
						"type":        "array",
						"description": "Specific container IDs/names to export (default: all running)",
						"items": map[string]interface{}{
							"type": "string",
						},
					},
				},
			},
		},
		{
			Name:        "filter_containers",
			Description: "Filter containers by various criteria",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"status": map[string]interface{}{
						"type":        "string",
						"description": "Filter by status (running, exited, paused, etc.)",
					},
					"image": map[string]interface{}{
						"type":        "string",
						"description": "Filter by image name",
					},
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Filter by container name (partial match)",
					},
				},
			},
		},
	}

	return MCPResponse{
		ID: req.ID,
		Result: map[string]interface{}{
			"tools": tools,
		},
	}
}

func (s *MCPStdioServer) handleToolCall(req *MCPRequest) MCPResponse {
	params, ok := req.Params["arguments"].(map[string]interface{})
	if !ok {
		params = make(map[string]interface{})
	}

	toolName, ok := req.Params["name"].(string)
	if !ok {
		return s.createErrorResponse(req.ID, -32602, "Invalid params: missing tool name")
	}

	switch toolName {
	case "list_containers":
		return s.handleListContainers(req.ID, params)
	case "get_container_logs":
		return s.handleGetContainerLogs(req.ID, params)
	case "export_logs_llm":
		return s.handleExportLogsLLM(req.ID, params)
	case "filter_containers":
		return s.handleFilterContainers(req.ID, params)
	default:
		return s.createErrorResponse(req.ID, -32601, "Unknown tool: "+toolName)
	}
}

func (s *MCPStdioServer) handleListContainers(id interface{}, args map[string]interface{}) MCPResponse {
	_, _ = args["all"].(bool) // Note: currently only lists running containers
	
	dockerService, err := s.getDockerService()
	if err != nil {
		return s.createErrorResponse(id, -32603, "Docker connection failed: "+err.Error())
	}
	
	containers, err := dockerService.ListRunningContainers(s.ctx)
	if err != nil {
		return s.createErrorResponse(id, -32603, "Failed to list containers: "+err.Error())
	}

	// Format containers for display
	var containerList []string
	for _, container := range containers {
		status := container.Status
		if len(status) > 20 {
			status = status[:20] + "..."
		}
		containerList = append(containerList, fmt.Sprintf("• %s (%s) - %s", container.Name, container.ID[:12], status))
	}
	
	response := fmt.Sprintf("Found %d containers:\n\n%s", len(containers), strings.Join(containerList, "\n"))
	
	return MCPResponse{
		ID: id,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": response,
				},
			},
		},
	}
}

func (s *MCPStdioServer) handleGetContainerLogs(id interface{}, args map[string]interface{}) MCPResponse {
	containerID, ok := args["container_id"].(string)
	if !ok {
		return s.createErrorResponse(id, -32602, "Missing required parameter: container_id")
	}

	tail := 50
	if t, ok := args["tail"].(float64); ok {
		tail = int(t)
	}

	dockerService, err := s.getDockerService()
	if err != nil {
		return s.createErrorResponse(id, -32603, "Docker connection failed: "+err.Error())
	}

	// Get recent logs directly
	logs, err := dockerService.GetRecentLogs(s.ctx, containerID, tail)
	if err != nil {
		return s.createErrorResponse(id, -32603, "Failed to get logs: "+err.Error())
	}
	// Format logs for display
	var logLines []string
	for _, log := range logs {
		timestamp := log.Timestamp.Format("15:04:05")
		logLines = append(logLines, fmt.Sprintf("[%s] %s", timestamp, log.Message))
	}
	
	response := fmt.Sprintf("Retrieved %d log entries from container %s:\n\n%s", 
		len(logs), truncateContainerID(containerID), strings.Join(logLines, "\n"))
	
	return MCPResponse{
		ID: id,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": response,
				},
			},
		},
	}
}

func (s *MCPStdioServer) handleExportLogsLLM(id interface{}, args map[string]interface{}) MCPResponse {
	tail := 50
	if t, ok := args["tail"].(float64); ok {
		tail = int(t)
	}

	containers, err := s.dockerService.ListRunningContainers(s.ctx)
	if err != nil {
		return s.createErrorResponse(id, -32603, "Failed to list containers: "+err.Error())
	}

	// Generate markdown export
	output := "# Docker Container Logs Summary\n\n"
	output += fmt.Sprintf("Generated at: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	for _, container := range containers {
		logCh := make(chan docker.LogEntry, 100)
		
		go func() {
			defer close(logCh)
			s.dockerService.StreamLogs(s.ctx, container.ID, logCh)
		}()

		var logs []docker.LogEntry
		timeout := time.After(3 * time.Second)
		collected := 0

		for collected < tail {
			select {
			case entry, ok := <-logCh:
				if !ok {
					goto nextContainer
				}
				logs = append(logs, entry)
				collected++
			case <-timeout:
				goto nextContainer
			}
		}

	nextContainer:
		if len(logs) > 0 {
			output += fmt.Sprintf("## Container: %s\n", container.Name)
			output += fmt.Sprintf("- Image: %s\n", container.Image)
			output += fmt.Sprintf("- Status: %s\n\n", container.Status)
			
			output += "```\n"
			for _, log := range logs {
				timestamp := log.Timestamp.Format("2006-01-02 15:04:05")
				output += fmt.Sprintf("[%s] %s\n", timestamp, log.Message)
			}
			output += "```\n\n"
		}
	}

	return MCPResponse{
		ID: id,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": output,
				},
			},
		},
	}
}

func (s *MCPStdioServer) handleFilterContainers(id interface{}, args map[string]interface{}) MCPResponse {
	containers, err := s.dockerService.ListRunningContainers(s.ctx)
	if err != nil {
		return s.createErrorResponse(id, -32603, "Failed to list containers: "+err.Error())
	}

	// Apply filters
	var filtered []docker.Container
	status, hasStatus := args["status"].(string)
	image, hasImage := args["image"].(string)
	name, hasName := args["name"].(string)

	for _, container := range containers {
		match := true
		
		if hasStatus && container.Status != status {
			match = false
		}
		if hasImage && container.Image != image {
			match = false
		}
		if hasName && container.Name != name {
			match = false
		}

		if match {
			filtered = append(filtered, container)
		}
	}

	// Format filtered containers for display
	var containerList []string
	for _, container := range filtered {
		status := container.Status
		if len(status) > 20 {
			status = status[:20] + "..."
		}
		containerList = append(containerList, fmt.Sprintf("• %s (%s) - %s", container.Name, container.ID[:12], status))
	}
	
	filtersUsed := []string{}
	if hasStatus { filtersUsed = append(filtersUsed, fmt.Sprintf("status=%s", status)) }
	if hasImage { filtersUsed = append(filtersUsed, fmt.Sprintf("image=%s", image)) }
	if hasName { filtersUsed = append(filtersUsed, fmt.Sprintf("name=%s", name)) }
	
	response := fmt.Sprintf("Found %d containers matching filters [%s]:\n\n%s", 
		len(filtered), strings.Join(filtersUsed, ", "), strings.Join(containerList, "\n"))
	
	return MCPResponse{
		ID: id,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": response,
				},
			},
		},
	}
}

func (s *MCPStdioServer) createErrorResponse(id interface{}, code int, message string) MCPResponse {
	return MCPResponse{
		ID: id,
		Error: &MCPError{
			Code:    code,
			Message: message,
		},
	}
}

func (s *MCPStdioServer) sendErrorResponse(id interface{}, code int, message string, data interface{}) {
	response := MCPResponse{
		ID: id,
		Error: &MCPError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	s.sendResponse(response)
}

func (s *MCPStdioServer) sendResponse(response MCPResponse) {
	response.JSONRPC = "2.0"
	data, err := json.Marshal(response)
	if err != nil {
		// Fallback error response
		fallback := MCPResponse{
			JSONRPC: "2.0",
			ID:      response.ID,
			Error: &MCPError{
				Code:    -32603,
				Message: "Internal error: failed to marshal response",
			},
		}
		data, _ = json.Marshal(fallback)
	}
	
	fmt.Println(string(data))
}

func RunMCPStdio() error {
	server, err := NewMCPStdioServer()
	if err != nil {
		return fmt.Errorf("failed to create MCP stdio server: %w", err)
	}

	return server.Start()
}

// Helper function to safely truncate container ID for display  
func truncateContainerID(containerID string) string {
	// If it's a hex ID (longer than 12 chars), truncate it
	// If it's a name (shorter), keep it as is
	if len(containerID) > 12 && isHexString(containerID) {
		return containerID[:12]
	}
	return containerID
}

// Helper to check if string looks like a hex container ID
func isHexString(s string) bool {
	if len(s) < 12 {
		return false
	}
	for _, c := range s[:12] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}