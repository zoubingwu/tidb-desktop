package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync" // Use a mutex for safe concurrent access if needed, though less critical for simple desktop apps
	"time"
)

const (
	ConfigDirName  = ".tidb-desktop"
	ConfigFileName = "config.json"
	MetadataDirName = "metadata"
)

// ThemeSettings holds theme preferences
type ThemeSettings struct {
	Mode      string `json:"mode"` // e.g., "light", "dark", "system"
	BaseTheme string `json:"baseTheme"` // e.g., "claude", "nature"
}

// OpenAISettings holds settings specific to OpenAI provider
type OpenAISettings struct {
	APIKey  string `json:"apiKey,omitempty"`
	BaseURL string `json:"baseURL,omitempty"` // Default: https://api.openai.com/v1
}

// AnthropicSettings holds settings specific to Anthropic provider
type AnthropicSettings struct {
	APIKey  string `json:"apiKey,omitempty"`
	BaseURL string `json:"baseURL,omitempty"` // Default: https://api.anthropic.com/v1
}

// OpenRouterSettings holds settings specific to OpenRouter provider
type OpenRouterSettings struct {
	APIKey string `json:"apiKey,omitempty"`
}

// AIProviderSettings holds API keys and settings for different AI providers
type AIProviderSettings struct {
	CurrentProvider string             `json:"provider,omitempty"` // 'openai', 'anthropic', 'openrouter'
	OpenAI          *OpenAISettings    `json:"openai,omitempty"`
	Anthropic       *AnthropicSettings `json:"anthropic,omitempty"`
	OpenRouter      *OpenRouterSettings `json:"openrouter,omitempty"`
}

// ConfigData defines the structure of the entire configuration file.
type ConfigData struct {
	// Use ConnectionDetails from database.go
	Connections   map[string]ConnectionDetails `json:"connections"`
	// Add other configuration fields here later, e.g., settings
	ThemeSettings    *ThemeSettings               `json:"appearance,omitempty"` // Use pointer to handle nil easily
	AIProviderSettings *AIProviderSettings          `json:"ai,omitempty"` // Settings for AI providers
}

// ConfigService handles loading and saving application configuration.
type ConfigService struct {
	configPath string
	config     *ConfigData
	mu         sync.RWMutex // Protects access to the config data
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
			ThemeSettings: &ThemeSettings{Mode: "system", BaseTheme: "claude"}, // Default theme settings
			AIProviderSettings: &AIProviderSettings{ // Initialize AI settings struct
				CurrentProvider: "openai", // Default provider
				OpenAI:          &OpenAISettings{},
				Anthropic:       &AnthropicSettings{},
				OpenRouter:      &OpenRouterSettings{},
			},
		},
	}

	// Load existing config on startup, potentially overwriting defaults
	if err := service.loadConfig(); err != nil {
		Info("Warning: Failed to load config file %s: %v. Starting with default config.", configFilePath, err)
		// Ensure base structure is still valid even if load fails
		if service.config == nil {
			service.config = &ConfigData{
				Connections:   make(map[string]ConnectionDetails),
				ThemeSettings: &ThemeSettings{Mode: "system", BaseTheme: "claude"},
				AIProviderSettings: &AIProviderSettings{
					CurrentProvider: "openai", // Default provider
					OpenAI:          &OpenAISettings{},
					Anthropic:       &AnthropicSettings{},
					OpenRouter:      &OpenRouterSettings{},
				},
			}
		}
		// Ensure sub-maps/structs are initialized if config was partially loaded or nil
		if service.config.Connections == nil {
			service.config.Connections = make(map[string]ConnectionDetails)
		}
		if service.config.ThemeSettings == nil {
			service.config.ThemeSettings = &ThemeSettings{Mode: "system", BaseTheme: "claude"}
		}
		// Ensure AIProviderSettings and its nested structs are initialized if they are missing
		if service.config.AIProviderSettings == nil {
			service.config.AIProviderSettings = &AIProviderSettings{
				CurrentProvider: "openai", // Default provider
				OpenAI:          &OpenAISettings{},
				Anthropic:       &AnthropicSettings{},
				OpenRouter:      &OpenRouterSettings{},
			}
		} else {
			if service.config.AIProviderSettings.CurrentProvider == "" { // Ensure default if loaded empty
				service.config.AIProviderSettings.CurrentProvider = "openai"
			}
			if service.config.AIProviderSettings.OpenAI == nil {
				service.config.AIProviderSettings.OpenAI = &OpenAISettings{}
			}
			if service.config.AIProviderSettings.Anthropic == nil {
				service.config.AIProviderSettings.Anthropic = &AnthropicSettings{}
			}
			if service.config.AIProviderSettings.OpenRouter == nil {
				service.config.AIProviderSettings.OpenRouter = &OpenRouterSettings{}
			}
		}
	} else {
		Info("Config loaded successfully from %s", configFilePath)
		// Ensure ThemeSettings is initialized if it was missing in the loaded file
		if service.config.ThemeSettings == nil {
			service.config.ThemeSettings = &ThemeSettings{Mode: "system", BaseTheme: "solar-dusk"}
			// Optionally save immediately to persist the default theme settings
			// if err := service.saveConfig(); err != nil {
			//  Info("Warning: Failed to save default theme settings after load: %v", err)
			// }
		}
		// Ensure AIProviderSettings is initialized if it was missing
		if service.config.AIProviderSettings == nil {
			service.config.AIProviderSettings = &AIProviderSettings{
				CurrentProvider: "openai", // Default provider
				OpenAI:          &OpenAISettings{},
				Anthropic:       &AnthropicSettings{},
				OpenRouter:      &OpenRouterSettings{},
			}
		} else {
			if service.config.AIProviderSettings.CurrentProvider == "" { // Ensure default if loaded empty
				service.config.AIProviderSettings.CurrentProvider = "openai"
			}
			if service.config.AIProviderSettings.OpenAI == nil {
				service.config.AIProviderSettings.OpenAI = &OpenAISettings{}
			}
			if service.config.AIProviderSettings.Anthropic == nil {
				service.config.AIProviderSettings.Anthropic = &AnthropicSettings{}
			}
			if service.config.AIProviderSettings.OpenRouter == nil {
				service.config.AIProviderSettings.OpenRouter = &OpenRouterSettings{}
			}
		}
	}

	return service, nil
}

// loadConfig reads the config file from disk.
func (s *ConfigService) loadConfig() error {
	s.mu.Lock() // Acquire write lock as we are modifying s.config
	defer s.mu.Unlock()

	// Check if file exists
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		Info("Config file %s does not exist, creating empty config with defaults.", s.configPath)
		// Defaults are already set in NewConfigService, ensure AI defaults are there too
		if s.config.AIProviderSettings == nil {
			s.config.AIProviderSettings = &AIProviderSettings{
				CurrentProvider: "openai", // Default provider
				OpenAI:          &OpenAISettings{},
				Anthropic:       &AnthropicSettings{},
				OpenRouter:      &OpenRouterSettings{},
			}
		}
		return nil
	}

	data, err := os.ReadFile(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", s.configPath, err)
	}

	// If the file is empty, initialize config
	if len(data) == 0 {
		Info("Config file %s is empty, using default config.", s.configPath)
		// Defaults are already set in NewConfigService, ensure AI defaults are there too
		if s.config.AIProviderSettings == nil {
			s.config.AIProviderSettings = &AIProviderSettings{
				CurrentProvider: "openai", // Default provider
				OpenAI:          &OpenAISettings{},
				Anthropic:       &AnthropicSettings{},
				OpenRouter:      &OpenRouterSettings{},
			}
		}
		return nil
	}

	// Temporarily unmarshal into a new struct to preserve defaults if load fails
	var loadedConfig ConfigData
	err = json.Unmarshal(data, &loadedConfig)
	if err != nil {
		return fmt.Errorf("failed to unmarshal config data from %s: %w", s.configPath, err)
	}

	// Assign loaded data to the service config
	s.config = &loadedConfig

	// Ensure nested structures are initialized if they were null/missing in the JSON
	if s.config.Connections == nil {
		s.config.Connections = make(map[string]ConnectionDetails)
	}
	if s.config.ThemeSettings == nil {
		// If theme settings are missing from the file, apply defaults
		s.config.ThemeSettings = &ThemeSettings{Mode: "system", BaseTheme: "claude"}
	}
	// Ensure AIProviderSettings and nested structs are non-nil
	if s.config.AIProviderSettings == nil {
		s.config.AIProviderSettings = &AIProviderSettings{
			CurrentProvider: "openai", // Default provider
			OpenAI:          &OpenAISettings{},
			Anthropic:       &AnthropicSettings{},
			OpenRouter:      &OpenRouterSettings{},
		}
	} else {
		if s.config.AIProviderSettings.CurrentProvider == "" { // Ensure default if loaded empty
			s.config.AIProviderSettings.CurrentProvider = "openai"
		}
		if s.config.AIProviderSettings.OpenAI == nil {
			s.config.AIProviderSettings.OpenAI = &OpenAISettings{}
		}
		if s.config.AIProviderSettings.Anthropic == nil {
			s.config.AIProviderSettings.Anthropic = &AnthropicSettings{}
		}
		if s.config.AIProviderSettings.OpenRouter == nil {
			s.config.AIProviderSettings.OpenRouter = &OpenRouterSettings{}
		}
	}

	return nil
}

// saveConfig is an internal helper that writes the current config data to disk.
// It assumes the caller holds the necessary lock.
func (s *ConfigService) saveConfig() error {
	// Ensure the directory exists
	configDir := filepath.Dir(s.configPath)
	Info("Attempting to create directory: %s", configDir)
	if err := os.MkdirAll(configDir, 0750); err != nil {
		Info("ERROR during MkdirAll: %v", err)
		return fmt.Errorf("failed to create config directory %s: %w", configDir, err)
	}
	Info("Directory ensured successfully.")

	Info("Attempting to marshal config data...")
	data, err := json.MarshalIndent(s.config, "", "  ")
	if err != nil {
		Info("ERROR during MarshalIndent: %v", err)
		return fmt.Errorf("failed to marshal config data: %w", err)
	}
	Info("Config marshaled successfully. Size: %d bytes", len(data))

	Info("Attempting to write file: %s", s.configPath)
	if err := os.WriteFile(s.configPath, data, 0600); err != nil {
		Info("ERROR during WriteFile: %v", err)
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

	// Return a copy to prevent external modification of the internal map
	connectionsCopy := make(map[string]ConnectionDetails)
	if s.config.Connections != nil {
		for name, details := range s.config.Connections {
			details.Name = name // Populate the Name field
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

	details, found := s.config.Connections[name]
	if found {
		details.Name = name // Populate the Name field
	}
	return details, found, nil
}

// RecordConnectionUsage updates the LastUsed timestamp for a connection.
func (s *ConfigService) RecordConnectionUsage(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	details, found := s.config.Connections[name]
	if !found {
		// Should we error? Or just ignore? Ignoring is safer if called speculatively.
		Info("Warning: Attempted to record usage for non-existent connection '%s'", name)
		return nil // Don't block connection flow if name somehow doesn't exist
		// return fmt.Errorf("connection '%s' not found", name) // Stricter alternative
	}

	details.LastUsed = time.Now().Format(time.RFC3339)
	s.config.Connections[name] = details // Update the map with the modified struct

	// Save the configuration
	return s.saveConfig()
}

// --- Theme Settings Management Methods ---

// GetThemeSettings retrieves the current theme settings.
func (s *ConfigService) GetThemeSettings() (*ThemeSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Return a copy to prevent modification of internal state?
	// For simple structs, returning the pointer might be okay for read,
	// but save must go through SaveThemeSettings.
	if s.config.ThemeSettings == nil {
		// Should not happen due to initialization, but return default if it does
		return &ThemeSettings{Mode: "system", BaseTheme: "claude"}, nil
	}
	// Return a copy to be safe
	settingsCopy := *s.config.ThemeSettings
	return &settingsCopy, nil
}

// SaveThemeSettings updates and saves the theme settings.
func (s *ConfigService) SaveThemeSettings(settings ThemeSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Basic validation (optional but good practice)
	if settings.Mode == "" { settings.Mode = "system" } // Default if empty
	if settings.BaseTheme == "" { settings.BaseTheme = "claude" } // Default if empty
	// Add validation against availableThemes if needed

	s.config.ThemeSettings = &settings // Update the internal config
	return s.saveConfig() // Save the entire config file
}

// --- AI Provider Settings Management Methods ---

// GetAIProviderSettings retrieves the current AI provider settings.
func (s *ConfigService) GetAIProviderSettings() (*AIProviderSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.config.AIProviderSettings == nil {
		// Should not happen due to initialization, but return default if it does
		return &AIProviderSettings{
			CurrentProvider: "openai", // Default provider
			OpenAI:          &OpenAISettings{},
			Anthropic:       &AnthropicSettings{},
			OpenRouter:      &OpenRouterSettings{},
		}, nil
	}
	// Return a copy to prevent modification of internal state
	settingsCopy := *s.config.AIProviderSettings
	if settingsCopy.CurrentProvider == "" {
		settingsCopy.CurrentProvider = "openai" // Ensure CurrentProvider has a default if empty
	}
	if settingsCopy.OpenAI == nil { settingsCopy.OpenAI = &OpenAISettings{} } // Ensure nested are non-nil
	if settingsCopy.Anthropic == nil { settingsCopy.Anthropic = &AnthropicSettings{} }
	if settingsCopy.OpenRouter == nil { settingsCopy.OpenRouter = &OpenRouterSettings{} }

	return &settingsCopy, nil
}

// SaveAIProviderSettings updates and saves the AI provider settings.
func (s *ConfigService) SaveAIProviderSettings(settings AIProviderSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Basic validation for CurrentProvider
	isValidProvider := false
	validProviders := []string{"openai", "anthropic", "openrouter"} // Keep this in sync with frontend
	for _, p := range validProviders {
		if settings.CurrentProvider == p {
			isValidProvider = true
			break
		}
	}
	if !isValidProvider {
		Info("Warning: Invalid CurrentProvider '%s' received in SaveAIProviderSettings. Defaulting to 'openai'.", settings.CurrentProvider)
		settings.CurrentProvider = "openai" // Default if invalid
	}

	// Ensure nested pointers are handled correctly before saving
	if settings.OpenAI == nil { settings.OpenAI = &OpenAISettings{} }

	s.config.AIProviderSettings = &settings // Update the internal config
	return s.saveConfig() // Save the entire config file
}
