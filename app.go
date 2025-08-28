package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type App struct {
	app           *tview.Application
	grid          *tview.Grid
	mainGrid      *tview.Grid
	helpBar       *tview.TextView
	dockerService *DockerService
	logViews      map[string]*tview.TextView
	containers    []Container
	colors        []tcell.Color
	colorIndex    int
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	recentLogs    map[string][]LogEntry
}

func NewApp() *App {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &App{
		app:        tview.NewApplication(),
		grid:       tview.NewGrid(),
		mainGrid:   tview.NewGrid(),
		helpBar:    tview.NewTextView(),
		logViews:   make(map[string]*tview.TextView),
		colors:     getContainerColors(),
		colorIndex: 0,
		ctx:        ctx,
		cancel:     cancel,
		recentLogs: make(map[string][]LogEntry),
	}
}

func (a *App) Run() error {
	var err error
	a.dockerService, err = NewDockerService()
	if err != nil {
		return fmt.Errorf("failed to connect to Docker: %w", err)
	}
	defer a.dockerService.Close()

	if err := a.loadContainers(); err != nil {
		return err
	}

	if len(a.containers) == 0 {
		return fmt.Errorf("no running containers found")
	}

	if err := a.setupUI(); err != nil {
		return err
	}

	a.setupGrid()
	a.setupHelpBar()
	a.setupMainLayout()
	a.startLogStreaming()
	a.setupKeyBindings()

	// Check if we have a proper TTY before starting the TUI
	if !isTTY() {
		fmt.Println("\nTTY not available, falling back to simple log output mode...")
		return a.runSimpleMode()
	}

	if err := a.app.SetRoot(a.mainGrid, true).Run(); err != nil {
		return fmt.Errorf("failed to run TUI application: %w", err)
	}
	return nil
}

func (a *App) setupUI() error {
	a.grid.SetBorders(true).
		SetBordersColor(tcell.ColorGray)

	return nil
}

func (a *App) loadContainers() error {
	containers, err := a.dockerService.ListRunningContainers(a.ctx)
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	a.mu.Lock()
	a.containers = containers
	a.mu.Unlock()

	return nil
}

func (a *App) setupGrid() {
	a.mu.RLock()
	containerCount := len(a.containers)
	a.mu.RUnlock()

	if containerCount == 0 {
		return
	}

	cols := int(math.Ceil(math.Sqrt(float64(containerCount))))
	rows := int(math.Ceil(float64(containerCount) / float64(cols)))

	a.grid.Clear()

	rowSizes := make([]int, rows)
	for i := range rowSizes {
		rowSizes[i] = 0
	}

	colSizes := make([]int, cols)
	for i := range colSizes {
		colSizes[i] = 0
	}

	a.grid.SetRows(rowSizes...).SetColumns(colSizes...)

	a.mu.RLock()
	for i, container := range a.containers {
		row := i / cols
		col := i % cols
		
		logView := a.createLogView(container)
		a.logViews[container.ID] = logView
		
		a.grid.AddItem(logView, row, col, 1, 1, 0, 0, i == 0)
	}
	a.mu.RUnlock()
}

func (a *App) setupHelpBar() {
	a.helpBar.SetDynamicColors(true).
		SetWrap(false).
		SetScrollable(false).
		SetBorder(true).
		SetBorderColor(tcell.ColorGray).
		SetTitle(" Shortcuts ")

	helpText := "[#FF8C00]q[white]/[#FF8C00]Ctrl+C[white]: Quit  [#FF8C00]g[white]: Export logs  [#FF8C00]Tab[white]: Navigate  [#FF8C00]↑↓←→[white]: Scroll"
	a.helpBar.SetText(helpText)
}

func (a *App) setupMainLayout() {
	a.mainGrid.SetBorders(false).
		SetRows(0, 3).  // Main content takes available space, help bar takes 3 rows
		SetColumns(0).   // Single column
		AddItem(a.grid, 0, 0, 1, 1, 0, 0, true).    // Container grid takes row 0
		AddItem(a.helpBar, 1, 0, 1, 1, 0, 0, false)  // Help bar takes row 1
}

func (a *App) createLogView(container Container) *tview.TextView {
	color := a.colors[a.colorIndex%len(a.colors)]
	a.colorIndex++

	logView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(true).
		SetMaxLines(1000)

	title := fmt.Sprintf(" %s ", container.Name)
	if len(title) > 30 {
		title = title[:27] + "... "
	}

	logView.SetBorder(true).
		SetTitle(title).
		SetTitleAlign(tview.AlignLeft).
		SetBorderColor(color)

	logView.SetText(fmt.Sprintf("[%s]Container: %s[white]\n[%s]Image: %s[white]\n[%s]Status: %s[white]\n[gray]────────────────────────────────[white]\n",
		colorToTviewColor(color), container.Name,
		colorToTviewColor(color), container.Image,
		colorToTviewColor(color), container.Status))

	return logView
}

func (a *App) startLogStreaming() {
	a.mu.RLock()
	containers := make([]Container, len(a.containers))
	copy(containers, a.containers)
	a.mu.RUnlock()

	for _, container := range containers {
		go a.streamContainerLogs(container)
	}
}

func (a *App) streamContainerLogs(container Container) {
	logCh := make(chan LogEntry, 100)
	
	err := a.dockerService.StreamLogs(a.ctx, container.ID, logCh)
	if err != nil {
		a.appendLog(container.ID, fmt.Sprintf("[red]Error streaming logs: %v[white]", err))
		return
	}

	for {
		select {
		case <-a.ctx.Done():
			return
		case entry, ok := <-logCh:
			if !ok {
				return
			}
			
			// Store log entry in recent logs (keep last 50)
			a.mu.Lock()
			if a.recentLogs[container.ID] == nil {
				a.recentLogs[container.ID] = make([]LogEntry, 0, 50)
			}
			a.recentLogs[container.ID] = append(a.recentLogs[container.ID], entry)
			if len(a.recentLogs[container.ID]) > 50 {
				a.recentLogs[container.ID] = a.recentLogs[container.ID][1:]
			}
			a.mu.Unlock()
			
			timestamp := entry.Timestamp.Format("15:04:05")
			logLine := fmt.Sprintf("[gray]%s[white] %s", timestamp, entry.Message)
			a.appendLog(container.ID, logLine)
		}
	}
}

func (a *App) appendLog(containerID, message string) {
	a.app.QueueUpdateDraw(func() {
		a.mu.RLock()
		logView, exists := a.logViews[containerID]
		a.mu.RUnlock()
		
		if exists {
			fmt.Fprintf(logView, "%s\n", message)
			logView.ScrollToEnd()
		}
	})
}

func (a *App) setupKeyBindings() {
	a.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyCtrlC:
			a.cancel()
			a.app.Stop()
			return nil
		case tcell.KeyRune:
			switch event.Rune() {
			case 'q', 'Q':
				a.cancel()
				a.app.Stop()
				return nil
			case 'g', 'G':
				a.exportLogsForLLM()
				return nil
			}
		}
		return event
	})
}

func (a *App) exportLogsForLLM() {
	a.mu.RLock()
	containers := make([]Container, len(a.containers))
	copy(containers, a.containers)
	
	// Create a copy of recent logs to avoid holding the lock too long
	allLogs := make(map[string][]LogEntry)
	for containerID, logs := range a.recentLogs {
		logsCopy := make([]LogEntry, len(logs))
		copy(logsCopy, logs)
		allLogs[containerID] = logsCopy
	}
	a.mu.RUnlock()
	
	if len(allLogs) == 0 {
		return
	}
	
	// Format logs for LLM consumption
	output := "# Docker Container Logs Summary\n\n"
	output += fmt.Sprintf("Generated at: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))
	
	for _, container := range containers {
		logs, exists := allLogs[container.ID]
		if !exists || len(logs) == 0 {
			continue
		}
		
		output += fmt.Sprintf("## Container: %s\n", container.Name)
		output += fmt.Sprintf("- Image: %s\n", container.Image)
		output += fmt.Sprintf("- Status: %s\n", container.Status)
		output += fmt.Sprintf("- Log entries: %d\n\n", len(logs))
		
		output += "```\n"
		for _, log := range logs {
			timestamp := log.Timestamp.Format("2006-01-02 15:04:05")
			output += fmt.Sprintf("[%s] %s\n", timestamp, log.Message)
		}
		output += "```\n\n"
	}
	
	// Write to temporary file and copy to clipboard if available
	filename := fmt.Sprintf("/tmp/colog_logs_%d.md", time.Now().Unix())
	if err := os.WriteFile(filename, []byte(output), 0644); err == nil {
		// Try to copy to clipboard using pbcopy (macOS) or xclip (Linux)
		if err := exec.Command("pbcopy").Run(); err == nil {
			// pbcopy exists, use it
			cmd := exec.Command("pbcopy")
			cmd.Stdin = strings.NewReader(output)
			cmd.Run()
		} else if err := exec.Command("xclip", "-version").Run(); err == nil {
			// xclip exists, use it
			cmd := exec.Command("xclip", "-selection", "clipboard")
			cmd.Stdin = strings.NewReader(output)
			cmd.Run()
		}
		
		// Show notification in app
		go func() {
			a.app.QueueUpdateDraw(func() {
				// Find the first container's log view to show the message
				for _, container := range containers {
					if logView, exists := a.logViews[container.ID]; exists {
						fmt.Fprintf(logView, "[#FF8C00]📋 Logs exported to %s and clipboard[white]\n", filename)
						logView.ScrollToEnd()
						break
					}
				}
			})
		}()
	}
}

func getContainerColors() []tcell.Color {
	return []tcell.Color{
		tcell.NewRGBColor(255, 140, 0),   // Claude Orange
		tcell.NewRGBColor(255, 165, 50),  // Light Claude Orange
		tcell.NewRGBColor(200, 110, 0),   // Dark Claude Orange
		tcell.ColorWhite,
		tcell.ColorSilver,
		tcell.ColorGray,
		tcell.NewRGBColor(169, 169, 169), // DarkGray
		tcell.NewRGBColor(105, 105, 105), // DimGray
		tcell.NewRGBColor(64, 64, 64),    // Dark charcoal
		tcell.NewRGBColor(128, 128, 128), // Medium gray
		tcell.NewRGBColor(192, 192, 192), // Light gray
		tcell.NewRGBColor(245, 245, 245), // WhiteSmoke
		tcell.NewRGBColor(220, 220, 220), // Gainsboro
		tcell.NewRGBColor(211, 211, 211), // LightGray
	}
}

func colorToTviewColor(color tcell.Color) string {
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
		return "#FF8C00" // Claude Orange
	}
	if r == 255 && g == 165 && b == 50 {
		return "#FFA532" // Light Claude Orange
	}
	if r == 200 && g == 110 && b == 0 {
		return "#C86E00" // Dark Claude Orange
	}
	
	// For other grays, return a hex approximation
	if r == g && g == b { // Grayscale color
		return fmt.Sprintf("#%02X%02X%02X", r, g, b)
	}
	
	return "white"
}

func isTTY() bool {
	// Check if stdout is a terminal
	if fileInfo, _ := os.Stdout.Stat(); (fileInfo.Mode() & os.ModeCharDevice) != 0 {
		// Also try to open /dev/tty to ensure full TTY support
		if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
			tty.Close()
			return true
		}
	}
	return false
}

func (a *App) runSimpleMode() error {
	fmt.Println("Starting simple log output mode (press Ctrl+C to stop)...")
	fmt.Println(strings.Repeat("=", 60))

	// Set up signal handling for Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start streaming logs in simple text mode
	for _, container := range a.containers {
		go a.streamContainerLogsSimple(container)
	}

	// Wait for signal or context cancellation
	select {
	case <-sigChan:
		fmt.Println("\nReceived interrupt signal, shutting down...")
		a.cancel()
	case <-a.ctx.Done():
	}
	
	return nil
}

func (a *App) streamContainerLogsSimple(container Container) {
	fmt.Printf("\n=== %s (%s) ===\n", container.Name, container.ID)
	
	logCh := make(chan LogEntry, 100)
	
	err := a.dockerService.StreamLogs(a.ctx, container.ID, logCh)
	if err != nil {
		fmt.Printf("Error streaming logs for %s: %v\n", container.Name, err)
		return
	}

	for {
		select {
		case <-a.ctx.Done():
			return
		case entry, ok := <-logCh:
			if !ok {
				return
			}
			
			timestamp := entry.Timestamp.Format("15:04:05")
			fmt.Printf("[%s] %s: %s\n", timestamp, container.Name, entry.Message)
		}
	}
}