package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/berkantay/colog/internal/docker"
	"github.com/berkantay/colog/internal/container"
)

type App struct {
	app           *tview.Application
	grid          *tview.Grid
	mainGrid      *tview.Grid
	helpBar       *tview.TextView
	dockerService *docker.DockerService
	contextManager *container.ContainerContextManager
	ctx           context.Context
	cancel        context.CancelFunc
	
	// Vim navigation state
	selectedContainer int  // currently focused container
	isFullscreen      bool // whether a container is in fullscreen mode
	
	// Search mode
	searchMode    bool               // whether we're in search mode
	searchInput   *tview.InputField  // search input field
	searchResults *tview.TextView    // search results display
	
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
		contextManager: container.NewContainerContextManager(),
		ctx:           ctx,
		cancel:        cancel,
		selectedContainer: 0,
		helpText:      "",
	}
}

func (a *App) Run() error {
	var err error
	a.dockerService, err = docker.NewDockerService()
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

	a.grid.Clear()

	// Create row-based list layout - all containers in a single column
	rowSizes := make([]int, containerCount)
	for i := range rowSizes {
		rowSizes[i] = 0 // Equal height for all rows
	}

	a.grid.SetRows(rowSizes...).SetColumns(0) // Single column

	contexts := a.contextManager.GetAllContexts()
	for i, context := range contexts {
		a.grid.AddItem(context.LogView, i, 0, 1, 1, 0, 0, i == 0)
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
	var baseText string
	if a.searchMode {
		baseText = "[#FF8C00]ESC[white]: Exit search  [#FF8C00]Type[white]: Search across all logs"
	} else {
		baseText = "[#FF8C00]hjkl[white]: Navigate containers  [#FF8C00]Space[white]: Toggle fullscreen  [#FF8C00]/[white]: Search logs  [#FF8C00]y[white]: Export logs for LLM  [#FF8C00]q[white]: Quit  [#FF8C00]Ctrl+C[white]: Quit"
	}
	
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
			case 'r':
				a.restartFocusedContainer()
				return nil
			case 'x':
				a.killFocusedContainer()
				return nil
			case '/':
				a.toggleSearchMode()
				return nil
			}
		}
		return event
	})
}

func (a *App) navigateLeft() {
	// In row list layout, left navigation is not applicable
}

func (a *App) navigateRight() {
	// In row list layout, right navigation is not applicable
}

func (a *App) navigateUp() {
	containerCount := a.contextManager.Count()
	if containerCount == 0 {
		return
	}
	
	if a.selectedContainer > 0 {
		a.selectedContainer--
		a.focusContainer(a.selectedContainer)
	}
}

func (a *App) navigateDown() {
	containerCount := a.contextManager.Count()
	if containerCount == 0 {
		return
	}
	
	if a.selectedContainer < containerCount-1 {
		a.selectedContainer++
		a.focusContainer(a.selectedContainer)
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
		allLogs := make(map[string][]docker.LogEntry)
		var containers []docker.Container
		
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


func (a *App) restartFocusedContainer() {
	if a.contextManager.Count() == 0 {
		a.showHelpMessage("[red]No containers available[white]", 2*time.Second)
		return
	}

	selectedContext := a.contextManager.GetContextByIndex(a.selectedContainer)
	if selectedContext == nil {
		a.showHelpMessage("[red]No container selected[white]", 2*time.Second)
		return
	}

	containerName := selectedContext.Container.Name
	containerID := selectedContext.Container.ID
	
	a.showHelpMessage(fmt.Sprintf("[yellow]Restarting %s...[white]", containerName), 1*time.Second)
	
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		
		if err := a.dockerService.RestartContainer(ctx, containerID); err != nil {
			a.app.QueueUpdateDraw(func() {
				a.showHelpMessage(fmt.Sprintf("[red]Failed to restart %s: %v[white]", containerName, err), 3*time.Second)
			})
		} else {
			a.app.QueueUpdateDraw(func() {
				a.showHelpMessage(fmt.Sprintf("[green]✓ Restarted %s[white]", containerName), 2*time.Second)
				// Refresh containers after restart
				a.refreshContainers()
			})
		}
	}()
}

func (a *App) killFocusedContainer() {
	if a.contextManager.Count() == 0 {
		a.showHelpMessage("[red]No containers available[white]", 2*time.Second)
		return
	}

	selectedContext := a.contextManager.GetContextByIndex(a.selectedContainer)
	if selectedContext == nil {
		a.showHelpMessage("[red]No container selected[white]", 2*time.Second)
		return
	}

	containerName := selectedContext.Container.Name
	containerID := selectedContext.Container.ID
	
	a.showHelpMessage(fmt.Sprintf("[red]Killing %s...[white]", containerName), 1*time.Second)
	
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		
		if err := a.dockerService.KillContainer(ctx, containerID); err != nil {
			a.app.QueueUpdateDraw(func() {
				a.showHelpMessage(fmt.Sprintf("[red]Failed to kill %s: %v[white]", containerName, err), 3*time.Second)
			})
		} else {
			a.app.QueueUpdateDraw(func() {
				a.showHelpMessage(fmt.Sprintf("[red]✗ Killed %s[white]", containerName), 2*time.Second)
				// Refresh containers after kill - this will remove dead containers
				a.refreshContainers()
			})
		}
	}()
}

// refreshContainers re-fetches the container list and updates the UI
func (a *App) refreshContainers() {
	go func() {
		// Get fresh container list
		containers, err := a.dockerService.ListRunningContainers(a.ctx)
		if err != nil {
			a.app.QueueUpdateDraw(func() {
				a.showHelpMessage(fmt.Sprintf("[red]Failed to refresh containers: %v[white]", err), 3*time.Second)
			})
			return
		}

		a.app.QueueUpdateDraw(func() {
			// Stop all existing contexts
			a.contextManager.StopAll()
			
			// Clear the grid
			a.grid.Clear()
			
			if len(containers) == 0 {
				a.showHelpMessage("[yellow]No running containers found[white]", 3*time.Second)
				return
			}
			
			// Reinitialize contexts with fresh container list
			if err := a.contextManager.InitializeContexts(containers, a.dockerService, a.app); err != nil {
				a.showHelpMessage(fmt.Sprintf("[red]Failed to reinitialize contexts: %v[white]", err), 3*time.Second)
				return
			}
			
			// Adjust selected container index if needed
			if a.selectedContainer >= len(containers) {
				a.selectedContainer = len(containers) - 1
			}
			if a.selectedContainer < 0 {
				a.selectedContainer = 0
			}
			
			// Re-setup the grid layout
			a.setupGrid()
			
			// Update focus
			if len(containers) > 0 {
				a.focusContainer(a.selectedContainer)
			}
		})
	}()
}

// toggleSearchMode toggles search mode on/off
func (a *App) toggleSearchMode() {
	if a.searchMode {
		// Exit search mode - restore normal layout
		a.searchMode = false
		
		if a.isFullscreen {
			a.toggleFullscreen() // Exit fullscreen
			a.toggleFullscreen() // Re-enter fullscreen to reset
		} else {
			a.setupMainLayout()
		}
		
		// Restore focus
		a.focusContainer(a.selectedContainer)
	} else {
		// Enter search mode
		a.searchMode = true
		a.setupSearchLayout()
	}
}

// setupSearchLayout creates the search interface
func (a *App) setupSearchLayout() {
	trueBlack := tcell.NewRGBColor(0, 0, 0)
	
	// Create search input if it doesn't exist
	if a.searchInput == nil {
		a.searchInput = tview.NewInputField().
			SetLabel("Search: ").
			SetLabelColor(tcell.ColorWhite).
			SetFieldBackgroundColor(trueBlack).
			SetFieldTextColor(tcell.ColorWhite)
		
		// Handle input changes
		a.searchInput.SetChangedFunc(func(text string) {
			a.performSearch(text)
		})
		
		// Handle Escape key to exit search
		a.searchInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if event.Key() == tcell.KeyEscape {
				a.toggleSearchMode()
				return nil
			}
			return event
		})
	}
	
	// Create search results if it doesn't exist  
	if a.searchResults == nil {
		a.searchResults = tview.NewTextView().
			SetDynamicColors(true).
			SetScrollable(true).
			SetWrap(true)
		
		a.searchResults.SetBackgroundColor(trueBlack)
		a.searchResults.SetBorder(true).
			SetBorderColor(tcell.NewRGBColor(128, 0, 128)).
			SetTitle(" Search Results - ESC to exit ")
	}
	
	// Set initial text
	a.searchResults.SetText("Enter search term...")
	
	// Setup search layout (same pattern as fullscreen)
	a.mainGrid.Clear()
	a.mainGrid.SetRows(3, 0, 3). // Search input, results, help bar
		SetColumns(0).
		AddItem(a.searchInput, 0, 0, 1, 1, 0, 0, true).
		AddItem(a.searchResults, 1, 0, 1, 1, 0, 0, false).
		AddItem(a.helpBar, 2, 0, 1, 1, 0, 0, false)
	
	// Focus search input
	a.app.SetFocus(a.searchInput)
}

// performSearch searches logs synchronously (like exportLogsForLLM)
func (a *App) performSearch(searchTerm string) {
	if searchTerm == "" {
		a.searchResults.SetText("Enter search term...")
		return
	}
	
	contexts := a.contextManager.GetAllContexts()
	if len(contexts) == 0 {
		a.searchResults.SetText("No containers available for search")
		return
	}
	
	var results []string
	searchTermLower := strings.ToLower(searchTerm)
	
	// Search through all container logs (simple synchronous approach)
	for _, context := range contexts {
		logBuffer := context.GetLogBuffer()
		containerMatches := []string{}
		
		for _, logEntry := range logBuffer {
			if strings.Contains(strings.ToLower(logEntry.Message), searchTermLower) {
				// Highlight matches in purple
				highlightedMessage := a.highlightSearchTerm(logEntry.Message, searchTerm)
				timestamp := logEntry.Timestamp.Format("15:04:05")
				matchLine := fmt.Sprintf("[gray]%s[white] %s", timestamp, highlightedMessage)
				containerMatches = append(containerMatches, matchLine)
			}
		}
		
		if len(containerMatches) > 0 {
			containerHeader := fmt.Sprintf("[orange]Container: %s (%d matches)[white]", context.Container.Name, len(containerMatches))
			results = append(results, containerHeader)
			results = append(results, containerMatches...)
			results = append(results, "") // Empty line between containers
		}
	}
	
	// Update results
	if len(results) == 0 {
		a.searchResults.SetText(fmt.Sprintf("No matches found for: %s", searchTerm))
	} else {
		a.searchResults.SetText(strings.Join(results, "\n"))
		a.searchResults.ScrollToBeginning()
	}
}

// highlightSearchTerm adds purple highlighting (simple string replacement)
func (a *App) highlightSearchTerm(text, searchTerm string) string {
	if searchTerm == "" {
		return text
	}
	
	// Case-insensitive replacement with purple highlighting
	searchLower := strings.ToLower(searchTerm)
	textLower := strings.ToLower(text)
	
	var result strings.Builder
	lastIndex := 0
	
	for {
		index := strings.Index(textLower[lastIndex:], searchLower)
		if index == -1 {
			result.WriteString(text[lastIndex:])
			break
		}
		
		index += lastIndex
		result.WriteString(text[lastIndex:index])
		
		originalMatch := text[index : index+len(searchTerm)]
		result.WriteString(fmt.Sprintf("[purple]%s[white]", originalMatch))
		
		lastIndex = index + len(searchTerm)
	}
	
	return result.String()
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

func (a *App) streamContainerLogsSimple(context *container.ContainerContext) {
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