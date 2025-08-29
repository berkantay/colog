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
	
	"github.com/berkantay/colog/pkg/colog"
)

type App struct {
	app           *tview.Application
	grid          *tview.Grid
	mainGrid      *tview.Grid
	helpBar       *tview.TextView
	dockerService *colog.DockerService
	logViews      map[string]*tview.TextView
	containers    []colog.Container
	colors        []tcell.Color
	colorIndex    int
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	recentLogs    map[string][]colog.LogEntry
	
	// Vim navigation state
	selectedContainer int  // currently focused container
	isFullscreen      bool // whether a container is in fullscreen mode
	
	// Notification overlay
	notification  *tview.TextView
	overlayGrid   *tview.Grid
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
		recentLogs: make(map[string][]colog.LogEntry),
		selectedContainer: 0,
		notification: tview.NewTextView(),
		overlayGrid: tview.NewGrid(),
	}
}

func (a *App) Run() error {
	var err error
	a.dockerService, err = colog.NewDockerService()
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
	a.grid.SetBorders(false)

	a.setupNotificationOverlay()

	return nil
}

func (a *App) setupNotificationOverlay() {
	a.notification.
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetBorder(true).
		SetBorderColor(tcell.NewRGBColor(255, 248, 235)).
		SetBackgroundColor(tcell.ColorBlack)

	a.overlayGrid.
		SetBorders(false).
		SetRows(0, 5, 0).
		SetColumns(0, 50, 0).
		AddItem(tview.NewBox(), 0, 0, 1, 3, 0, 0, false).
		AddItem(tview.NewBox(), 1, 0, 1, 1, 0, 0, false).
		AddItem(a.notification, 1, 1, 1, 1, 0, 0, false).
		AddItem(tview.NewBox(), 1, 2, 1, 1, 0, 0, false).
		AddItem(tview.NewBox(), 2, 0, 1, 3, 0, 0, false)
}

func (a *App) showNotification(message string, duration time.Duration) {
	a.app.QueueUpdateDraw(func() {
		a.notification.SetText(message)
		a.app.SetRoot(a.overlayGrid, true)
	})
	
	go func() {
		time.Sleep(duration)
		a.app.QueueUpdateDraw(func() {
			a.app.SetRoot(a.mainGrid, true)
		})
	}()
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
	
	// Set initial focus
	if len(a.containers) > 0 {
		a.focusContainer(0)
	}
}

func (a *App) setupHelpBar() {
	a.helpBar.SetDynamicColors(true).
		SetWrap(false).
		SetScrollable(false).
		SetBorder(true).
		SetBorderColor(tcell.ColorGray).
		SetTitle(" Vim Shortcuts ")

	helpText := "[#FF8C00]hjkl[white]: Navigate containers  [#FF8C00]Space[white]: Toggle fullscreen  [#FF8C00]y[white]: Export logs for LLM  [#FF8C00]q[white]: Quit  [#FF8C00]Ctrl+C[white]: Quit"
	a.helpBar.SetText(helpText)
}

func (a *App) setupMainLayout() {
	a.mainGrid.SetBorders(false).
		SetRows(0, 3).  // Main content takes available space, help bar takes 3 rows
		SetColumns(0).   // Single column
		AddItem(a.grid, 0, 0, 1, 1, 0, 0, true).    // Container grid takes row 0
		AddItem(a.helpBar, 1, 0, 1, 1, 0, 0, false)  // Help bar takes row 1
}

func (a *App) createLogView(container colog.Container) *tview.TextView {
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
	containers := make([]colog.Container, len(a.containers))
	copy(containers, a.containers)
	a.mu.RUnlock()

	for _, container := range containers {
		go a.streamContainerLogs(container)
	}
}

func (a *App) streamContainerLogs(container colog.Container) {
	logCh := make(chan colog.LogEntry, 100)
	
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
				a.recentLogs[container.ID] = make([]colog.LogEntry, 0, 50)
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
			case 'h':
				a.navigateLeft()
				return nil
			case 'j':
				a.navigateDown()
				return nil
			case 'k':
				a.navigateUp()
				return nil
			case 'l':
				a.navigateRight()
				return nil
			case 'y':
				a.exportLogsForLLM()
				return nil
			case ' ':
				a.toggleFullscreen()
				return nil
			}
		}
		return event
	})
}

func (a *App) navigateLeft() {
	if len(a.containers) == 0 {
		return
	}
	
	cols := int(math.Ceil(math.Sqrt(float64(len(a.containers)))))
	currentCol := a.selectedContainer % cols
	if currentCol > 0 {
		a.selectedContainer--
		a.focusContainer(a.selectedContainer)
	}
}

func (a *App) navigateRight() {
	if len(a.containers) == 0 {
		return
	}
	
	cols := int(math.Ceil(math.Sqrt(float64(len(a.containers)))))
	currentCol := a.selectedContainer % cols
	if currentCol < cols-1 && a.selectedContainer < len(a.containers)-1 {
		a.selectedContainer++
		a.focusContainer(a.selectedContainer)
	}
}

func (a *App) navigateUp() {
	if len(a.containers) == 0 {
		return
	}
	
	cols := int(math.Ceil(math.Sqrt(float64(len(a.containers)))))
	if a.selectedContainer >= cols {
		a.selectedContainer -= cols
		a.focusContainer(a.selectedContainer)
	}
}

func (a *App) navigateDown() {
	if len(a.containers) == 0 {
		return
	}
	
	cols := int(math.Ceil(math.Sqrt(float64(len(a.containers)))))
	if a.selectedContainer < len(a.containers)-cols {
		a.selectedContainer += cols
		a.focusContainer(a.selectedContainer)
	}
}

func (a *App) focusNextContainer() {
	if len(a.containers) == 0 {
		return
	}
	
	a.selectedContainer = (a.selectedContainer + 1) % len(a.containers)
	a.focusContainer(a.selectedContainer)
}

func (a *App) scrollUp() {
	if a.selectedContainer >= 0 && a.selectedContainer < len(a.containers) {
		containerID := a.containers[a.selectedContainer].ID
		if logView, exists := a.logViews[containerID]; exists {
			row, col := logView.GetScrollOffset()
			if row > 0 {
				logView.ScrollTo(row-1, col)
			}
		}
	}
}

func (a *App) scrollDown() {
	if a.selectedContainer >= 0 && a.selectedContainer < len(a.containers) {
		containerID := a.containers[a.selectedContainer].ID
		if logView, exists := a.logViews[containerID]; exists {
			row, col := logView.GetScrollOffset()
			logView.ScrollTo(row+1, col)
		}
	}
}

func (a *App) focusContainer(index int) {
	if index < 0 || index >= len(a.containers) {
		return
	}
	
	containerID := a.containers[index].ID
	if logView, exists := a.logViews[containerID]; exists {
		a.app.SetFocus(logView)
		
		// Update border color to indicate focus
		for i, container := range a.containers {
			if view, ok := a.logViews[container.ID]; ok {
				if i == index {
					view.SetBorderColor(tcell.NewRGBColor(255, 140, 0)) // Bright orange for focus
				} else {
					view.SetBorderColor(a.colors[i%len(a.colors)])
				}
			}
		}
	}
}

func (a *App) toggleFullscreen() {
	if len(a.containers) == 0 {
		return
	}
	
	a.isFullscreen = !a.isFullscreen
	
	if a.isFullscreen {
		// Enter fullscreen mode - show only the selected container
		a.mainGrid.Clear()
		containerID := a.containers[a.selectedContainer].ID
		if logView, exists := a.logViews[containerID]; exists {
			a.mainGrid.SetRows(0, 3).
				SetColumns(0).
				AddItem(logView, 0, 0, 1, 1, 0, 0, true).
				AddItem(a.helpBar, 1, 0, 1, 1, 0, 0, false)
		}
	} else {
		// Exit fullscreen mode - restore grid layout
		a.mainGrid.Clear()
		a.mainGrid.SetRows(0, 3).
			SetColumns(0).
			AddItem(a.grid, 0, 0, 1, 1, 0, 0, true).
			AddItem(a.helpBar, 1, 0, 1, 1, 0, 0, false)
		
		// Restore focus to the selected container
		a.focusContainer(a.selectedContainer)
	}
}


func (a *App) exportLogsForLLM() {
	a.mu.RLock()
	containers := make([]colog.Container, len(a.containers))
	copy(containers, a.containers)
	
	// Create a copy of recent logs to avoid holding the lock too long
	allLogs := make(map[string][]colog.LogEntry)
	for containerID, logs := range a.recentLogs {
		logsCopy := make([]colog.LogEntry, len(logs))
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
		
		// Show centered notification
		a.showNotification("[#00FF00]📋 Copied to clipboard[white]", 2*time.Second)
	}
}

func getContainerColors() []tcell.Color {
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

func (a *App) streamContainerLogsSimple(container colog.Container) {
	fmt.Printf("\n=== %s (%s) ===\n", container.Name, container.ID)
	
	logCh := make(chan colog.LogEntry, 100)
	
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