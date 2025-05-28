package main

import (
	"context"
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/zoubingwu/tidb-desktop/services"
)

// App struct holds application context and services
type App struct {
	ctx                context.Context
	dbService          *services.DatabaseService
	configService      *services.ConfigService
	metadataService    *services.MetadataService
	activeConnection   *services.ConnectionDetails
	activeConnectionID string // Store the ID of the active connection
}

// NewApp creates a new App application struct
func NewApp() *App {
	if err := services.InitLogger(); err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}

	dbService := services.NewDatabaseService()
	configService, err := services.NewConfigService()
	if err != nil {
		// This is more critical, perhaps panic or return error if config cannot be handled
		panic(fmt.Sprintf("FATAL: Failed to initialize Config Service: %v", err))
	}

	metadataService, err := services.NewMetadataService(configService, dbService)
	if err != nil {
		panic(fmt.Sprintf("FATAL: Failed to initialize Metadata Service: %v", err))
	}

	return &App{
		dbService:       dbService,
		configService:   configService,
		metadataService: metadataService,
		// activeConnection starts as nil
	}
}

// startup is called when the app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Load window settings
	settings, err := a.configService.GetWindowSettings()
	if err != nil {
		services.LogInfo("Warning: Failed to get window settings: %v. Using defaults.", err)
		// Use Wails default (centered) or define fallback defaults here
		runtime.WindowCenter(a.ctx)
	} else {
		if settings.IsMaximized {
			runtime.WindowMaximise(a.ctx)
		} else {
			// Only set size and position if not maximized and position is valid
			if settings.Width > 0 && settings.Height > 0 {
				runtime.WindowSetSize(a.ctx, settings.Width, settings.Height)
			}
			if settings.X != -1 && settings.Y != -1 { // -1 means use default/centered
				runtime.WindowSetPosition(a.ctx, settings.X, settings.Y)
			} else {
				runtime.WindowCenter(a.ctx) // Center if no specific position was saved
			}
		}
	}

	// Subscribe to metadata extraction events
	runtime.EventsOn(a.ctx, "metadata:extraction:start", func(optionalData ...interface{}) {
		connectionID := optionalData[0].(string)
		force := optionalData[1].(bool)
		dbName := optionalData[2].(string)

		services.LogInfo("Metadata extraction started for connection ID '%s' and force extraction: %v", connectionID, force)

		if connectionID == "" {
			connectionID = a.activeConnectionID
		}

		var metadata *services.ConnectionMetadata
		var err error

		if force {
			if dbName != "" {
				metadata, err = a.metadataService.ExtractMetadata(a.ctx, connectionID, dbName)
			} else {
				metadata, err = a.metadataService.ExtractMetadata(a.ctx, connectionID)
			}
		} else {
			metadata, err = a.metadataService.GetMetadata(a.ctx, connectionID)
		}

		if err != nil {
			services.LogError("Background metadata extraction failed for connection ID '%s': %v", connectionID, err)
			runtime.EventsEmit(a.ctx, "metadata:extraction:failed", err.Error())
		} else {
			services.LogInfo("Background metadata extraction completed for connection ID '%s'", connectionID)
			// Save the freshly extracted/retrieved metadata to disk
			if errSave := a.metadataService.SaveMetadata(connectionID); errSave != nil {
				services.LogError("Failed to save metadata after extraction for connection ID '%s': %v", connectionID, errSave)
			}
			runtime.EventsEmit(a.ctx, "metadata:extraction:completed", metadata)
		}
	})
}

// shutdown is called when the app terminates.
func (a *App) shutdown(ctx context.Context) {
	// Save current window state
	width, height := runtime.WindowGetSize(a.ctx)
	x, y := runtime.WindowGetPosition(a.ctx)
	isMaximized := runtime.WindowIsMaximised(a.ctx)

	settings := services.WindowSettings{
		Width:       width,
		Height:      height,
		X:           x,
		Y:           y,
		IsMaximized: isMaximized,
	}

	if err := a.configService.SaveWindowSettings(settings); err != nil {
		services.LogInfo("Error saving window settings on shutdown: %v", err)
	} else {
		services.LogInfo("Window settings saved on shutdown: %+v", settings)
	}

	// Perform other cleanup here if needed
	runtime.EventsOff(a.ctx, "metadata:extraction:start")
}

// --- Exposed Methods ---

// TestConnection attempts to connect to the database.
// Returns true on success, error message otherwise.
func (a *App) TestConnection(details services.ConnectionDetails) (bool, error) {
	if a.ctx == nil {
		return false, fmt.Errorf("app context not initialized")
	}
	return a.dbService.TestConnection(a.ctx, details)
}

// ConnectUsingSaved establishes the *current active* connection using a saved connection ID.
// Returns the connection details on success.
func (a *App) ConnectUsingSaved(connectionID string) (*services.ConnectionDetails, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("app context not initialized")
	}
	services.LogInfo("Attempting to connect using saved connection ID: %s", connectionID)
	details, found, err := a.configService.GetConnection(connectionID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve saved connection '%s': %w", connectionID, err)
	}
	if !found {
		return nil, fmt.Errorf("saved connection '%s' not found", connectionID)
	}

	// Test the retrieved connection
	success, err := a.dbService.TestConnection(a.ctx, details)
	if err != nil {
		return nil, fmt.Errorf("connection test failed for saved connection '%s': %w", details.Name, err)
	}
	if !success {
		return nil, fmt.Errorf("connection test reported failure for saved connection '%s'", details.Name)
	}

	// Store as the *active* connection for this session
	a.activeConnection = &details
	a.activeConnectionID = connectionID
	services.LogInfo("Connection '%s' activated successfully", details.Name)

	// Debug: Log the connectionID and details.ID to check for discrepancies
	services.LogInfo("DEBUG: connectionID=%s, details.ID=%s, details.Name=%s", connectionID, details.ID, details.Name)

	// Record usage timestamp in config
	if err := a.configService.RecordConnectionUsage(connectionID); err != nil {
		// Log the error but don't fail the connection for this
		services.LogInfo("Warning: Failed to record usage for connection '%s': %v", details.Name, err)
	}

	// Load metadata into memory for this connection
	metadata, err := a.metadataService.LoadMetadata(a.ctx, connectionID)
	if err != nil {
		// Log the error but don't fail the connection
		services.LogInfo("Warning: Failed to load metadata for connection '%s': %v", details.Name, err)
		// Emit extraction failed event to notify UI
		runtime.EventsEmit(a.ctx, "metadata:extraction:failed", err.Error())
	} else if !metadata.LastExtracted.IsZero() {
		// Only emit completed event if we actually loaded existing metadata from file
		runtime.EventsEmit(a.ctx, "metadata:extraction:completed", metadata)
	}
	// If LastExtracted is zero, it means we created an empty structure - frontend will trigger extraction

	// Emit event to notify frontend the active session is ready
	runtime.EventsEmit(a.ctx, "connection:established", details)

	return &details, nil
}

// Disconnect clears the active connection details for the current session.
func (a *App) Disconnect() {
	services.LogInfo("Disconnecting session...")
	a.activeConnection = nil
	a.activeConnectionID = ""
	// Optionally emit an event if the frontend needs to react specifically
	runtime.EventsEmit(a.ctx, "connection:disconnected") // Notify frontend
}

// GetActiveConnection returns the connection details for the current session.
func (a *App) GetActiveConnection() *services.ConnectionDetails {
	return a.activeConnection
}

// --- Configuration Management Methods ---

// ListSavedConnections returns all connection names and details from config.
func (a *App) ListSavedConnections() (map[string]services.ConnectionDetails, error) {
	return a.configService.GetAllConnections()
}

// SaveConnection saves or updates connection details in the config file.
// Returns the connection ID.
func (a *App) SaveConnection(details services.ConnectionDetails) (string, error) {
	if details.Name == "" {
		return "", fmt.Errorf("connection name cannot be empty")
	}
	services.LogInfo("Saving connection details for: %s", details.Name)
	connectionID, err := a.configService.AddOrUpdateConnection(details)
	if err != nil {
		services.LogInfo("Failed to save connection '%s': %v", details.Name, err)
		return "", err
	}
	services.LogInfo("Connection '%s' saved successfully with ID: %s", details.Name, connectionID)
	return connectionID, nil
}

// DeleteSavedConnection removes a connection from the config file by ID.
func (a *App) DeleteSavedConnection(connectionID string) error {
	if connectionID == "" {
		return fmt.Errorf("connection ID cannot be empty")
	}

	// Get connection details for logging before deletion
	details, found, _ := a.configService.GetConnection(connectionID)
	connectionName := "unknown"
	if found {
		connectionName = details.Name
	}

	services.LogInfo("Deleting saved connection '%s' (ID: %s)", connectionName, connectionID)
	err := a.configService.DeleteConnection(connectionID)
	if err != nil {
		return err
	}

	// Delete metadata for this connection
	if err := a.metadataService.DeleteConnectionMetadata(connectionID); err != nil {
		// Log the error but don't fail the deletion
		services.LogInfo("Warning: Failed to delete metadata for connection '%s': %v", connectionName, err)
	}

	// If the deleted connection was the active one, disconnect the session
	if a.activeConnectionID == connectionID {
		services.LogInfo("Disconnecting active session as it was deleted")
		a.Disconnect()
	}

	return nil
}

// --- SQL Execution Method ---

// ExecuteSQL uses the *active session connection* details to execute a query.
func (a *App) ExecuteSQL(query string) (*services.SQLResult, error) {
	services.LogInfo("Executing SQL with active connection: %s", query)
	if a.ctx == nil {
		return nil, fmt.Errorf("app context not initialized")
	}
	if a.activeConnection == nil {
		return nil, fmt.Errorf("no active database connection established for this session")
	}
	result, err := a.dbService.ExecuteSQL(a.ctx, *a.activeConnection, query)
	if err != nil {
		services.LogInfo("SQL execution failed: %v", err)
		return nil, err
	}
	services.LogInfo("SQL execution completed successfully")
	return result, nil
}

// ListDatabases retrieves a list of database/schema names accessible by the connection.
func (a *App) ListDatabases() ([]string, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("app context not initialized")
	}
	if a.activeConnection == nil {
		return nil, fmt.Errorf("no active connection")
	}

	// Delegate to DatabaseService
	return a.dbService.ListDatabases(a.ctx, *a.activeConnection)
}

// ListTables retrieves a list of table names from the specified database.
// If dbName is empty, it uses the database specified in the active connection details.
func (a *App) ListTables(dbName string) ([]string, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("app context not initialized")
	}
	if a.activeConnection == nil {
		return nil, fmt.Errorf("no active connection")
	}

	// Delegate to DatabaseService
	return a.dbService.ListTables(a.ctx, *a.activeConnection, dbName)
}

// GetTableData retrieves data (rows and columns) for a specific table with pagination and filtering.
// filterParams is a placeholder for potential future filtering implementation.
func (a *App) GetTableData(dbName string, tableName string, limit int, offset int, filterParams *map[string]any) (*services.TableDataResponse, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("app context not initialized")
	}
	if a.activeConnection == nil {
		return nil, fmt.Errorf("no active connection")
	}

	// Delegate to DatabaseService
	return a.dbService.GetTableData(a.ctx, *a.activeConnection, dbName, tableName, limit, offset, filterParams)
}

// GetTableSchema retrieves the detailed schema/structure for a specific table.
func (a *App) GetTableSchema(dbName string, tableName string) (*services.TableSchema, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("app context not initialized")
	}
	if a.activeConnection == nil {
		return nil, fmt.Errorf("no active connection")
	}

	// Delegate to DatabaseService
	return a.dbService.GetTableSchema(a.ctx, *a.activeConnection, dbName, tableName)
}

// --- Theme Settings ---

// GetThemeSettings retrieves the currently saved theme settings.
func (a *App) GetThemeSettings() (*services.ThemeSettings, error) {
	if a.configService == nil {
		return nil, fmt.Errorf("config service not initialized")
	}
	return a.configService.GetThemeSettings()
}

// SaveThemeSettings saves the provided theme settings to the config file.
func (a *App) SaveThemeSettings(settings services.ThemeSettings) error {
	services.LogInfo("Saving theme settings: %+v\n", settings)
	if a.configService == nil {
		return fmt.Errorf("config service not initialized")
	}
	return a.configService.SaveThemeSettings(settings)
}

// --- AI Provider Settings ---

// GetAIProviderSettings retrieves the currently saved AI provider settings.
func (a *App) GetAIProviderSettings() (*services.AIProviderSettings, error) {
	if a.configService == nil {
		return nil, fmt.Errorf("config service not initialized")
	}
	return a.configService.GetAIProviderSettings()
}

// SaveAIProviderSettings saves the provided AI provider settings to the config file.
func (a *App) SaveAIProviderSettings(settings services.AIProviderSettings) error {
	services.LogInfo("Saving AI provider settings")
	if a.configService == nil {
		return fmt.Errorf("config service not initialized")
	}
	return a.configService.SaveAIProviderSettings(settings)
}

// --- Window Settings (not directly exposed to frontend, but used internally) ---

// GetWindowSettings retrieves the currently saved window settings.
// This might be useful if you need to expose it to the frontend later.
func (a *App) GetWindowSettings() (*services.WindowSettings, error) {
	if a.configService == nil {
		return nil, fmt.Errorf("config service not initialized")
	}
	return a.configService.GetWindowSettings()
}

// SaveWindowSettings saves the provided window settings to the config file.
// This might be useful if you need to expose it to the frontend later.
func (a *App) SaveWindowSettings(settings services.WindowSettings) error {
	if a.configService == nil {
		return fmt.Errorf("config service not initialized")
	}
	return a.configService.SaveWindowSettings(settings)
}

// --- Database Metadata Methods ---

// GetDatabaseMetadata retrieves metadata for the current connection
func (a *App) GetDatabaseMetadata() (*services.ConnectionMetadata, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("app context not initialized")
	}
	if a.activeConnection == nil {
		return nil, fmt.Errorf("no active connection")
	}

	return a.metadataService.GetMetadata(a.ctx, a.activeConnectionID)
}

// ExtractDatabaseMetadata forces a fresh extraction of database metadata
func (a *App) ExtractDatabaseMetadata(dbName ...string) (*services.ConnectionMetadata, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("app context not initialized")
	}
	if a.activeConnection == nil {
		return nil, fmt.Errorf("no active connection")
	}

	var optionalDbName []string
	if len(dbName) > 0 && dbName[0] != "" {
		optionalDbName = dbName
	}

	metadata, err := a.metadataService.ExtractMetadata(a.ctx, a.activeConnectionID, optionalDbName...)
	if err != nil {
		return nil, err
	}

	// Save the extracted metadata to disk
	if saveErr := a.metadataService.SaveMetadata(a.activeConnectionID); saveErr != nil {
		services.LogError("Failed to save metadata after extraction: %v", saveErr)
		// Don't fail the operation, just log the error
	}

	return metadata, nil
}

// UpdateAIDescription updates the AI-generated description for a database component
func (a *App) UpdateAIDescription(dbName string, targetType string, tableName string, columnName string, description string) error {
	if a.ctx == nil {
		return fmt.Errorf("app context not initialized")
	}
	if a.activeConnection == nil {
		return fmt.Errorf("no active connection")
	}

	target := services.DescriptionTarget{
		Type:       targetType,
		TableName:  tableName,
		ColumnName: columnName,
	}

	err := a.metadataService.UpdateAIDescription(a.ctx, a.activeConnectionID, dbName, target, description)
	if err != nil {
		return fmt.Errorf("failed to update AI description: %w", err)
	}

	// Save the updated metadata to disk
	if saveErr := a.metadataService.SaveMetadata(a.activeConnectionID); saveErr != nil {
		services.LogError("Failed to save metadata after AI description update: %v", saveErr)
		// Don't fail the operation, just log the error
	}

	return nil
}
