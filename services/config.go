package services

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	ConfigDirName          = ".tidb-desktop"
	ConfigFileName         = "config.json"
	MetadataDirName        = "metadata"
	DefaultOpenAIModel     = "gpt-4o"
	DefaultAnthropicModel  = "claude-3-5-sonnet-latest"
	DefaultOpenRouterModel = "anthropic/claude-3.5-sonnet"
	DefaultThemeMode       = "system"
	DefaultBaseTheme       = "solar-dusk"
	DefaultAIProvider      = "openai"
	DefaultWindowWidth     = 1024
	DefaultWindowHeight    = 768
	DefaultWindowX         = -1 // Represents center
	DefaultWindowY         = -1 // Represents center
)

// ThemeSettings holds theme preferences
type ThemeSettings struct {
	Mode      string `json:"mode"`      // e.g., "light", "dark", "system"
	BaseTheme string `json:"baseTheme"` // e.g., "claude", "nature"
}

// OpenAISettings holds settings specific to OpenAI provider
type OpenAISettings struct {
	APIKey  string `json:"apiKey,omitempty"`
	BaseURL string `json:"baseURL,omitempty"` // Default: https://api.openai.com/v1
	Model   string `json:"model,omitempty"`   // e.g., "gpt-4", "gpt-3.5-turbo"
}

// AnthropicSettings holds settings specific to Anthropic provider
type AnthropicSettings struct {
	APIKey  string `json:"apiKey,omitempty"`
	BaseURL string `json:"baseURL,omitempty"` // Default: https://api.anthropic.com/v1
	Model   string `json:"model,omitempty"`   // e.g., "claude-2", "claude-instant-1"
}

// OpenRouterSettings holds settings specific to OpenRouter provider
type OpenRouterSettings struct {
	APIKey string `json:"apiKey,omitempty"`
	Model  string `json:"model,omitempty"` // e.g., "openrouter/auto"
}

// WindowSettings holds window geometry preferences
type WindowSettings struct {
	Width       int  `json:"width,omitempty"`
	Height      int  `json:"height,omitempty"`
	X           int  `json:"x,omitempty"`
	Y           int  `json:"y,omitempty"`
	IsMaximized bool `json:"isMaximized,omitempty"`
}

// AIProviderSettings holds API keys and settings for different AI providers
type AIProviderSettings struct {
	CurrentProvider string              `json:"provider,omitempty"` // 'openai', 'anthropic', 'openrouter'
	OpenAI          *OpenAISettings     `json:"openai,omitempty"`
	Anthropic       *AnthropicSettings  `json:"anthropic,omitempty"`
	OpenRouter      *OpenRouterSettings `json:"openrouter,omitempty"`
}

// generateConnectionID creates a random 8-character hex string for connection ID
func generateConnectionID() string {
	bytes := make([]byte, 4) // 4 bytes = 8 hex characters
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// ConfigData defines the structure of the entire configuration file.
type ConfigData struct {
	Connections        map[string]ConnectionDetails `json:"connections"` // key is connection ID
	ThemeSettings      *ThemeSettings               `json:"appearance,omitempty"`
	AIProviderSettings *AIProviderSettings          `json:"ai,omitempty"`
	WindowSettings     *WindowSettings              `json:"window,omitempty"`
}

// ConfigService handles loading and saving application configuration.
type ConfigService struct {
	configPath string
	config     *ConfigData
	mu         sync.RWMutex
}

// NewConfigService creates a new service and loads the initial config.
func NewConfigService() (*ConfigService, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	configDirPath := filepath.Join(homeDir, ConfigDirName)
	configFilePath := filepath.Join(configDirPath, ConfigFileName)

	service := &ConfigService{
		configPath: configFilePath,
		config: &ConfigData{
			Connections:   make(map[string]ConnectionDetails),
			ThemeSettings: &ThemeSettings{Mode: DefaultThemeMode, BaseTheme: DefaultBaseTheme},
			AIProviderSettings: &AIProviderSettings{
				CurrentProvider: DefaultAIProvider,
				OpenAI:          &OpenAISettings{Model: DefaultOpenAIModel},
				Anthropic:       &AnthropicSettings{Model: DefaultAnthropicModel},
				OpenRouter:      &OpenRouterSettings{Model: DefaultOpenRouterModel},
			},
			WindowSettings: &WindowSettings{
				Width:       DefaultWindowWidth,
				Height:      DefaultWindowHeight,
				X:           DefaultWindowX,
				Y:           DefaultWindowY,
				IsMaximized: false,
			},
		},
	}

	// Try to load existing config
	if err := service.loadConfig(); err != nil {
		LogInfo("Warning: Failed to load config file: %v. Using defaults.", err)
	}

	return service, nil
}

// loadConfig reads the config file from disk.
func (s *ConfigService) loadConfig() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist, use defaults
		}
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if len(data) == 0 {
		return nil // Empty file, use defaults
	}

	var loadedConfig ConfigData
	if err := json.Unmarshal(data, &loadedConfig); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Merge loaded config with defaults
	if loadedConfig.Connections != nil {
		s.config.Connections = loadedConfig.Connections
	}
	if loadedConfig.ThemeSettings != nil {
		s.config.ThemeSettings = loadedConfig.ThemeSettings
	}
	if loadedConfig.AIProviderSettings != nil {
		s.config.AIProviderSettings = loadedConfig.AIProviderSettings
	}
	if loadedConfig.WindowSettings != nil {
		s.config.WindowSettings = loadedConfig.WindowSettings
	}

	return nil
}

// saveConfig writes the current config data to disk.
func (s *ConfigService) saveConfig() error {
	configDir := filepath.Dir(s.configPath)
	if err := os.MkdirAll(configDir, 0750); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(s.config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(s.configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// --- Connection Management Methods ---

// GetAllConnections returns a copy of all stored connections.
func (s *ConfigService) GetAllConnections() (map[string]ConnectionDetails, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	connectionsCopy := make(map[string]ConnectionDetails)
	for id, details := range s.config.Connections {
		details.ID = id
		connectionsCopy[id] = details
	}
	return connectionsCopy, nil
}

// AddOrUpdateConnection adds a new connection or updates an existing one.
// Returns the connection ID.
func (s *ConfigService) AddOrUpdateConnection(details ConnectionDetails) (string, error) {
	if details.Name == "" {
		return "", fmt.Errorf("connection name cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Generate ID if not provided (new connection)
	if details.ID == "" {
		// Check for name conflicts
		for _, existing := range s.config.Connections {
			if existing.Name == details.Name {
				return "", fmt.Errorf("connection name '%s' already exists", details.Name)
			}
		}
		details.ID = generateConnectionID()
	} else {
		// Updating existing connection - check for name conflicts
		for id, existing := range s.config.Connections {
			if id != details.ID && existing.Name == details.Name {
				return "", fmt.Errorf("connection name '%s' already exists", details.Name)
			}
		}
	}

	s.config.Connections[details.ID] = details
	err := s.saveConfig()
	return details.ID, err
}

// DeleteConnection removes a connection by ID.
func (s *ConfigService) DeleteConnection(connectionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.config.Connections[connectionID]; !exists {
		return fmt.Errorf("connection '%s' not found", connectionID)
	}

	delete(s.config.Connections, connectionID)
	return s.saveConfig()
}

// GetConnection retrieves a specific connection by ID.
func (s *ConfigService) GetConnection(connectionID string) (ConnectionDetails, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	details, found := s.config.Connections[connectionID]
	if found {
		details.ID = connectionID
	}
	return details, found, nil
}

// RecordConnectionUsage updates the LastUsed timestamp for a connection by ID.
func (s *ConfigService) RecordConnectionUsage(connectionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	details, found := s.config.Connections[connectionID]
	if !found {
		return nil // Connection not found, ignore
	}

	details.LastUsed = time.Now().Format(time.RFC3339)
	s.config.Connections[connectionID] = details
	return s.saveConfig()
}

// --- Theme Settings Management Methods ---

// GetThemeSettings retrieves the current theme settings.
func (s *ConfigService) GetThemeSettings() (*ThemeSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.config.ThemeSettings, nil
}

// SaveThemeSettings updates and saves the theme settings.
func (s *ConfigService) SaveThemeSettings(settings ThemeSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config.ThemeSettings = &settings
	return s.saveConfig()
}

// --- AI Provider Settings Management Methods ---

// GetAIProviderSettings retrieves the current AI provider settings.
func (s *ConfigService) GetAIProviderSettings() (*AIProviderSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.config.AIProviderSettings, nil
}

// SaveAIProviderSettings updates and saves the AI provider settings.
func (s *ConfigService) SaveAIProviderSettings(settings AIProviderSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config.AIProviderSettings = &settings
	return s.saveConfig()
}

// --- Window Settings Management Methods ---

// GetWindowSettings retrieves the current window settings.
func (s *ConfigService) GetWindowSettings() (*WindowSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.config.WindowSettings, nil
}

// SaveWindowSettings updates and saves the window settings.
func (s *ConfigService) SaveWindowSettings(settings WindowSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config.WindowSettings = &settings
	return s.saveConfig()
}
