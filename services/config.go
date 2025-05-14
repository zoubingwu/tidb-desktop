package services

import (
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
	DefaultBaseTheme       = "claude"
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

// newDefaultThemeSettings creates ThemeSettings with default values.
func newDefaultThemeSettings() *ThemeSettings {
	return &ThemeSettings{Mode: DefaultThemeMode, BaseTheme: DefaultBaseTheme}
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

// newDefaultWindowSettings creates WindowSettings with default values.
func newDefaultWindowSettings() *WindowSettings {
	return &WindowSettings{
		Width:       DefaultWindowWidth,
		Height:      DefaultWindowHeight,
		X:           DefaultWindowX,
		Y:           DefaultWindowY,
		IsMaximized: false,
	}
}

// AIProviderSettings holds API keys and settings for different AI providers
type AIProviderSettings struct {
	CurrentProvider string              `json:"provider,omitempty"` // 'openai', 'anthropic', 'openrouter'
	OpenAI          *OpenAISettings     `json:"openai,omitempty"`
	Anthropic       *AnthropicSettings  `json:"anthropic,omitempty"`
	OpenRouter      *OpenRouterSettings `json:"openrouter,omitempty"`
}

// newDefaultAIProviderSettings creates AIProviderSettings with default values.
func newDefaultAIProviderSettings() *AIProviderSettings {
	return &AIProviderSettings{
		CurrentProvider: DefaultAIProvider,
		OpenAI:          &OpenAISettings{Model: DefaultOpenAIModel},
		Anthropic:       &AnthropicSettings{Model: DefaultAnthropicModel},
		OpenRouter:      &OpenRouterSettings{Model: DefaultOpenRouterModel},
	}
}

// ConfigData defines the structure of the entire configuration file.
type ConfigData struct {
	Connections        map[string]ConnectionDetails `json:"connections"`
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
		config: &ConfigData{ // Initialize with defaults
			Connections:        make(map[string]ConnectionDetails),
			ThemeSettings:      newDefaultThemeSettings(),
			AIProviderSettings: newDefaultAIProviderSettings(),
			WindowSettings:     newDefaultWindowSettings(),
		},
	}

	// Attempt to load existing config, potentially overwriting defaults
	if err := service.loadConfig(); err != nil {
		Info("Warning: Failed to load config file %s: %v. Starting with default config.", configFilePath, err)
		// Defaults are already set, ensure map is initialized if somehow config became nil (unlikely here)
		if service.config == nil { // Defensive check
			service.config = &ConfigData{
				Connections:        make(map[string]ConnectionDetails),
				ThemeSettings:      newDefaultThemeSettings(),
				AIProviderSettings: newDefaultAIProviderSettings(),
				WindowSettings:     newDefaultWindowSettings(),
			}
		} else { // Ensure sub-fields are at least their defaults if loadConfig partially failed or cleared them
			if service.config.Connections == nil {
				service.config.Connections = make(map[string]ConnectionDetails)
			}
			if service.config.ThemeSettings == nil {
				service.config.ThemeSettings = newDefaultThemeSettings()
			}
			if service.config.AIProviderSettings == nil {
				service.config.AIProviderSettings = newDefaultAIProviderSettings()
			}
			// Further ensure nested AI settings if AIProviderSettings was not nil but its fields were
			ensureAIProviderSubSettings(service.config.AIProviderSettings)

			if service.config.WindowSettings == nil {
				service.config.WindowSettings = newDefaultWindowSettings()
			}
		}
	} else {
		Info("Config loaded successfully from %s", configFilePath)
		// Ensure all parts of the config are present and have defaults if missing from the loaded file.
		// loadConfig itself handles most of this, but we can double-check top-level structs here.
		if service.config.ThemeSettings == nil {
			service.config.ThemeSettings = newDefaultThemeSettings()
			// Optionally save immediately, though loadConfig should handle this.
		}
		if service.config.AIProviderSettings == nil {
			service.config.AIProviderSettings = newDefaultAIProviderSettings()
		}
		// ensureAIProviderSubSettings is called within loadConfig, so it should be fine.
		if service.config.WindowSettings == nil {
			service.config.WindowSettings = newDefaultWindowSettings()
		}
	}

	return service, nil
}

// ensureAIProviderSubSettings ensures that the AIProviderSettings and its nested structs are initialized.
// It also ensures that model fields have default values if they are empty.
func ensureAIProviderSubSettings(settings *AIProviderSettings) {
	if settings == nil {
		return // Should be handled by caller by assigning newDefaultAIProviderSettings
	}
	if settings.CurrentProvider == "" {
		settings.CurrentProvider = DefaultAIProvider
	}
	if settings.OpenAI == nil {
		settings.OpenAI = &OpenAISettings{Model: DefaultOpenAIModel}
	} else if settings.OpenAI.Model == "" {
		settings.OpenAI.Model = DefaultOpenAIModel
	}
	if settings.Anthropic == nil {
		settings.Anthropic = &AnthropicSettings{Model: DefaultAnthropicModel}
	} else if settings.Anthropic.Model == "" {
		settings.Anthropic.Model = DefaultAnthropicModel
	}
	if settings.OpenRouter == nil {
		settings.OpenRouter = &OpenRouterSettings{Model: DefaultOpenRouterModel}
	} else if settings.OpenRouter.Model == "" {
		settings.OpenRouter.Model = DefaultOpenRouterModel
	}
}

// loadConfig reads the config file from disk.
func (s *ConfigService) loadConfig() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		Info("Config file %s does not exist. Using defaults set in NewConfigService.", s.configPath)
		// Defaults are already set in NewConfigService, no need to re-initialize here.
		// s.config should already have default values.
		return nil
	}

	data, err := os.ReadFile(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", s.configPath, err)
	}

	if len(data) == 0 {
		Info("Config file %s is empty. Using defaults set in NewConfigService.", s.configPath)
		// Defaults are already set in NewConfigService.
		return nil
	}

	var loadedConfig ConfigData
	if err := json.Unmarshal(data, &loadedConfig); err != nil {
		return fmt.Errorf("failed to unmarshal config data from %s: %w", s.configPath, err)
	}

	// Assign loaded data to the service config, potentially overwriting initial defaults
	s.config = &loadedConfig

	// Ensure primary nested structures are initialized if they were null/missing in the JSON
	if s.config.Connections == nil {
		s.config.Connections = make(map[string]ConnectionDetails)
	}
	if s.config.ThemeSettings == nil {
		s.config.ThemeSettings = newDefaultThemeSettings()
	}
	if s.config.AIProviderSettings == nil {
		s.config.AIProviderSettings = newDefaultAIProviderSettings()
	} else {
		// If AIProviderSettings was loaded, ensure its sub-settings and model defaults
		ensureAIProviderSubSettings(s.config.AIProviderSettings)
	}
	if s.config.WindowSettings == nil {
		s.config.WindowSettings = newDefaultWindowSettings()
	}

	return nil
}

// saveConfig is an internal helper that writes the current config data to disk.
// It assumes the caller holds the necessary lock.
func (s *ConfigService) saveConfig() error {
	configDir := filepath.Dir(s.configPath)
	if err := os.MkdirAll(configDir, 0750); err != nil {
		return fmt.Errorf("failed to create config directory %s: %w", configDir, err)
	}

	data, err := json.MarshalIndent(s.config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config data: %w", err)
	}

	if err := os.WriteFile(s.configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", s.configPath, err)
	}
	Info("Config saved successfully to %s", s.configPath)
	return nil
}

// --- Connection Management Methods ---

// GetAllConnections returns a copy of all stored connections.
func (s *ConfigService) GetAllConnections() (map[string]ConnectionDetails, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	connectionsCopy := make(map[string]ConnectionDetails)
	if s.config != nil && s.config.Connections != nil {
		for name, details := range s.config.Connections {
			details.Name = name
			connectionsCopy[name] = details
		}
	}
	return connectionsCopy, nil
}

// AddOrUpdateConnection adds a new connection or updates an existing one by name.
func (s *ConfigService) AddOrUpdateConnection(name string, details ConnectionDetails) error {
	if name == "" {
		return fmt.Errorf("connection name cannot be empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.config == nil { // Should not happen with NewConfigService initialization
		s.config = &ConfigData{}
	}
	if s.config.Connections == nil {
		s.config.Connections = make(map[string]ConnectionDetails)
	}
	s.config.Connections[name] = details
	return s.saveConfig()
}

// DeleteConnection removes a connection by name.
func (s *ConfigService) DeleteConnection(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.config == nil || s.config.Connections == nil {
		return fmt.Errorf("connection '%s' not found (config or connections map is nil)", name)
	}
	if _, exists := s.config.Connections[name]; !exists {
		return fmt.Errorf("connection '%s' not found", name)
	}

	delete(s.config.Connections, name)
	return s.saveConfig()
}

// GetConnection retrieves a specific connection by name.
func (s *ConfigService) GetConnection(name string) (ConnectionDetails, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.config == nil || s.config.Connections == nil {
		return ConnectionDetails{}, false, nil
	}
	details, found := s.config.Connections[name]
	if found {
		details.Name = name
	}
	return details, found, nil
}

// RecordConnectionUsage updates the LastUsed timestamp for a connection.
func (s *ConfigService) RecordConnectionUsage(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.config == nil || s.config.Connections == nil {
		Info("Warning: Attempted to record usage for connection '%s' but config or connections map is nil", name)
		return nil
	}
	details, found := s.config.Connections[name]
	if !found {
		Info("Warning: Attempted to record usage for non-existent connection '%s'", name)
		return nil
	}

	details.LastUsed = time.Now().Format(time.RFC3339)
	s.config.Connections[name] = details
	return s.saveConfig()
}

// --- Theme Settings Management Methods ---

// GetThemeSettings retrieves the current theme settings.
func (s *ConfigService) GetThemeSettings() (*ThemeSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.config == nil || s.config.ThemeSettings == nil {
		Info("ThemeSettings were nil in config, returning defaults.")
		return newDefaultThemeSettings(), nil
	}
	settingsCopy := *s.config.ThemeSettings // Return a copy
	return &settingsCopy, nil
}

// SaveThemeSettings updates and saves the theme settings.
func (s *ConfigService) SaveThemeSettings(settings ThemeSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if settings.Mode == "" {
		settings.Mode = DefaultThemeMode
	}
	if settings.BaseTheme == "" {
		settings.BaseTheme = DefaultBaseTheme
	}
	if s.config == nil { // Should not happen
		s.config = &ConfigData{}
	}
	s.config.ThemeSettings = &settings
	return s.saveConfig()
}

// --- AI Provider Settings Management Methods ---

// GetAIProviderSettings retrieves the current AI provider settings.
func (s *ConfigService) GetAIProviderSettings() (*AIProviderSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.config == nil || s.config.AIProviderSettings == nil {
		Info("AIProviderSettings were nil in config, returning defaults.")
		return newDefaultAIProviderSettings(), nil
	}
	// Return a copy and ensure sub-settings are valid
	settingsCopy := *s.config.AIProviderSettings
	ensureAIProviderSubSettings(&settingsCopy) // Ensure copy has valid sub-settings and model defaults
	return &settingsCopy, nil
}

// SaveAIProviderSettings updates and saves the AI provider settings.
func (s *ConfigService) SaveAIProviderSettings(settings AIProviderSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	isValidProvider := false
	validProviders := []string{"openai", "anthropic", "openrouter"}
	for _, p := range validProviders {
		if settings.CurrentProvider == p {
			isValidProvider = true
			break
		}
	}
	if !isValidProvider {
		Info("Warning: Invalid CurrentProvider '%s'. Defaulting to '%s'.", settings.CurrentProvider, DefaultAIProvider)
		settings.CurrentProvider = DefaultAIProvider
	}

	// Ensure nested structs and their model fields have defaults before saving
	ensureAIProviderSubSettings(&settings)

	if s.config == nil { // Should not happen
		s.config = &ConfigData{}
	}
	s.config.AIProviderSettings = &settings
	return s.saveConfig()
}

// --- Window Settings Management Methods ---

// GetWindowSettings retrieves the current window settings.
func (s *ConfigService) GetWindowSettings() (*WindowSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.config == nil || s.config.WindowSettings == nil {
		Info("WindowSettings were nil in config, returning defaults.")
		return newDefaultWindowSettings(), nil
	}
	settingsCopy := *s.config.WindowSettings // Return a copy
	return &settingsCopy, nil
}

// SaveWindowSettings updates and saves the window settings.
func (s *ConfigService) SaveWindowSettings(settings WindowSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if settings.Width <= 0 {
		settings.Width = DefaultWindowWidth
	}
	if settings.Height <= 0 {
		settings.Height = DefaultWindowHeight
	}
	// X and Y can be negative; -1 is our convention for center.

	if s.config == nil { // Should not happen
		s.config = &ConfigData{}
	}
	s.config.WindowSettings = &settings
	Info("Saving window settings: %+v", settings)
	return s.saveConfig()
}
