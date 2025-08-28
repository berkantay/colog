package main

import (
	"context"
	"fmt"
	"math"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type App struct {
	app           *tview.Application
	grid          *tview.Grid
	dockerService *DockerService
	logViews      map[string]*tview.TextView
	containers    []Container
	colors        []tcell.Color
	colorIndex    int
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
}

func NewApp() *App {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &App{
		app:        tview.NewApplication(),
		grid:       tview.NewGrid(),
		logViews:   make(map[string]*tview.TextView),
		colors:     getContainerColors(),
		colorIndex: 0,
		ctx:        ctx,
		cancel:     cancel,
	}
}

func (a *App) Run() error {
	var err error
	a.dockerService, err = NewDockerService()
	if err != nil {
		return fmt.Errorf("failed to connect to Docker: %w", err)
	}
	defer a.dockerService.Close()

	if err := a.setupUI(); err != nil {
		return err
	}

	if err := a.loadContainers(); err != nil {
		return err
	}

	if len(a.containers) == 0 {
		return fmt.Errorf("no running containers found")
	}

	a.setupGrid()
	a.startLogStreaming()
	a.setupKeyBindings()

	return a.app.SetRoot(a.grid, true).Run()
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
			fmt.Fprint(logView, message)
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
			}
		}
		return event
	})
}

func getContainerColors() []tcell.Color {
	return []tcell.Color{
		tcell.ColorBlue,
		tcell.ColorGreen,
		tcell.ColorYellow,
		tcell.ColorRed,
		tcell.ColorPurple,
		tcell.ColorOrange,
		tcell.ColorLime,
		tcell.ColorTeal,
		tcell.ColorNavy,
		tcell.ColorMaroon,
		tcell.ColorOlive,
		tcell.ColorSilver,
		tcell.ColorWhite,
		tcell.ColorGray,
	}
}

func colorToTviewColor(color tcell.Color) string {
	colorMap := map[tcell.Color]string{
		tcell.ColorBlue:   "blue",
		tcell.ColorGreen:  "green",
		tcell.ColorYellow: "yellow",
		tcell.ColorRed:    "red",
		tcell.ColorPurple: "purple",
		tcell.ColorOrange: "orange",
		tcell.ColorLime:   "lime",
		tcell.ColorTeal:   "teal",
		tcell.ColorNavy:   "navy",
		tcell.ColorMaroon: "maroon",
		tcell.ColorOlive:  "olive",
		tcell.ColorSilver: "silver",
		tcell.ColorWhite:  "white",
		tcell.ColorGray:   "gray",
	}
	
	if name, ok := colorMap[color]; ok {
		return name
	}
	return "white"
}