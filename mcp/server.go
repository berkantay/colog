package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/cors"
	"github.com/rs/xid"

	// Import parent Colog SDK
	"github.com/berkantay/colog/pkg/colog"
)

// MCPServer represents the Model Context Protocol server for Docker logs
type MCPServer struct {
	colog       *colog.Colog
	sessions    map[string]*Session
	sessionsMux sync.RWMutex
	upgrader    websocket.Upgrader
	port        string
	host        string
	auth        *AuthConfig
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
	colog, err := colog.NewColog(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Colog: %w", err)
	}

	if auth == nil {
		auth = &AuthConfig{
			AllowedOrigins: []string{"*"},
			RequireAuth:    false,
		}
	}

	return &MCPServer{
		colog:    colog,
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

// Tool implementations
func (s *MCPServer) handleContainersListTool(id interface{}, args map[string]interface{}) MCPResponse {
	showAll := false
	if all, ok := args["all"].(bool); ok {
		showAll = all
	}

	var containers []colog.ContainerInfo
	var err error

	if showAll {
		containers, err = s.colog.ListAllContainers()
	} else {
		containers, err = s.colog.ListRunningContainers()
	}

	if err != nil {
		return MCPResponse{
			ID: id,
			Error: &MCPError{
				Code:    -32603,
				Message: "Failed to list containers",
				Data:    err.Error(),
			},
		}
	}

	return MCPResponse{
		ID: id,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf("Found %d containers", len(containers)),
				},
				{
					"type": "resource",
					"resource": map[string]interface{}{
						"type":        "containers",
						"containers":  containers,
						"count":       len(containers),
						"generated_at": time.Now().Format(time.RFC3339),
					},
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
				Message: "container_id required",
			},
		}
	}

	options := colog.LogOptions{
		Tail:       50,
		Follow:     false,
		Timestamps: true,
	}

	if tail, ok := args["tail"].(float64); ok {
		options.Tail = int(tail)
	}

	if follow, ok := args["follow"].(bool); ok {
		options.Follow = follow
	}

	logs, err := s.colog.GetContainerLogs(containerID, options)
	if err != nil {
		return MCPResponse{
			ID: id,
			Error: &MCPError{
				Code:    -32603,
				Message: "Failed to get container logs",
				Data:    err.Error(),
			},
		}
	}

	return MCPResponse{
		ID: id,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf("Retrieved %d log entries from container %s", len(logs), containerID[:12]),
				},
				{
					"type": "resource",
					"resource": map[string]interface{}{
						"type":         "logs",
						"container_id": containerID,
						"logs":         logs,
						"count":        len(logs),
						"generated_at": time.Now().Format(time.RFC3339),
					},
				},
			},
		},
	}
}

func (s *MCPServer) handleExportLogsTool(id interface{}, args map[string]interface{}) MCPResponse {
	containerIDs, ok := args["container_ids"].([]interface{})
	if !ok {
		return MCPResponse{
			ID: id,
			Error: &MCPError{
				Code:    -32602,
				Message: "container_ids required",
			},
		}
	}

	// Convert to string slice
	ids := make([]string, len(containerIDs))
	for i, cid := range containerIDs {
		if str, ok := cid.(string); ok {
			ids[i] = str
		}
	}

	format := "markdown"
	if f, ok := args["format"].(string); ok {
		format = f
	}

	options := colog.LogOptions{
		Tail:       100,
		Follow:     false,
		Timestamps: true,
	}

	if tail, ok := args["tail"].(float64); ok {
		options.Tail = int(tail)
	}

	var result string
	var err error

	switch format {
	case "json":
		result, err = s.colog.ExportLogsAsJSON(ids, options)
	default:
		result, err = s.colog.ExportLogsAsMarkdown(ids, options)
	}

	if err != nil {
		return MCPResponse{
			ID: id,
			Error: &MCPError{
				Code:    -32603,
				Message: "Failed to export logs",
				Data:    err.Error(),
			},
		}
	}

	return MCPResponse{
		ID: id,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf("Exported logs from %d containers in %s format (%d characters)", len(ids), format, len(result)),
				},
				{
					"type": "resource",
					"resource": map[string]interface{}{
						"type":          "export",
						"format":        format,
						"container_ids": ids,
						"data":          result,
						"size":          len(result),
						"generated_at":  time.Now().Format(time.RFC3339),
					},
				},
			},
		},
	}
}

func (s *MCPServer) handleFilterContainersTool(id interface{}, args map[string]interface{}) MCPResponse {
	filter := colog.ContainerFilter{}

	if name, ok := args["name"].(string); ok {
		filter.Name = name
	}
	if image, ok := args["image"].(string); ok {
		filter.Image = image
	}
	if status, ok := args["status"].(string); ok {
		filter.Status = status
	}

	containers, err := s.colog.FilterContainers(filter)
	if err != nil {
		return MCPResponse{
			ID: id,
			Error: &MCPError{
				Code:    -32603,
				Message: "Failed to filter containers",
				Data:    err.Error(),
			},
		}
	}

	return MCPResponse{
		ID: id,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf("Found %d containers matching filter", len(containers)),
				},
				{
					"type": "resource",
					"resource": map[string]interface{}{
						"type":        "filtered_containers",
						"containers":  containers,
						"filter":      filter,
						"count":       len(containers),
						"generated_at": time.Now().Format(time.RFC3339),
					},
				},
			},
		},
	}
}

// Legacy handlers for direct endpoints
func (s *MCPServer) handleContainersList(req *MCPRequest) MCPResponse {
	containers, err := s.colog.ListRunningContainers()
	if err != nil {
		return MCPResponse{
			ID: req.ID,
			Error: &MCPError{
				Code:    -32603,
				Message: "Failed to list containers",
				Data:    err.Error(),
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