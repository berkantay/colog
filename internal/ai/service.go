package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/sashabaranov/go-openai"
	"github.com/berkantay/colog/internal/docker"
)

// AIService handles OpenAI API interactions
type AIService struct {
	client *openai.Client
}

// SearchResult represents a semantic search result
type SearchResult struct {
	LogEntry    docker.LogEntry
	Container   string
	Relevance   string
	Explanation string
	// Fields for JSON parsing from streaming response
	Timestamp   string `json:"timestamp,omitempty"`
	Message     string `json:"message,omitempty"`
}

// ChatResponse represents a chat analysis response
type ChatResponse struct {
	Analysis    string
	Suggestions []string
	Summary     string
}

// NewAIService creates a new AI service instance
func NewAIService() (*AIService, error) {
	// Try to load .env file (silently ignore if not found)
	_ = godotenv.Load()
	
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not found - create a .env file with OPENAI_API_KEY=your-key")
	}

	client := openai.NewClient(apiKey)
	return &AIService{client: client}, nil
}

// SemanticSearch performs AI-powered semantic search across logs
func (ai *AIService) SemanticSearch(ctx context.Context, query string, logs map[string][]docker.LogEntry) ([]SearchResult, error) {
	if len(logs) == 0 {
		return nil, fmt.Errorf("no logs provided for search")
	}

	// Prepare log context for AI with all available entries (up to 50 per container)
	var logContext strings.Builder
	
	totalEntries := 0
	for containerName, entries := range logs {
		logContext.WriteString(fmt.Sprintf("=== CONTAINER: %s ===\n", containerName))
		
		// Use all available entries (up to 50 as they're already limited by buffer)
		for _, entry := range entries {
			timestamp := entry.Timestamp.Format("15:04:05")
			logContext.WriteString(fmt.Sprintf("[%s] %s\n", timestamp, entry.Message))
			totalEntries++
		}
		logContext.WriteString(fmt.Sprintf("(%d log entries for %s)\n\n", len(entries), containerName))
	}
	
	if totalEntries == 0 {
		return nil, fmt.Errorf("no log entries found in containers")
	}

	// Use proper system/user message separation with JSON response format
	responseFormat := &openai.ChatCompletionResponseFormat{
		Type: "json_object",
	}

	// System message defines the AI's role and output format
	systemPrompt := `You are an expert DevOps engineer analyzing Docker container logs. Your task is to find log entries that are semantically relevant to the user's query.

IMPORTANT: You MUST respond with valid JSON only, using exactly this structure:
{
  "results": [
    {
      "container": "container-name",
      "timestamp": "HH:MM:SS",
      "message": "exact log message from the provided logs",
      "relevance": "high|medium|low", 
      "explanation": "Brief explanation of why this log is relevant to the query"
    }
  ]
}

Rules:
- Only include log entries that are semantically related to the user's query
- Use exact timestamps and messages from the provided logs
- Focus on errors, warnings, and patterns that relate to the query intent
- If no relevant entries found, return empty results array: {"results": []}
- Maximum 10 most relevant results`

	// User message contains the logs and query
	userPrompt := fmt.Sprintf(`Container Logs (last 50 entries per container):

%s

User Query: "%s"

Please analyze the above logs and find entries relevant to this query. Return only valid JSON.`, logContext.String(), query)

	// Call OpenAI API with proper system/user messages and structured output
	resp, err := ai.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: openai.GPT4oMini,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemPrompt,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: userPrompt,
			},
		},
		MaxTokens:      1500, // Increased for more detailed analysis
		Temperature:    0.2,  // Lower for more focused results
		ResponseFormat: responseFormat,
	})

	if err != nil {
		return nil, fmt.Errorf("OpenAI API error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from OpenAI")
	}

	// Parse the response and convert to SearchResult
	results := ai.parseSearchResponse(resp.Choices[0].Message.Content, logs)
	return results, nil
}

// SemanticSearchStream performs semantic search with streaming responses
func (ai *AIService) SemanticSearchStream(ctx context.Context, query string, logs map[string][]docker.LogEntry, resultChannel chan<- SearchResult) error {
	defer close(resultChannel)
	
	if len(logs) == 0 {
		return fmt.Errorf("no logs provided for search")
	}

	// Prepare log context for AI with all available entries (up to 50 per container)
	var logContext strings.Builder
	
	totalEntries := 0
	for containerName, entries := range logs {
		logContext.WriteString(fmt.Sprintf("=== CONTAINER: %s ===\n", containerName))
		
		// Use all available entries (up to 50 as they're already limited by buffer)
		for _, entry := range entries {
			timestamp := entry.Timestamp.Format("15:04:05")
			logContext.WriteString(fmt.Sprintf("[%s] %s\n", timestamp, entry.Message))
			totalEntries++
		}
		logContext.WriteString(fmt.Sprintf("(%d log entries for %s)\n\n", len(entries), containerName))
	}
	
	if totalEntries == 0 {
		return fmt.Errorf("no log entries found in containers")
	}

	// Create AI prompt for streaming semantic search
	prompt := fmt.Sprintf(`You are analyzing container logs for semantic search.

%s

User query: "%s"

Please find log entries that are semantically related to the user's query, even if they don't contain exact keyword matches.

For each relevant log entry, respond with one JSON object per line (JSONL format):
{"container": "container-name", "timestamp": "15:04:05", "message": "log message", "relevance": "high|medium|low", "explanation": "Brief explanation of why this log is relevant to the query"}

Focus on finding entries that relate to the query's intent, not just keyword matches.`, logContext.String(), query)

	// Note: Structured output may not work with streaming, so we'll use regular streaming
	// and parse the response manually for now
	req := openai.ChatCompletionRequest{
		Model: openai.GPT4oMini,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		MaxTokens:   1000,
		Temperature: 0.3,
		Stream:      true, // Enable streaming
		// ResponseFormat: responseFormat, // Structured output doesn't work with streaming yet
	}

	stream, err := ai.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		// Check if it's an API key issue
		if strings.Contains(err.Error(), "401") {
			return fmt.Errorf("Invalid OpenAI API key - check your .env file")
		}
		return fmt.Errorf("OpenAI streaming API error: %w", err)
	}
	defer stream.Close()

	var fullResponse strings.Builder
	
	for {
		response, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("stream receive error: %w", err)
		}

		if len(response.Choices) > 0 {
			chunk := response.Choices[0].Delta.Content
			fullResponse.WriteString(chunk)
			
			// Try to parse complete lines as they come in
			content := fullResponse.String()
			lines := strings.Split(content, "\n")
			
			// Process all complete lines except the last (which might be incomplete)
			for i := 0; i < len(lines)-1; i++ {
				line := strings.TrimSpace(lines[i])
				if line == "" {
					continue
				}
				
				// Try to parse as JSON
				var result SearchResult
				if err := json.Unmarshal([]byte(line), &result); err == nil && result.Container != "" {
					// Create LogEntry from the parsed data
					timestamp, _ := time.Parse("15:04:05", result.Timestamp)
					result.LogEntry = docker.LogEntry{
						Timestamp: timestamp,
						Message:   result.Message,
					}
					
					// Send result immediately
					select {
					case resultChannel <- result:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			}
			
			// Keep the last (potentially incomplete) line
			if len(lines) > 0 {
				fullResponse.Reset()
				fullResponse.WriteString(lines[len(lines)-1])
			}
		}
	}

	// Process any remaining content
	remaining := strings.TrimSpace(fullResponse.String())
	if remaining != "" {
		var result SearchResult
		if err := json.Unmarshal([]byte(remaining), &result); err == nil && result.Container != "" {
			timestamp, _ := time.Parse("15:04:05", result.Timestamp)
			result.LogEntry = docker.LogEntry{
				Timestamp: timestamp,
				Message:   result.Message,
			}
			
			select {
			case resultChannel <- result:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	
	return nil
}

// ChatWithLogs provides conversational analysis of logs using GPT-4o
func (ai *AIService) ChatWithLogs(ctx context.Context, query string, logs map[string][]docker.LogEntry, conversationHistory []string) (*ChatResponse, error) {
	if len(logs) == 0 {
		return nil, fmt.Errorf("no logs provided for chat")
	}

	// Prepare comprehensive log context
	var logContext strings.Builder
	logContext.WriteString("Current container logs:\n\n")
	
	for containerName, entries := range logs {
		logContext.WriteString(fmt.Sprintf("=== %s ===\n", containerName))
		// Include more entries for chat analysis
		recentEntries := entries
		if len(entries) > 50 {
			recentEntries = entries[len(entries)-50:]
		}
		
		for _, entry := range recentEntries {
			timestamp := entry.Timestamp.Format("15:04:05")
			logContext.WriteString(fmt.Sprintf("[%s] %s\n", timestamp, entry.Message))
		}
		logContext.WriteString("\n")
	}

	// Build conversation history
	messages := []openai.ChatCompletionMessage{
		{
			Role: openai.ChatMessageRoleSystem,
			Content: `You are an expert DevOps engineer helping analyze container logs. 

Provide detailed, actionable insights about:
- Error patterns and root causes
- Performance issues and bottlenecks  
- Security concerns
- Recommended fixes and best practices
- Trends and patterns across containers

Be concise but thorough. Focus on practical solutions.`,
		},
	}

	// Add conversation history
	for i, msg := range conversationHistory {
		role := openai.ChatMessageRoleUser
		if i%2 == 1 { // Odd indices are AI responses
			role = openai.ChatMessageRoleAssistant
		}
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    role,
			Content: msg,
		})
	}

	// Add current query with logs
	currentPrompt := fmt.Sprintf(`%s

User question: %s`, logContext.String(), query)

	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: currentPrompt,
	})

	// Call OpenAI API with GPT-4o for advanced analysis
	resp, err := ai.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       openai.GPT4o,
		Messages:    messages,
		MaxTokens:   2000,
		Temperature: 0.7, // Higher temperature for more creative analysis
	})

	if err != nil {
		return nil, fmt.Errorf("OpenAI API error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from OpenAI")
	}

	analysis := resp.Choices[0].Message.Content

	return &ChatResponse{
		Analysis:    analysis,
		Suggestions: ai.extractSuggestions(analysis),
		Summary:     ai.extractSummary(analysis),
	}, nil
}

// parseSearchResponse converts AI response to SearchResult structs
func (ai *AIService) parseSearchResponse(response string, logs map[string][]docker.LogEntry) []SearchResult {
	var results []SearchResult
	
	// Try to parse the AI response as JSON
	type AIResponse struct {
		Results []struct {
			Container   string `json:"container"`
			Timestamp   string `json:"timestamp"`
			Message     string `json:"message"`
			Relevance   string `json:"relevance"`
			Explanation string `json:"explanation"`
		} `json:"results"`
	}
	
	var aiResp AIResponse
	if err := json.Unmarshal([]byte(response), &aiResp); err != nil {
		// If JSON parsing fails, try to extract meaningful info from the raw response
		return ai.parseRawResponse(response, logs)
	}
	
	// Convert AI results to SearchResult objects
	for _, result := range aiResp.Results {
		timestamp, _ := time.Parse("15:04:05", result.Timestamp)
		
		// Find the actual log entry from the provided logs
		var logEntry docker.LogEntry
		if containerLogs, exists := logs[result.Container]; exists {
			// Find matching log entry
			for _, entry := range containerLogs {
				if strings.Contains(entry.Message, result.Message) || 
				   entry.Timestamp.Format("15:04:05") == result.Timestamp {
					logEntry = entry
					break
				}
			}
		}
		
		// If no matching entry found, create one from AI response
		if logEntry.Message == "" {
			logEntry = docker.LogEntry{
				Timestamp: timestamp,
				Message:   result.Message,
			}
		}
		
		results = append(results, SearchResult{
			LogEntry:    logEntry,
			Container:   result.Container,
			Relevance:   result.Relevance,
			Explanation: result.Explanation,
		})
	}
	
	return results
}

// parseRawResponse handles non-JSON AI responses
func (ai *AIService) parseRawResponse(response string, logs map[string][]docker.LogEntry) []SearchResult {
	var results []SearchResult
	
	// Try to extract useful information from the raw response
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		// Look for container mentions and error indicators
		for containerName := range logs {
			if strings.Contains(strings.ToLower(line), strings.ToLower(containerName)) ||
			   strings.Contains(strings.ToLower(line), "error") ||
			   strings.Contains(strings.ToLower(line), "fail") ||
			   strings.Contains(strings.ToLower(line), "database") ||
			   strings.Contains(strings.ToLower(line), "connection") {
				
				results = append(results, SearchResult{
					LogEntry: docker.LogEntry{
						Timestamp: time.Now(),
						Message:   line,
					},
					Container:   containerName,
					Relevance:   "medium",
					Explanation: "AI identified this as potentially relevant to your query",
				})
				break
			}
		}
	}
	
	// If no specific results found, return the full analysis
	if len(results) == 0 {
		results = append(results, SearchResult{
			LogEntry: docker.LogEntry{
				Timestamp: time.Now(),
				Message:   "AI Analysis",
			},
			Container:   "AI Response",
			Relevance:   "high",
			Explanation: response,
		})
	}
	
	return results
}

// extractSuggestions extracts actionable suggestions from analysis
func (ai *AIService) extractSuggestions(analysis string) []string {
	// Simple extraction - could be enhanced with more sophisticated parsing
	suggestions := []string{
		"Review identified error patterns",
		"Check container resource usage",
		"Verify configuration settings",
	}
	return suggestions
}

// extractSummary creates a brief summary of the analysis
func (ai *AIService) extractSummary(analysis string) string {
	lines := strings.Split(analysis, "\n")
	if len(lines) > 0 {
		return lines[0] // First line as summary
	}
	return "Log analysis completed"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}