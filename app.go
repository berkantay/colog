package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"strings"
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
	contextManager *ContainerContextManager
	ctx           context.Context
	cancel        context.CancelFunc
	
	// Vim navigation state
	selectedContainer int  // currently focused container
	isFullscreen      bool // whether a container is in fullscreen mode
	
	// Help section for status messages
	helpText      string
}

func NewApp() *App {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &App{
		app:           tview.NewApplication(),
		grid:          tview.NewGrid(),
		mainGrid:      tview.NewGrid(),
		helpBar:       tview.NewTextView(),
		contextManager: NewContainerContextManager(),
		ctx:           ctx,
		cancel:        cancel,
		selectedContainer: 0,
		helpText:      "",
	}
}

func (a *App) Run() error {
	var err error
	a.dockerService, err = colog.NewDockerService()
	if err != nil {
		return fmt.Errorf("failed to connect to Docker: %w", err)
	}
	defer a.dockerService.Close()

	containers, err := a.dockerService.ListRunningContainers(a.ctx)
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	if len(containers) == 0 {
		return fmt.Errorf("no running containers found")
	}

	if err := a.contextManager.InitializeContexts(containers, a.dockerService, a.app); err != nil {
		return fmt.Errorf("failed to initialize container contexts: %w", err)
	}

	if err := a.setupUI(); err != nil {
		return err
	}

	a.setupGrid()
	a.setupHelpBar()
	a.setupMainLayout()
	a.setupKeyBindings()

	// Check if we have a proper TTY before starting the TUI
	if !isTTY() {
		fmt.Println("\nTTY not available, falling back to simple log output mode...")
		return a.runSimpleMode()
	}

	defer a.contextManager.Cleanup()
	
	if err := a.app.SetRoot(a.mainGrid, true).Run(); err != nil {
		return fmt.Errorf("failed to run TUI application: %w", err)
	}
	return nil
}

func (a *App) setupUI() error {
	trueBlack := tcell.NewRGBColor(0, 0, 0)
	a.grid.SetBorders(false).SetBackgroundColor(trueBlack)
	a.mainGrid.SetBackgroundColor(trueBlack)
	a.helpBar.SetBackgroundColor(trueBlack)
	return nil
}

func (a *App) showHelpMessage(message string, duration time.Duration) {
	a.helpText = message
	a.app.QueueUpdateDraw(func() {
		a.updateHelpBar()
	})
	
	go func() {
		time.Sleep(duration)
		a.helpText = ""
		a.app.QueueUpdateDraw(func() {
			a.updateHelpBar()
		})
	}()
}


func (a *App) setupGrid() {
	containerCount := a.contextManager.Count()
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

	contexts := a.contextManager.GetAllContexts()
	for i, context := range contexts {
		row := i / cols
		col := i % cols
		
		a.grid.AddItem(context.LogView, row, col, 1, 1, 0, 0, i == 0)
	}
	
	// Set initial focus
	if containerCount > 0 {
		a.focusContainer(0)
	}
}

func (a *App) setupHelpBar() {
	trueBlack := tcell.NewRGBColor(0, 0, 0)
	a.helpBar.SetDynamicColors(true).
		SetWrap(false).
		SetScrollable(false).
		SetBorder(true).
		SetBorderColor(tcell.ColorGray).
		SetTitle(" Shortcuts & Status ").
		SetBackgroundColor(trueBlack)

	a.updateHelpBar()
}

func (a *App) updateHelpBar() {
	baseText := "[#FF8C00]hjkl[white]: Navigate containers  [#FF8C00]Space[white]: Toggle fullscreen  [#FF8C00]y[white]: Export logs for LLM  [#FF8C00]q[white]: Quit  [#FF8C00]Ctrl+C[white]: Quit"
	
	if a.helpText != "" {
		text := baseText + "  " + a.helpText
		a.helpBar.SetText(text)
	} else {
		a.helpBar.SetText(baseText)
	}
}

func (a *App) setupMainLayout() {
	a.mainGrid.SetBorders(false).
		SetRows(0, 3).  // Main content takes available space, help bar takes 3 rows
		SetColumns(0).   // Single column
		AddItem(a.grid, 0, 0, 1, 1, 0, 0, true).    // Container grid takes row 0
		AddItem(a.helpBar, 1, 0, 1, 1, 0, 0, false)  // Help bar takes row 1
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
	containerCount := a.contextManager.Count()
	if containerCount == 0 {
		return
	}
	
	cols := int(math.Ceil(math.Sqrt(float64(containerCount))))
	currentCol := a.selectedContainer % cols
	if currentCol > 0 {
		a.selectedContainer--
		a.focusContainer(a.selectedContainer)
	}
}

func (a *App) navigateRight() {
	containerCount := a.contextManager.Count()
	if containerCount == 0 {
		return
	}
	
	cols := int(math.Ceil(math.Sqrt(float64(containerCount))))
	currentCol := a.selectedContainer % cols
	if currentCol < cols-1 && a.selectedContainer < containerCount-1 {
		a.selectedContainer++
		a.focusContainer(a.selectedContainer)
	}
}

func (a *App) navigateUp() {
	containerCount := a.contextManager.Count()
	if containerCount == 0 {
		return
	}
	
	cols := int(math.Ceil(math.Sqrt(float64(containerCount))))
	if a.selectedContainer >= cols {
		a.selectedContainer -= cols
		a.focusContainer(a.selectedContainer)
	}
}

func (a *App) navigateDown() {
	containerCount := a.contextManager.Count()
	if containerCount == 0 {
		return
	}
	
	cols := int(math.Ceil(math.Sqrt(float64(containerCount))))
	if a.selectedContainer < containerCount-cols {
		a.selectedContainer += cols
		a.focusContainer(a.selectedContainer)
	}
}

func (a *App) focusNextContainer() {
	containerCount := a.contextManager.Count()
	if containerCount == 0 {
		return
	}
	
	a.selectedContainer = (a.selectedContainer + 1) % containerCount
	a.focusContainer(a.selectedContainer)
}

func (a *App) scrollUp() {
	context := a.contextManager.GetContextByIndex(a.selectedContainer)
	if context != nil && context.LogView != nil {
		row, col := context.LogView.GetScrollOffset()
		if row > 0 {
			context.LogView.ScrollTo(row-1, col)
		}
	}
}

func (a *App) scrollDown() {
	context := a.contextManager.GetContextByIndex(a.selectedContainer)
	if context != nil && context.LogView != nil {
		row, col := context.LogView.GetScrollOffset()
		context.LogView.ScrollTo(row+1, col)
	}
}

func (a *App) focusContainer(index int) {
	containerCount := a.contextManager.Count()
	if index < 0 || index >= containerCount {
		return
	}
	
	// Update selection state for all contexts
	contexts := a.contextManager.GetAllContexts()
	for i, context := range contexts {
		context.SetSelected(i == index)
	}
	
	// Set focus on the selected context's log view
	selectedContext := a.contextManager.GetContextByIndex(index)
	if selectedContext != nil && selectedContext.LogView != nil {
		a.app.SetFocus(selectedContext.LogView)
	}
}

func (a *App) toggleFullscreen() {
	if a.contextManager.Count() == 0 {
		return
	}
	
	a.isFullscreen = !a.isFullscreen
	
	if a.isFullscreen {
		// Enter fullscreen mode - show only the selected container
		a.mainGrid.Clear()
		selectedContext := a.contextManager.GetContextByIndex(a.selectedContainer)
		if selectedContext != nil && selectedContext.LogView != nil {
			a.mainGrid.SetRows(0, 3).
				SetColumns(0).
				AddItem(selectedContext.LogView, 0, 0, 1, 1, 0, 0, true).
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
	// Run export in background to avoid blocking the UI
	go func() {
		contexts := a.contextManager.GetAllContexts()
		if len(contexts) == 0 {
			a.showHelpMessage("[red]No containers available for export[white]", 2*time.Second)
			return
		}
		
		// Collect logs from all contexts
		allLogs := make(map[string][]colog.LogEntry)
		var containers []colog.Container
		
		for _, context := range contexts {
			logBuffer := context.GetLogBuffer()
			if len(logBuffer) > 0 {
				allLogs[context.Container.ID] = logBuffer
				containers = append(containers, context.Container)
			}
		}
		
		if len(allLogs) == 0 {
			a.showHelpMessage("[red]No logs available for export[white]", 2*time.Second)
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
			clipboardSuccess := false
			if err := exec.Command("pbcopy").Run(); err == nil {
				// pbcopy exists, use it
				cmd := exec.Command("pbcopy")
				cmd.Stdin = strings.NewReader(output)
				if cmd.Run() == nil {
					clipboardSuccess = true
				}
			} else if err := exec.Command("xclip", "-version").Run(); err == nil {
				// xclip exists, use it
				cmd := exec.Command("xclip", "-selection", "clipboard")
				cmd.Stdin = strings.NewReader(output)
				if cmd.Run() == nil {
					clipboardSuccess = true
				}
			}
			
			if clipboardSuccess {
				a.showHelpMessage("[#00FF00]📋 Logs copied to clipboard[white]", 3*time.Second)
			} else {
				a.showHelpMessage(fmt.Sprintf("[#FFA500]📄 Logs saved to %s[white]", filename), 3*time.Second)
			}
		} else {
			a.showHelpMessage("[red]❌ Failed to export logs[white]", 2*time.Second)
		}
	}()
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
	contexts := a.contextManager.GetAllContexts()
	for _, context := range contexts {
		go a.streamContainerLogsSimple(context)
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

func (a *App) streamContainerLogsSimple(context *ContainerContext) {
	container := context.Container
	fmt.Printf("\n=== %s (%s) ===\n", container.Name, container.ID)
	
	for {
		select {
		case <-a.ctx.Done():
			return
		case entry, ok := <-context.LogChannel:
			if !ok {
				return
			}
			
			timestamp := entry.Timestamp.Format("15:04:05")
			fmt.Printf("[%s] %s: %s\n", timestamp, container.Name, entry.Message)
		}
	}
}