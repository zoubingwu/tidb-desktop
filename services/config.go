package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync" // Use a mutex for safe concurrent access if needed, though less critical for simple desktop apps
)

const (
	configDirName  = ".tidb-desktop"
	configFileName = "config.json"
)

// ConfigData defines the structure of the entire configuration file.
type ConfigData struct {
	// Use ConnectionDetails from database.go
	Connections map[string]ConnectionDetails `json:"connections"`
	// Add other configuration fields here later, e.g., settings
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

	configDirPath := filepath.Join(homeDir, configDirName)
	configFilePath := filepath.Join(configDirPath, configFileName)

	service := &ConfigService{
		configPath: configFilePath,
		config:     &ConfigData{Connections: make(map[string]ConnectionDetails)}, // Initialize map
	}

	// Load existing config on startup
	if err := service.loadConfig(); err != nil {
		// If loading fails (e.g., corrupted JSON), log but continue with empty config
		fmt.Printf("Warning: Failed to load config file %s: %v. Starting with empty config.\n", configFilePath, err)
		// Ensure map is initialized even if loading fails partially
		if service.config.Connections == nil {
			service.config.Connections = make(map[string]ConnectionDetails)
		}
	} else {
		fmt.Printf("Config loaded successfully from %s\n", configFilePath)
	}

	return service, nil
}

// loadConfig reads the config file from disk.
func (s *ConfigService) loadConfig() error {
	s.mu.Lock() // Acquire write lock as we are modifying s.config
	defer s.mu.Unlock()

	// Check if file exists
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		fmt.Printf("Config file %s does not exist, creating empty config.\n", s.configPath)
		// Ensure map is initialized
		if s.config.Connections == nil {
			s.config.Connections = make(map[string]ConnectionDetails)
		}
		// No need to save yet, will save when data is added
		return nil // Not an error if file doesn't exist
	}

	data, err := os.ReadFile(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", s.configPath, err)
	}

	// If the file is empty, initialize config
	if len(data) == 0 {
		fmt.Printf("Config file %s is empty, creating empty config.\n", s.configPath)
		if s.config.Connections == nil {
			s.config.Connections = make(map[string]ConnectionDetails)
		}
		return nil
	}

	// Unmarshal the data
	err = json.Unmarshal(data, s.config)
	if err != nil {
		return fmt.Errorf("failed to unmarshal config data from %s: %w", s.configPath, err)
	}

	// Ensure the map is initialized if JSON was like {"connections": null}
	if s.config.Connections == nil {
		s.config.Connections = make(map[string]ConnectionDetails)
	}

	return nil
}

// saveConfig is an internal helper that writes the current config data to disk.
// It assumes the caller holds the necessary lock.
func (s *ConfigService) saveConfig() error {
	// Ensure the directory exists
	configDir := filepath.Dir(s.configPath)
	fmt.Printf("Attempting to create directory: %s\n", configDir)
	if err := os.MkdirAll(configDir, 0750); err != nil {
		fmt.Printf("ERROR during MkdirAll: %v\n", err)
		return fmt.Errorf("failed to create config directory %s: %w", configDir, err)
	}
	fmt.Printf("Directory ensured successfully.\n")

	fmt.Printf("Attempting to marshal config data...\n")
	data, err := json.MarshalIndent(s.config, "", "  ")
	if err != nil {
		fmt.Printf("ERROR during MarshalIndent: %v\n", err)
		return fmt.Errorf("failed to marshal config data: %w", err)
	}
	fmt.Printf("Config marshaled successfully. Size: %d bytes\n", len(data))

	fmt.Printf("Attempting to write file: %s\n", s.configPath)
	if err := os.WriteFile(s.configPath, data, 0600); err != nil {
		fmt.Printf("ERROR during WriteFile: %v\n", err)
		return fmt.Errorf("failed to write config file %s: %w", s.configPath, err)
	}
	fmt.Printf("Config saved successfully to %s\n", s.configPath)
	return nil
}

// --- Connection Management Methods ---

// GetAllConnections returns a copy of all stored connections.
func (s *ConfigService) GetAllConnections() (map[string]ConnectionDetails, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to prevent external modification of the internal map
	connectionsCopy := make(map[string]ConnectionDetails)
	for name, details := range s.config.Connections {
		connectionsCopy[name] = details // Assuming ConnectionDetails is a struct (value type)
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
	return details, found, nil
}
