package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/cors"
	"github.com/rs/xid"
)

// Docker types (copied from main package)
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

type LogEntry struct {
	ContainerID string
	Timestamp   time.Time
	Message     string
	Stream      string
}

// MCPServer represents the Model Context Protocol server for Docker logs
type MCPServer struct {
	dockerService *DockerService
	sessions    map[string]*Session
	sessionsMux sync.RWMutex
	upgrader    websocket.Upgrader
	port        string
	host        string
	auth        *AuthConfig
	ctx         context.Context
}

// Session represents an MCP session with SSE support
type Session struct {
	ID          string
	Created     time.Time
	LastAccess  time.Time
	SSEWriter   http.ResponseWriter
	SSEFlusher  http.Flusher
	SSEActive   bool
	Context     context.Context
	Cancel      context.CancelFunc
	RequestChan chan MCPRequest
	mutex       sync.RWMutex
}

// AuthConfig holds authentication settings
type AuthConfig struct {
	APIKey        string
	AllowedOrigins []string
	RequireAuth   bool
}

// MCPRequest represents an incoming MCP request
type MCPRequest struct {
	ID     interface{} `json:"id"`
	Method string      `json:"method"`
	Params interface{} `json:"params,omitempty"`
}

// MCPResponse represents an MCP response
type MCPResponse struct {
	ID     interface{} `json:"id"`
	Result interface{} `json:"result,omitempty"`
	Error  *MCPError   `json:"error,omitempty"`
}

// MCPError represents an MCP error
type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MCPNotification represents an MCP notification (no ID)
type MCPNotification struct {
	Method string      `json:"method"`
	Params interface{} `json:"params,omitempty"`
}

// Tool definitions
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// NewMCPServer creates a new MCP server instance
func NewMCPServer(port, host string, auth *AuthConfig) (*MCPServer, error) {
	ctx := context.Background()
	
	if auth == nil {
		auth = &AuthConfig{
			AllowedOrigins: []string{"*"},
			RequireAuth:    false,
		}
	}

	return &MCPServer{
		dockerService: nil, // Initialize lazily when needed
		sessions: make(map[string]*Session),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if len(auth.AllowedOrigins) == 0 || auth.AllowedOrigins[0] == "*" {
					return true
				}
				for _, allowed := range auth.AllowedOrigins {
					if origin == allowed {
						return true
					}
				}
				return false
			},
		},
		port: port,
		host: host,
		auth: auth,
		ctx:  ctx,
	}, nil
}

// Start starts the MCP server
func (s *MCPServer) Start() error {
	router := mux.NewRouter()

	// MCP endpoints
	router.HandleFunc("/mcp", s.handleMCPConnection).Methods("GET")
	router.HandleFunc("/mcp", s.handleMCPRequest).Methods("POST")
	router.HandleFunc("/health", s.handleHealth).Methods("GET")
	router.HandleFunc("/capabilities", s.handleCapabilities).Methods("GET")

	// Setup CORS
	c := cors.New(cors.Options{
		AllowedOrigins: s.auth.AllowedOrigins,
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"*"},
		AllowCredentials: true,
	})

	handler := c.Handler(router)

	// Add authentication middleware if required
	if s.auth.RequireAuth {
		handler = s.authMiddleware(handler)
	}

	addr := fmt.Sprintf("%s:%s", s.host, s.port)
	log.Printf("🚀 MCP Docker Log Server starting on http://%s", addr)
	log.Printf("🔧 Health check: http://%s/health", addr)
	log.Printf("📋 Capabilities: http://%s/capabilities", addr)

	return http.ListenAndServe(addr, handler)
}

// handleMCPConnection handles initial MCP connection with SSE support
func (s *MCPServer) handleMCPConnection(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		sessionID = xid.New().String()
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Create session context
	ctx, cancel := context.WithCancel(r.Context())

	session := &Session{
		ID:          sessionID,
		Created:     time.Now(),
		LastAccess:  time.Now(),
		SSEWriter:   w,
		SSEFlusher:  flusher,
		SSEActive:   true,
		Context:     ctx,
		Cancel:      cancel,
		RequestChan: make(chan MCPRequest, 100),
	}

	// Store session
	s.sessionsMux.Lock()
	s.sessions[sessionID] = session
	s.sessionsMux.Unlock()

	// Send initial capabilities
	s.sendSSEMessage(session, MCPNotification{
		Method: "capabilities",
		Params: s.getCapabilities(),
	})

	// Keep connection alive and handle cleanup
	defer func() {
		cancel()
		s.sessionsMux.Lock()
		delete(s.sessions, sessionID)
		s.sessionsMux.Unlock()
	}()

	// Keep connection alive
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sendSSEMessage(session, MCPNotification{
				Method: "ping",
				Params: map[string]interface{}{"timestamp": time.Now().Unix()},
			})
		}
	}
}

// handleMCPRequest handles MCP requests via HTTP POST
func (s *MCPServer) handleMCPRequest(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("X-Session-ID")
	if sessionID == "" {
		sessionID = r.URL.Query().Get("sessionId")
	}

	var req MCPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendErrorResponse(w, nil, -32700, "Parse error", nil)
		return
	}

	response := s.handleRequest(&req)

	// If we have an active session, also send via SSE
	if sessionID != "" {
		s.sessionsMux.RLock()
		if session, exists := s.sessions[sessionID]; exists && session.SSEActive {
			s.sendSSEMessage(session, response)
		}
		s.sessionsMux.RUnlock()
	}

	// Send HTTP response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleRequest processes MCP requests
func (s *MCPServer) handleRequest(req *MCPRequest) MCPResponse {
	switch req.Method {
	case "tools/list":
		return MCPResponse{
			ID:     req.ID,
			Result: map[string]interface{}{"tools": s.getTools()},
		}
	
	case "tools/call":
		return s.handleToolCall(req)

	case "containers/list":
		return s.handleContainersList(req)

	case "containers/logs":
		return s.handleContainerLogs(req)

	case "containers/export":
		return s.handleContainerExport(req)

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

// handleToolCall processes tool execution requests
func (s *MCPServer) handleToolCall(req *MCPRequest) MCPResponse {
	params, ok := req.Params.(map[string]interface{})
	if !ok {
		return MCPResponse{
			ID: req.ID,
			Error: &MCPError{
				Code:    -32602,
				Message: "Invalid params",
			},
		}
	}

	toolName, ok := params["name"].(string)
	if !ok {
		return MCPResponse{
			ID: req.ID,
			Error: &MCPError{
				Code:    -32602,
				Message: "Tool name required",
			},
		}
	}

	args, _ := params["arguments"].(map[string]interface{})

	switch toolName {
	case "list_containers":
		return s.handleContainersListTool(req.ID, args)
	case "get_container_logs":
		return s.handleContainerLogsTool(req.ID, args)
	case "export_logs_llm":
		return s.handleExportLogsTool(req.ID, args)
	case "filter_containers":
		return s.handleFilterContainersTool(req.ID, args)
	default:
		return MCPResponse{
			ID: req.ID,
			Error: &MCPError{
				Code:    -32601,
				Message: "Tool not found",
			},
		}
	}
}

// Docker service implementation (copied from main package)
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
	
	cmd := exec.Command("docker", "context", "ls", "--format", "{{.Name}}\\t{{.Description}}\\t{{.DockerEndpoint}}\\t{{.Current}}")
	output, err := cmd.Output()
	if err != nil {
		return endpoints
	}
	
	lines := strings.Split(string(output), "\\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		parts := strings.Split(line, "\\t")
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
	// For non-interactive mode, just return the first endpoint
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
	
	log.Printf("✓ Connected to Docker via %s (%s)", endpoint.Name, endpoint.Description)
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

func (ds *DockerService) GetRecentLogs(ctx context.Context, containerID string, tail int) ([]LogEntry, error) {
	// Use Docker SDK - this works regardless of PATH issues
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: true,
		Tail:       fmt.Sprintf("%d", tail),
	}
	
	out, err := ds.client.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return nil, fmt.Errorf("failed to get logs for container %s: %w", containerID, err)
	}
	defer out.Close()
	
	var logs []LogEntry
	
	// Docker API returns logs with a special header format
	// Read raw bytes and handle the multiplexed stream
	buf := make([]byte, 8) // Header is 8 bytes
	var logData []byte
	
	for {
		// Read header
		n, err := out.Read(buf)
		if err != nil || n == 0 {
			break // End of stream
		}
		
		// Extract payload size from header (bytes 4-7, big-endian)
		if n >= 8 {
			payloadSize := int(buf[4])<<24 | int(buf[5])<<16 | int(buf[6])<<8 | int(buf[7])
			
			// Read payload
			payload := make([]byte, payloadSize)
			n, err := out.Read(payload)
			if err != nil || n != payloadSize {
				break
			}
			
			logData = append(logData, payload...)
		}
	}
	
	// Parse the collected log data
	lines := strings.Split(string(logData), "\\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		
		logEntry := parseLogEntry(containerID, line)
		if !logEntry.Timestamp.IsZero() {
			logs = append(logs, logEntry)
		}
	}
	
	return logs, nil
}

// RestartContainer restarts a running container
func (ds *DockerService) RestartContainer(ctx context.Context, containerID string) error {
	return ds.client.ContainerRestart(ctx, containerID, container.StopOptions{})
}

// KillContainer forcefully kills a running container
func (ds *DockerService) KillContainer(ctx context.Context, containerID string) error {
	return ds.client.ContainerKill(ctx, containerID, "SIGKILL")
}

func parseLogEntry(containerID, line string) LogEntry {
	if len(line) == 0 {
		return LogEntry{}
	}

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

// Helper method to get Docker service with lazy initialization
func (s *MCPServer) getDockerService() (*DockerService, error) {
	if s.dockerService == nil {
		dockerService, err := NewDockerServiceWithSelection(false)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to Docker: %w", err)
		}
		s.dockerService = dockerService
	}
	return s.dockerService, nil
}

// Tool implementations
func (s *MCPServer) handleContainersListTool(id interface{}, args map[string]interface{}) MCPResponse {
	dockerService, err := s.getDockerService()
	if err != nil {
		return MCPResponse{
			ID: id,
			Error: &MCPError{
				Code:    -32603,
				Message: "Docker connection failed: " + err.Error(),
			},
		}
	}

	containers, err := dockerService.ListRunningContainers(s.ctx)
	if err != nil {
		return MCPResponse{
			ID: id,
			Error: &MCPError{
				Code:    -32603,
				Message: "Failed to list containers: " + err.Error(),
			},
		}
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

func (s *MCPServer) handleContainerLogsTool(id interface{}, args map[string]interface{}) MCPResponse {
	containerID, ok := args["container_id"].(string)
	if !ok {
		return MCPResponse{
			ID: id,
			Error: &MCPError{
				Code:    -32602,
				Message: "Missing required parameter: container_id",
			},
		}
	}

	tail := 50
	if t, ok := args["tail"].(float64); ok {
		tail = int(t)
	}

	dockerService, err := s.getDockerService()
	if err != nil {
		return MCPResponse{
			ID: id,
			Error: &MCPError{
				Code:    -32603,
				Message: "Docker connection failed: " + err.Error(),
			},
		}
	}

	// Get recent logs directly
	logs, err := dockerService.GetRecentLogs(s.ctx, containerID, tail)
	if err != nil {
		return MCPResponse{
			ID: id,
			Error: &MCPError{
				Code:    -32603,
				Message: "Failed to get logs: " + err.Error(),
			},
		}
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

func (s *MCPServer) handleExportLogsTool(id interface{}, args map[string]interface{}) MCPResponse {
	tail := 50
	if t, ok := args["tail"].(float64); ok {
		tail = int(t)
	}

	dockerService, err := s.getDockerService()
	if err != nil {
		return MCPResponse{
			ID: id,
			Error: &MCPError{
				Code:    -32603,
				Message: "Docker connection failed: " + err.Error(),
			},
		}
	}

	containers, err := dockerService.ListRunningContainers(s.ctx)
	if err != nil {
		return MCPResponse{
			ID: id,
			Error: &MCPError{
				Code:    -32603,
				Message: "Failed to list containers: " + err.Error(),
			},
		}
	}

	// Generate markdown export
	output := "# Docker Container Logs Summary\n\n"
	output += fmt.Sprintf("Generated at: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	for _, container := range containers {
		logs, err := dockerService.GetRecentLogs(s.ctx, container.ID, tail)
		if err != nil {
			continue // Skip containers with log errors
		}

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

func (s *MCPServer) handleFilterContainersTool(id interface{}, args map[string]interface{}) MCPResponse {
	dockerService, err := s.getDockerService()
	if err != nil {
		return MCPResponse{
			ID: id,
			Error: &MCPError{
				Code:    -32603,
				Message: "Docker connection failed: " + err.Error(),
			},
		}
	}

	containers, err := dockerService.ListRunningContainers(s.ctx)
	if err != nil {
		return MCPResponse{
			ID: id,
			Error: &MCPError{
				Code:    -32603,
				Message: "Failed to list containers: " + err.Error(),
			},
		}
	}

	// Apply filters
	var filtered []Container
	status, hasStatus := args["status"].(string)
	image, hasImage := args["image"].(string)
	name, hasName := args["name"].(string)

	for _, container := range containers {
		match := true
		
		if hasStatus && !strings.Contains(strings.ToLower(container.Status), strings.ToLower(status)) {
			match = false
		}
		if hasImage && !strings.Contains(strings.ToLower(container.Image), strings.ToLower(image)) {
			match = false
		}
		if hasName && !strings.Contains(strings.ToLower(container.Name), strings.ToLower(name)) {
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

// Legacy handlers for direct endpoints
func (s *MCPServer) handleContainersList(req *MCPRequest) MCPResponse {
	dockerService, err := s.getDockerService()
	if err != nil {
		return MCPResponse{
			ID: req.ID,
			Error: &MCPError{
				Code:    -32603,
				Message: "Docker connection failed: " + err.Error(),
			},
		}
	}

	containers, err := dockerService.ListRunningContainers(s.ctx)
	if err != nil {
		return MCPResponse{
			ID: req.ID,
			Error: &MCPError{
				Code:    -32603,
				Message: "Failed to list containers: " + err.Error(),
			},
		}
	}

	return MCPResponse{
		ID:     req.ID,
		Result: containers,
	}
}

func (s *MCPServer) handleContainerLogs(req *MCPRequest) MCPResponse {
	// Implementation similar to handleContainerLogsTool
	return MCPResponse{
		ID: req.ID,
		Error: &MCPError{
			Code:    -32601,
			Message: "Use tools/call with get_container_logs instead",
		},
	}
}

func (s *MCPServer) handleContainerExport(req *MCPRequest) MCPResponse {
	// Implementation similar to handleExportLogsTool
	return MCPResponse{
		ID: req.ID,
		Error: &MCPError{
			Code:    -32601,
			Message: "Use tools/call with export_logs_llm instead",
		},
	}
}

// Helper methods
func (s *MCPServer) sendSSEMessage(session *Session, message interface{}) {
	if !session.SSEActive {
		return
	}

	session.mutex.Lock()
	defer session.mutex.Unlock()

	data, err := json.Marshal(message)
	if err != nil {
		return
	}

	fmt.Fprintf(session.SSEWriter, "data: %s\n\n", data)
	session.SSEFlusher.Flush()
}

func (s *MCPServer) sendErrorResponse(w http.ResponseWriter, id interface{}, code int, message string, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	
	response := MCPResponse{
		ID: id,
		Error: &MCPError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	
	json.NewEncoder(w).Encode(response)
}

func (s *MCPServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			apiKey = r.URL.Query().Get("api_key")
		}

		if apiKey != s.auth.APIKey {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *MCPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	response := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"version":   "1.2.0",
		"sessions":  len(s.sessions),
		"capabilities": s.getCapabilities(),
	}
	
	json.NewEncoder(w).Encode(response)
}

func (s *MCPServer) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.getCapabilities())
}

func (s *MCPServer) getCapabilities() map[string]interface{} {
	return map[string]interface{}{
		"experimental": map[string]interface{}{},
		"logging":      map[string]interface{}{},
		"prompts":      map[string]interface{}{},
		"resources":    map[string]interface{}{},
		"tools": map[string]interface{}{
			"listChanged": false,
		},
	}
}

func (s *MCPServer) getTools() []ToolDefinition {
	return []ToolDefinition{
		{
			Name:        "list_containers",
			Description: "List Docker containers with optional filtering",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"all": map[string]interface{}{
						"type":        "boolean",
						"description": "Include stopped containers",
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
						"type":        "number",
						"description": "Number of log lines to retrieve",
						"default":     50,
					},
					"follow": map[string]interface{}{
						"type":        "boolean",
						"description": "Follow log output",
						"default":     false,
					},
				},
				"required": []string{"container_id"},
			},
		},
		{
			Name:        "export_logs_llm",
			Description: "Export container logs in LLM-friendly format",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"container_ids": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "string",
						},
						"description": "List of container IDs",
					},
					"format": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"json", "markdown"},
						"description": "Export format",
						"default":     "markdown",
					},
					"tail": map[string]interface{}{
						"type":        "number",
						"description": "Number of log lines per container",
						"default":     100,
					},
				},
				"required": []string{"container_ids"},
			},
		},
		{
			Name:        "filter_containers",
			Description: "Filter containers by various criteria",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Filter by container name",
					},
					"image": map[string]interface{}{
						"type":        "string",
						"description": "Filter by image name",
					},
					"status": map[string]interface{}{
						"type":        "string",
						"description": "Filter by container status",
					},
				},
			},
		},
	}
}

func main() {
	port := os.Getenv("MCP_PORT")
	if port == "" {
		port = "8080"
	}

	host := os.Getenv("MCP_HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	// Setup authentication
	auth := &AuthConfig{
		APIKey:      os.Getenv("MCP_API_KEY"),
		RequireAuth: os.Getenv("MCP_API_KEY") != "",
		AllowedOrigins: []string{"*"},
	}

	if origins := os.Getenv("MCP_ALLOWED_ORIGINS"); origins != "" {
		auth.AllowedOrigins = strings.Split(origins, ",")
	}

	server, err := NewMCPServer(port, host, auth)
	if err != nil {
		log.Fatalf("Failed to create MCP server: %v", err)
	}

	if err := server.Start(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// Helper function to safely truncate container ID for display
func truncateContainerID(containerID string) string {
	if len(containerID) <= 12 {
		return containerID
	}
	return containerID[:12]
}