package container

import (
	"context"
	"fmt"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/berkantay/colog/internal/docker"
)

// ContainerContext represents an isolated context for a single container
type ContainerContext struct {
	Container     docker.Container
	LogView       *tview.TextView
	LogBuffer     []docker.LogEntry
	LogChannel    chan docker.LogEntry
	Color         tcell.Color
	IsSelected    bool
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	streamStarted bool
	app           *tview.Application // Reference to app for thread-safe UI updates
}

// NewContainerContext creates a new container context
func NewContainerContext(container docker.Container, color tcell.Color, app *tview.Application) *ContainerContext {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &ContainerContext{
		Container:  container,
		LogBuffer:  make([]docker.LogEntry, 0, 50), // Keep last 50 entries
		LogChannel: make(chan docker.LogEntry, 100),
		Color:      color,
		IsSelected: false,
		ctx:        ctx,
		cancel:     cancel,
		app:        app,
	}
}

// Initialize sets up the log view and starts log streaming
func (cc *ContainerContext) Initialize(dockerService *docker.DockerService) error {
	cc.setupLogView()
	return cc.startLogStreaming(dockerService)
}

// setupLogView creates and configures the tview.TextView for this container
func (cc *ContainerContext) setupLogView() {
	cc.LogView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(true).
		SetMaxLines(1000)
	
	trueBlack := tcell.NewRGBColor(0, 0, 0)
	cc.LogView.SetBackgroundColor(trueBlack)

	title := fmt.Sprintf(" %s ", cc.Container.Name)
	if len(title) > 30 {
		title = title[:27] + "... "
	}

	cc.LogView.SetBorder(true).
		SetTitle(title).
		SetTitleAlign(tview.AlignLeft).
		SetBorderColor(cc.Color)

	// Display container info
	cc.LogView.SetText(fmt.Sprintf("[%s:#000000]Container: %s[white:#000000]\n[%s:#000000]Image: %s[white:#000000]\n[%s:#000000]Status: %s[white:#000000]\n[gray:#000000]────────────────────────────────[white:#000000]\n",
		cc.colorToTviewColor(cc.Color), cc.Container.Name,
		cc.colorToTviewColor(cc.Color), cc.Container.Image,
		cc.colorToTviewColor(cc.Color), cc.Container.Status))
}

// startLogStreaming begins streaming logs for this container
func (cc *ContainerContext) startLogStreaming(dockerService *docker.DockerService) error {
	if cc.streamStarted {
		return nil
	}
	
	cc.streamStarted = true
	
	go func() {
		err := dockerService.StreamLogs(cc.ctx, cc.Container.ID, cc.LogChannel)
		if err != nil {
			cc.AppendLog(fmt.Sprintf("[red]Error streaming logs: %v[white]", err))
		}
	}()
	
	// Start log processing goroutine
	go cc.processLogs()
	
	return nil
}

// processLogs handles incoming log entries
func (cc *ContainerContext) processLogs() {
	for {
		select {
		case <-cc.ctx.Done():
			return
		case entry, ok := <-cc.LogChannel:
			if !ok {
				return
			}
			
			// Add to buffer (keep last 50 entries)
			cc.mu.Lock()
			cc.LogBuffer = append(cc.LogBuffer, entry)
			if len(cc.LogBuffer) > 50 {
				cc.LogBuffer = cc.LogBuffer[1:]
			}
			cc.mu.Unlock()
			
			// Format and display log entry
			timestamp := entry.Timestamp.Format("15:04:05")
			logLine := fmt.Sprintf("[gray:#000000]%s[white:#000000] %s", timestamp, entry.Message)
			cc.AppendLog(logLine)
		}
	}
}

// AppendLog adds a log line to the view (thread-safe)
func (cc *ContainerContext) AppendLog(message string) {
	if cc.LogView != nil && cc.app != nil {
		cc.app.QueueUpdateDraw(func() {
			fmt.Fprintf(cc.LogView, "%s\n", message)
			cc.LogView.ScrollToEnd()
		})
	}
}

// SetSelected updates the visual selection state
func (cc *ContainerContext) SetSelected(selected bool) {
	cc.IsSelected = selected
	if cc.LogView != nil {
		if selected {
			cc.LogView.SetBorderColor(tcell.NewRGBColor(255, 140, 0)) // Orange for focus
		} else {
			cc.LogView.SetBorderColor(cc.Color)
		}
	}
}

// GetLogBuffer returns a copy of the current log buffer
func (cc *ContainerContext) GetLogBuffer() []docker.LogEntry {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	
	buffer := make([]docker.LogEntry, len(cc.LogBuffer))
	copy(buffer, cc.LogBuffer)
	return buffer
}

// Cleanup stops log streaming and cleans up resources
func (cc *ContainerContext) Cleanup() {
	if cc.cancel != nil {
		cc.cancel()
	}
	if cc.LogChannel != nil {
		close(cc.LogChannel)
	}
}

// colorToTviewColor converts tcell.Color to tview color string
func (cc *ContainerContext) colorToTviewColor(color tcell.Color) string {
	// Handle standard colors
	colorMap := map[tcell.Color]string{
		tcell.ColorWhite:  "white",
		tcell.ColorSilver: "silver", 
		tcell.ColorGray:   "gray",
	}
	
	if name, ok := colorMap[color]; ok {
		return name
	}
	
	// For RGB colors, convert to hex format for tview
	r, g, b := color.RGB()
	if r == 255 && g == 140 && b == 0 {
		return "#FF8C00" // Orange
	}
	if r == 255 && g == 248 && b == 235 {
		return "#FFF8EB" // Orangish white
	}
	
	// For other grays, return a hex approximation
	if r == g && g == b { // Grayscale color
		return fmt.Sprintf("#%02X%02X%02X", r, g, b)
	}
	
	return "white"
}

// ContainerContextManager manages all container contexts
type ContainerContextManager struct {
	contexts      map[string]*ContainerContext
	orderedIDs    []string
	colors        []tcell.Color
	colorIndex    int
	mu            sync.RWMutex
}

// NewContainerContextManager creates a new context manager
func NewContainerContextManager() *ContainerContextManager {
	return &ContainerContextManager{
		contexts:   make(map[string]*ContainerContext),
		orderedIDs: make([]string, 0),
		colors:     GetContainerColors(),
		colorIndex: 0,
	}
}

// InitializeContexts creates contexts for all containers
func (ccm *ContainerContextManager) InitializeContexts(containers []docker.Container, dockerService *docker.DockerService, app *tview.Application) error {
	ccm.mu.Lock()
	defer ccm.mu.Unlock()
	
	for _, container := range containers {
		color := ccm.colors[ccm.colorIndex%len(ccm.colors)]
		ccm.colorIndex++
		
		context := NewContainerContext(container, color, app)
		if err := context.Initialize(dockerService); err != nil {
			return fmt.Errorf("failed to initialize context for %s: %w", container.Name, err)
		}
		
		ccm.contexts[container.ID] = context
		ccm.orderedIDs = append(ccm.orderedIDs, container.ID)
	}
	
	return nil
}

// GetContext returns the context for a specific container ID
func (ccm *ContainerContextManager) GetContext(containerID string) (*ContainerContext, bool) {
	ccm.mu.RLock()
	defer ccm.mu.RUnlock()
	context, exists := ccm.contexts[containerID]
	return context, exists
}

// GetAllContexts returns all contexts in order
func (ccm *ContainerContextManager) GetAllContexts() []*ContainerContext {
	ccm.mu.RLock()
	defer ccm.mu.RUnlock()
	
	contexts := make([]*ContainerContext, 0, len(ccm.orderedIDs))
	for _, id := range ccm.orderedIDs {
		if context, exists := ccm.contexts[id]; exists {
			contexts = append(contexts, context)
		}
	}
	return contexts
}

// GetContextByIndex returns context at specific index
func (ccm *ContainerContextManager) GetContextByIndex(index int) *ContainerContext {
	ccm.mu.RLock()
	defer ccm.mu.RUnlock()
	
	if index >= 0 && index < len(ccm.orderedIDs) {
		id := ccm.orderedIDs[index]
		return ccm.contexts[id]
	}
	return nil
}

// Count returns the number of contexts
func (ccm *ContainerContextManager) Count() int {
	ccm.mu.RLock()
	defer ccm.mu.RUnlock()
	return len(ccm.orderedIDs)
}

// SetSelected updates the selection state for a context at index
func (ccm *ContainerContextManager) SetSelected(index int, selected bool) {
	context := ccm.GetContextByIndex(index)
	if context != nil {
		context.SetSelected(selected)
	}
}

// Cleanup cleans up all contexts
func (ccm *ContainerContextManager) Cleanup() {
	ccm.mu.Lock()
	defer ccm.mu.Unlock()
	
	for _, context := range ccm.contexts {
		context.Cleanup()
	}
	ccm.contexts = make(map[string]*ContainerContext)
	ccm.orderedIDs = make([]string, 0)
}

// StopAll stops all running contexts (alias for Cleanup for clarity)
func (ccm *ContainerContextManager) StopAll() {
	ccm.Cleanup()
}

// GetContainerColors returns the list of colors used for container display
func GetContainerColors() []tcell.Color {
	orangishWhite := tcell.NewRGBColor(255, 248, 235) // Single orangish white color
	return []tcell.Color{
		orangishWhite,
		orangishWhite,
		orangishWhite,
		orangishWhite,
		orangishWhite,
		orangishWhite,
		orangishWhite,
		orangishWhite,
		orangishWhite,
		orangishWhite,
		orangishWhite,
		orangishWhite,
		orangishWhite,
		orangishWhite,
		orangishWhite,
	}
}