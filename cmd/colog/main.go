package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/berkantay/colog/internal/app"
	"github.com/berkantay/colog/internal/sdk"
	"github.com/berkantay/colog/internal/mcp"
)

func main() {
	// Check for help first
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		printHelp()
		return
	}
	
	// Check if this is an SDK command
	if len(os.Args) > 1 && os.Args[1] == "sdk" {
		if err := sdk.RunSDKCommand(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "SDK Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Check if this is an MCP server command
	if len(os.Args) > 2 && os.Args[1] == "-m" && os.Args[2] == "sse" {
		if err := runMCPServer(); err != nil {
			fmt.Fprintf(os.Stderr, "MCP Server Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Check if this is an MCP stdio command
	if len(os.Args) > 2 && os.Args[1] == "-m" && os.Args[2] == "stdio" {
		if err := mcp.RunMCPStdio(); err != nil {
			fmt.Fprintf(os.Stderr, "MCP Stdio Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fmt.Println("Colog - Docker Container Logs Viewer")
	
	app := app.NewApp()
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runMCPServer() error {
	fmt.Println("Starting Colog MCP Server with SSE support...")
	
	// Get configuration from environment or set defaults
	port := os.Getenv("MCP_PORT")
	if port == "" {
		port = "8080"
	}

	host := os.Getenv("MCP_HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	fmt.Printf("MCP Server will start on %s:%s\n", host, port)
	
	// Build and run the MCP server
	cmd := exec.Command("go", "run", "./mcp/server.go")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"MCP_PORT="+port,
		"MCP_HOST="+host,
	)
	
	// Pass through any existing MCP environment variables
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "MCP_") {
			cmd.Env = append(cmd.Env, env)
		}
	}
	
	return cmd.Run()
}

func printHelp() {
	fmt.Println(`Colog - Live Docker Container Logs Viewer

USAGE:
    colog [COMMAND] [OPTIONS]

COMMANDS:
    (default)      Start the interactive TUI log viewer
    sdk            Use SDK commands for programmatic access
    -m sse         Start MCP server with SSE support
    -m stdio       Start MCP server with stdio transport (for direct integration)

OPTIONS:
    -h, --help     Show this help message

TUI CONTROLS:
    q              Quit the application
    y              Export last 50 log lines from each container for LLM analysis
    j/k            Navigate up/down between containers
    Space          Toggle fullscreen mode for focused container
    /              Search across all container logs (with purple highlighting)
    ?              AI-powered semantic search (requires OPENAI_API_KEY)
    C              Chat with your logs using GPT-4o (requires OPENAI_API_KEY)
    ESC            Exit search/AI mode
    r              Restart focused container
    x              Kill focused container
    Ctrl+C         Quit the application

AI FEATURES:
    Create a .env file with your OpenAI API key to enable AI features:
        echo "OPENAI_API_KEY=your-api-key" > .env
    
    Features:
    - Semantic search: Find logs by meaning, not just keywords
    - Log analysis chat: Ask GPT-4o questions about your logs

SDK USAGE:
    colog sdk --help                           # Show SDK help
    colog sdk list                             # List running containers
    colog sdk logs <container_id> --tail 50    # Get container logs
    colog sdk export --format markdown         # Export logs for LLM

DESCRIPTION:
    Colog displays live logs from all running Docker containers in a clean,
    grid-based terminal interface. Each container gets its own pane with
    color-coded titles and real-time log streaming.

    The SDK mode provides programmatic access to container information and logs,
    perfect for integration with monitoring systems or LLM analysis workflows.`)
}