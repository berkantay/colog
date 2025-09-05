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
	"github.com/berkantay/colog/internal/ai"
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
	
	// Search modes
	searchMode       bool               // whether we're in literal search mode
	aiSearchMode     bool               // whether we're in AI semantic search mode
	chatMode         bool               // whether we're in AI chat mode
	searchInput      *tview.InputField  // search input field
	searchResults    *tview.TextView    // search results display
	chatHistory      []string           // chat conversation history
	
	// AI service
	aiService        *ai.AIService      // AI service for semantic search and chat
	
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

	// Initialize AI service (optional - will show message if API key not set)
	a.aiService, err = ai.NewAIService()
	if err != nil {
		fmt.Printf("AI features disabled: %v\n", err)
		fmt.Println("Create a .env file with: OPENAI_API_KEY=your-openai-api-key")
	}

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
	} else if a.aiSearchMode {
		baseText = "[#FF8C00]ESC[white]: Exit AI search  [#FF8C00]Type[white]: AI semantic search (powered by GPT-4o-mini)"
	} else if a.chatMode {
		baseText = "[#FF8C00]ESC[white]: Exit chat  [#FF8C00]Type[white]: Chat with your logs (powered by GPT-4o)"
	} else {
		aiHint := ""
		if a.aiService != nil {
			aiHint = "  [#FF8C00]?[white]: AI search  [#FF8C00]C[white]: AI chat"
		}
		baseText = "[#FF8C00]hjkl[white]: Navigate containers  [#FF8C00]Space[white]: Toggle fullscreen  [#FF8C00]/[white]: Search logs" + aiHint + "  [#FF8C00]y[white]: Export logs for LLM  [#FF8C00]q[white]: Quit  [#FF8C00]Ctrl+C[white]: Quit"
	}
	
	if a.helpText != "" {
		text := baseText + "  " + a.helpText
		a.helpBar.SetText(text)
	} else {
		a.helpBar.SetText(baseText)
	}
}

func (a *App) setupMainLayout() {
	// Clear existing layout completely and reset to normal 2-row layout
	a.mainGrid.Clear()
	a.mainGrid.SetBorders(false).
		SetRows(0, 3).  // Main content takes available space, help bar takes 3 rows
		SetColumns(0).   // Single column
		AddItem(a.grid, 0, 0, 1, 1, 0, 0, true).    // Container grid takes row 0
		AddItem(a.helpBar, 1, 0, 1, 1, 0, 0, false)  // Help bar takes row 1
}



func (a *App) setupKeyBindings() {
	a.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// When in search mode, only allow Ctrl+C and ESC to work
		// All other keys should be handled by the search input field
		if a.searchMode || a.aiSearchMode || a.chatMode {
			switch event.Key() {
			case tcell.KeyCtrlC:
				a.cancel()
				a.app.Stop()
				return nil
			}
			// Pass all other events to the focused component (search input)
			return event
		}
		
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
			case '?':
				a.toggleAISearchMode()
				return nil
			case 'C':
				a.toggleChatMode()
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
	
	// Show immediate feedback
	a.showHelpMessage(fmt.Sprintf("[yellow]Restarting %s...[white]", containerName), 3*time.Second)
	
	// Use a channel to communicate result back to main thread instead of QueueUpdateDraw from goroutine
	resultChan := make(chan error, 1)
	
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		
		err := a.dockerService.RestartContainer(ctx, containerID)
		resultChan <- err
		close(resultChan)
	}()
	
	// Handle result in main thread without blocking
	go func() {
		err := <-resultChan
		
		// Use a simple approach - append to the container's log instead of help message
		message := ""
		if err != nil {
			message = fmt.Sprintf("[red]RESTART FAILED: %v[white]", err)
		} else {
			message = fmt.Sprintf("[green]RESTART SUCCESS: %s restarted[white]", containerName)
		}
		
		// Add result to the selected container's log stream - this avoids QueueUpdateDraw conflicts
		if selectedContext.LogView != nil {
			selectedContext.AppendLog(message)
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
			})
		}
	}()
}

// toggleSearchMode toggles literal search mode on/off
func (a *App) toggleSearchMode() {
	if a.searchMode || a.aiSearchMode || a.chatMode {
		// Exit any active mode - restore normal layout
		a.searchMode = false
		a.aiSearchMode = false
		a.chatMode = false
		
		// Clear search input text for clean state
		if a.searchInput != nil {
			a.searchInput.SetText("")
		}
		
		// Simply restore the original layout (streams are preserved)
		a.setupMainLayout()
		
		// Update help bar and restore focus
		a.updateHelpBar()
		a.focusContainer(a.selectedContainer)
	} else {
		// Enter literal search mode
		a.searchMode = true
		a.setupSearchLayout("Search")
	}
}

// toggleAISearchMode toggles AI semantic search mode on/off
func (a *App) toggleAISearchMode() {
	if a.aiService == nil {
		a.showHelpMessage("[red]AI features disabled - create a .env file with OPENAI_API_KEY[white]", 3*time.Second)
		return
	}

	if a.searchMode || a.aiSearchMode || a.chatMode {
		// Exit any active mode - restore normal layout
		a.searchMode = false
		a.aiSearchMode = false
		a.chatMode = false
		
		// Clear search input text for clean state
		if a.searchInput != nil {
			a.searchInput.SetText("")
		}
		
		// Simply restore the original layout (streams are preserved)
		a.setupMainLayout()
		
		// Update help bar and restore focus
		a.updateHelpBar()
		a.focusContainer(a.selectedContainer)
	} else {
		// Enter AI search mode
		a.aiSearchMode = true
		a.setupSearchLayout("AI Search")
	}
}

// toggleChatMode toggles AI chat mode on/off
func (a *App) toggleChatMode() {
	if a.aiService == nil {
		a.showHelpMessage("[red]AI features disabled - create a .env file with OPENAI_API_KEY[white]", 3*time.Second)
		return
	}

	if a.searchMode || a.aiSearchMode || a.chatMode {
		// Exit any active mode - restore normal layout
		a.searchMode = false
		a.aiSearchMode = false
		a.chatMode = false
		
		// Clear search input text for clean state
		if a.searchInput != nil {
			a.searchInput.SetText("")
		}
		
		// Simply restore the original layout (streams are preserved)
		a.setupMainLayout()
		
		// Update help bar and restore focus
		a.updateHelpBar()
		a.focusContainer(a.selectedContainer)
	} else {
		// Enter chat mode
		a.chatMode = true
		a.setupSearchLayout("AI Chat")
	}
}

// setupSearchLayout creates the search interface as overlay
func (a *App) setupSearchLayout(mode string) {
	trueBlack := tcell.NewRGBColor(0, 0, 0)
	
	// Create search input if it doesn't exist
	if a.searchInput == nil {
		a.searchInput = tview.NewInputField().
			SetLabelColor(tcell.ColorWhite).
			SetFieldBackgroundColor(trueBlack).
			SetFieldTextColor(tcell.ColorWhite)
		
		// Handle Escape key to exit search
		a.searchInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if event.Key() == tcell.KeyEscape {
				a.toggleSearchMode()
				return nil
			}
			return event
		})
	}
	
	// Update label and handler based on mode
	if mode == "AI Search" {
		a.searchInput.SetLabel("AI Search: ")
		a.searchInput.SetChangedFunc(func(text string) {
			// AI Search mode processes on Enter, not on change
		})
		a.searchInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if event.Key() == tcell.KeyEscape {
				a.toggleSearchMode()
				return nil
			} else if event.Key() == tcell.KeyEnter {
				text := a.searchInput.GetText()
				if text != "" {
					a.performAISearch(text)
					a.searchInput.SetText("")
				}
				return nil
			}
			return event
		})
	} else if mode == "AI Chat" {
		a.searchInput.SetLabel("Chat: ")
		a.searchInput.SetChangedFunc(func(text string) {
			// Chat mode processes on Enter, not on change
		})
		a.searchInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if event.Key() == tcell.KeyEscape {
				a.toggleSearchMode()
				return nil
			} else if event.Key() == tcell.KeyEnter {
				text := a.searchInput.GetText()
				if text != "" {
					a.performAIChat(text)
					a.searchInput.SetText("")
				}
				return nil
			}
			return event
		})
	} else {
		a.searchInput.SetLabel("Search: ")
		a.searchInput.SetChangedFunc(func(text string) {
			a.performSearch(text)
		})
	}
	
	// Create search results if it doesn't exist  
	if a.searchResults == nil {
		a.searchResults = tview.NewTextView().
			SetDynamicColors(true).
			SetScrollable(true).
			SetWrap(true)
		
		a.searchResults.SetBackgroundColor(trueBlack)
		a.searchResults.SetBorder(true)
	}
	
	// Update border color and title based on mode
	if mode == "AI Search" {
		a.searchResults.SetBorderColor(tcell.NewRGBColor(0, 255, 127)). // Green for AI
			SetTitle(" AI Semantic Search Results - ESC to exit ")
		a.searchResults.SetText("Enter query for AI-powered semantic search...")
	} else if mode == "AI Chat" {
		a.searchResults.SetBorderColor(tcell.NewRGBColor(64, 224, 255)). // Blue for chat
			SetTitle(" AI Chat - Press Enter to send, ESC to exit ")
		a.searchResults.SetText("Ask questions about your logs. GPT-4o will analyze them for you...")
	} else {
		a.searchResults.SetBorderColor(tcell.NewRGBColor(128, 0, 128)). // Purple for regular search
			SetTitle(" Search Results - ESC to exit ")
		a.searchResults.SetText("Enter search term...")
	}
	
	// KEEP EXISTING GRID INTACT - just add search overlay on top
	// Change layout to: [search input] [original grid] [search results] [help bar]
	a.mainGrid.Clear()
	a.mainGrid.SetRows(3, 0, 8, 3). // Search input, original grid, search results, help bar
		SetColumns(0).
		AddItem(a.searchInput, 0, 0, 1, 1, 0, 0, true).
		AddItem(a.grid, 1, 0, 1, 1, 0, 0, false).        // Keep original streaming grid
		AddItem(a.searchResults, 2, 0, 1, 1, 0, 0, false).
		AddItem(a.helpBar, 3, 0, 1, 1, 0, 0, false)
	
	// Focus search input
	a.app.SetFocus(a.searchInput)
	
	// Update help bar
	a.updateHelpBar()
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

// performAISearch performs AI-powered semantic search
func (a *App) performAISearch(query string) {
	logs := a.getAllLogs()
	if len(logs) == 0 {
		a.app.QueueUpdateDraw(func() {
			a.searchResults.SetText("[red]No logs available for AI search[white]")
		})
		return
	}

	// Perform AI search in background to avoid blocking UI
	go func() {
		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		
		// Start loading animation
		loadingDone := make(chan bool, 1)
		go func() {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			frame := 0
			starFrames := []string{
				"[cyan]✢[white]", "[blue]✣[white]", "[yellow]✤[white]", "[magenta]✥[white]",
				"[green]✦[white]", "[red]✧[white]", "[cyan]✩[white]", "[blue]✪[white]",
			}
			
			for {
				select {
				case <-loadingDone:
					return
				case <-ticker.C:
					currentStar := starFrames[frame%len(starFrames)]
					frame++
					
					a.app.QueueUpdateDraw(func() {
						a.searchResults.SetText(fmt.Sprintf("%s Analyzing logs with AI for: [green]%s[white]\n\n[cyan]Processing with GPT-4o-mini...[white]", currentStar, query))
						a.searchResults.ScrollToEnd()
					})
					a.app.ForceDraw()
				}
			}
		}()
		
		// Perform the AI search
		results, err := a.aiService.SemanticSearch(ctx, query, logs)
		loadingDone <- true
		
		// Display results
		a.app.QueueUpdateDraw(func() {
			if err != nil {
				a.searchResults.SetText(fmt.Sprintf("[red]AI Search Error: %v[white]", err))
				return
			}
			
			// Clear and show clean results
			var output strings.Builder
			output.WriteString(fmt.Sprintf("AI Semantic Search Results for: [green]%s[white]\n\n", query))
			
			if len(results) == 0 {
				output.WriteString("[gray]No semantic matches found for this query.[white]")
			} else {
				for i, result := range results {
					output.WriteString(fmt.Sprintf("[green]%d. Container: %s[white] ([yellow]%s[white])\n", i+1, result.Container, result.Relevance))
					output.WriteString(fmt.Sprintf("   [gray]%s[white] %s\n", result.LogEntry.Timestamp.Format("15:04:05"), result.LogEntry.Message))
					if result.Explanation != "" {
						output.WriteString(fmt.Sprintf("   [cyan]%s[white]\n", result.Explanation))
					}
					output.WriteString("\n")
				}
			}
			
			a.searchResults.SetText(output.String())
			a.searchResults.ScrollToEnd()
		})
		a.app.ForceDraw()
	}()
}

// getAllLogs collects logs from all containers
func (a *App) getAllLogs() map[string][]docker.LogEntry {
	contexts := a.contextManager.GetAllContexts()
	logs := make(map[string][]docker.LogEntry)
	for _, context := range contexts {
		logBuffer := context.GetLogBuffer()
		if len(logBuffer) > 0 {
			logs[context.Container.Name] = logBuffer
		}
	}
	return logs
}

// performAIChat performs AI-powered chat analysis
func (a *App) performAIChat(query string) {
	if query == "" {
		return
	}
	
	if a.aiService == nil {
		a.searchResults.SetText("[red]AI service not available - set OPENAI_API_KEY environment variable[white]")
		return
	}
	
	// Add user message to chat history
	a.chatHistory = append(a.chatHistory, query)
	
	// Show loading message
	currentChat := a.formatChatHistory()
	currentChat += fmt.Sprintf("\n[blue]You:[white] %s\n\n🤖 GPT-4o is analyzing your logs...", query)
	a.searchResults.SetText(currentChat)
	a.searchResults.ScrollToEnd()
	
	// Get logs from all containers
	contexts := a.contextManager.GetAllContexts()
	if len(contexts) == 0 {
		a.searchResults.SetText("No containers available for AI chat")
		return
	}
	
	logs := make(map[string][]docker.LogEntry)
	for _, context := range contexts {
		logBuffer := context.GetLogBuffer()
		if len(logBuffer) > 0 {
			logs[context.Container.Name] = logBuffer
		}
	}
	
	// Perform AI chat in background to avoid blocking UI
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		
		response, err := a.aiService.ChatWithLogs(ctx, query, logs, a.chatHistory[:len(a.chatHistory)-1]) // Exclude the current query
		
		// Update UI in main thread
		a.app.QueueUpdateDraw(func() {
			if err != nil {
				a.chatHistory = append(a.chatHistory, fmt.Sprintf("Error: %v", err))
			} else {
				a.chatHistory = append(a.chatHistory, response.Analysis)
			}
			
			// Update chat display
			chatDisplay := a.formatChatHistory()
			a.searchResults.SetText(chatDisplay)
			a.searchResults.ScrollToEnd()
		})
	}()
}

// formatChatHistory formats the chat history for display
func (a *App) formatChatHistory() string {
	if len(a.chatHistory) == 0 {
		return "🤖 AI Chat with your logs\nAsk questions like:\n- \"Why is my app slow?\"\n- \"What errors occurred in the last few minutes?\"\n- \"Are there any security issues?\"\n\nType your question and press Enter..."
	}
	
	var output strings.Builder
	output.WriteString("🤖 AI Chat Session\n\n")
	
	for i, msg := range a.chatHistory {
		if i%2 == 0 { // User messages
			output.WriteString(fmt.Sprintf("[blue]You:[white] %s\n\n", msg))
		} else { // AI responses
			output.WriteString(fmt.Sprintf("[green]🤖 GPT-4o:[white] %s\n\n", msg))
		}
	}
	
	return output.String()
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