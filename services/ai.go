package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings" // Added for parsing
	"time"
)

const (
	openRouterAPIURL = "https://openrouter.ai/api/v1/chat/completions"
	// Consider making the model configurable if needed
	defaultModel = "google/gemini-flash-1.5:free" // Use a fast, free model for inference
)

// AIService handles interactions with the LLM API
type AIService struct {
	apiKey     string
	httpClient *http.Client
}

// NewAIService creates a new AI service instance.
// Reads the API key from the OPENROUTER_API_KEY environment variable.
func NewAIService() (*AIService, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY environment variable not set")
	}

	return &AIService{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // Set a reasonable timeout
		},
	}, nil
}

// --- Request Structures ---

type OpenRouterRequest struct {
	Model    string          `json:"model"`
	Messages []RequestMessage `json:"messages"`
	// Add other parameters like temperature, max_tokens if needed
}

type RequestMessage struct {
	Role    string          `json:"role"`
	Content []MessageContent `json:"content"` // Use array for multi-modal potential
}

type MessageContent struct {
	Type string `json:"type"` // "text" or "image_url"
	Text string `json:"text,omitempty"`
	// ImageURL *ImageURLContent `json:"image_url,omitempty"` // Add if needed later
}

// --- Response Structures ---

type OpenRouterResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	// Add Usage field if needed
}

type Choice struct {
	Index        int             `json:"index"`
	Message      ResponseMessage `json:"message"`
	FinishReason string          `json:"finish_reason"`
}

type ResponseMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"` // Response content is usually a single string
}

// --- Service Methods ---

// InferConnectionDetails sends text to the LLM and attempts to parse connection details.
// It returns a *partially populated* ConnectionDetails struct.
func (s *AIService) InferConnectionDetails(ctx context.Context, inputText string) (*ConnectionDetails, error) {
	if strings.TrimSpace(inputText) == "" {
		return nil, fmt.Errorf("input text cannot be empty")
	}

	// Construct a specific prompt asking for JSON output
	prompt := fmt.Sprintf(`
Analyze the following text and extract database connection details. Respond ONLY with a JSON object containing the keys "host", "port", "user", "password", "dbName", and "useTLS" (boolean, true if TLS/SSL is mentioned or implied or it is tidbcloud.com, otherwise false). If a value is not found, use an empty string "" for string fields or false for the boolean.

Input Text:
"""
%s
"""

JSON Output:
`, inputText)

	requestPayload := OpenRouterRequest{
		Model: defaultModel,
		Messages: []RequestMessage{
			{
				Role: "user",
				Content: []MessageContent{
					{Type: "text", Text: prompt},
				},
			},
		},
	}

	jsonData, err := json.Marshal(requestPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", openRouterAPIURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	// OpenRouter specific headers (optional but recommended)
	req.Header.Set("HTTP-Referer", "http://localhost") // Replace with your app URL/name
	req.Header.Set("X-Title", "TiDB Desktop")      // Replace with your app name

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to OpenRouter: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var apiResponse OpenRouterResponse
	err = json.Unmarshal(bodyBytes, &apiResponse)
	if err != nil {
		// Attempt to log the raw response if JSON parsing fails
		fmt.Printf("Failed to unmarshal JSON response. Raw response: %s\n", string(bodyBytes))
		return nil, fmt.Errorf("failed to unmarshal API response: %w", err)
	}

	if len(apiResponse.Choices) == 0 || apiResponse.Choices[0].Message.Content == "" {
		// Log the full response for debugging if content is missing
		fmt.Printf("API response missing expected content. Full response: %+v\n", apiResponse)
		return nil, fmt.Errorf("received an empty response from the LLM")
	}

	llmContent := apiResponse.Choices[0].Message.Content

	// --- Attempt to parse the LLM's response as JSON ---
	// Clean the response slightly - sometimes LLMs wrap JSON in backticks or add prefixes
	cleanedContent := strings.TrimSpace(llmContent)
	cleanedContent = strings.TrimPrefix(cleanedContent, "```json")
	cleanedContent = strings.TrimPrefix(cleanedContent, "```")
	cleanedContent = strings.TrimSuffix(cleanedContent, "```")
	cleanedContent = strings.TrimSpace(cleanedContent)

	var inferredDetails ConnectionDetails
	err = json.Unmarshal([]byte(cleanedContent), &inferredDetails)
	if err != nil {
		// If direct JSON parsing fails, log and return an error (or attempt regex as fallback)
		fmt.Printf("Failed to parse LLM content as JSON. Raw content: %s\nError: %v\n", cleanedContent, err)
		// You could add regex parsing here as a fallback if needed
		return nil, fmt.Errorf("LLM response was not valid JSON: %w. Content: %s", err, cleanedContent)
	}

	// Optional: Validate or clean up inferred values (e.g., ensure port is numeric if needed)

	return &inferredDetails, nil
}
