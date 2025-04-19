package main

import (
	"context"
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/zoubingwu/tidb-desktop/services"
)

// App struct holds application context and services
type App struct {
	ctx              context.Context
	dbService        *services.DatabaseService
	configService    *services.ConfigService
	activeConnection *services.ConnectionDetails
}

// NewApp creates a new App application struct
func NewApp() *App {
	dbService := services.NewDatabaseService()
	configService, err := services.NewConfigService()
	if err != nil {
		// This is more critical, perhaps panic or return error if config cannot be handled
		panic(fmt.Sprintf("FATAL: Failed to initialize Config Service: %v", err))
	}

	return &App{
		dbService:     dbService,
		configService: configService,
		// activeConnection starts as nil
	}
}

// startup is called when the app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	runtime.WindowCenter(a.ctx)
}

// shutdown is called when the app terminates.
func (a *App) shutdown(ctx context.Context) {
	// Perform cleanup here if needed
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

// ConnectUsingSaved establishes the *current active* connection using a saved connection name.
// Returns the connection details on success.
func (a *App) ConnectUsingSaved(name string) (*services.ConnectionDetails, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("app context not initialized")
	}
	details, found, err := a.configService.GetConnection(name)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve saved connection '%s': %w", name, err)
	}
	if !found {
		return nil, fmt.Errorf("saved connection '%s' not found", name)
	}

	// Test the retrieved connection
	success, err := a.dbService.TestConnection(a.ctx, details)
	if err != nil {
		return nil, fmt.Errorf("connection test failed for saved connection '%s': %w", name, err)
	}
	if !success {
		return nil, fmt.Errorf("connection test reported failure for saved connection '%s'", name)
	}

	// Store as the *active* connection for this session
	a.activeConnection = &details
	fmt.Printf("Session connection activated using saved connection '%s': %+v\n", name, *a.activeConnection)

	// Record usage timestamp in config
	if err := a.configService.RecordConnectionUsage(name); err != nil {
		// Log the error but don't fail the connection for this
		fmt.Printf("Warning: Failed to record usage for connection '%s': %v\n", name, err)
	}

	// Emit event to notify frontend the active session is ready
	runtime.EventsEmit(a.ctx, "connection:established", details)

	return &details, nil
}

// Disconnect clears the active connection details for the current session.
func (a *App) Disconnect() {
	fmt.Println("Disconnecting session...")
	a.activeConnection = nil
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

// SaveConnection saves or updates connection details under a given name in the config file.
func (a *App) SaveConnection(name string, details services.ConnectionDetails) error {
	if name == "" {
		return fmt.Errorf("connection name cannot be empty")
	}
	fmt.Printf("Saving connection '%s': %+v\n", name, details)
	return a.configService.AddOrUpdateConnection(name, details)
}

// DeleteSavedConnection removes a connection from the config file.
func (a *App) DeleteSavedConnection(name string) error {
	if name == "" {
		return fmt.Errorf("connection name cannot be empty")
	}
	fmt.Printf("Deleting saved connection '%s'\n", name)
	err := a.configService.DeleteConnection(name)
	if err != nil {
		return err
	}
	// If the deleted connection was the active one, disconnect the session
	if a.activeConnection != nil {
		// This comparison is tricky if details can change subtly.
		// A better approach might be to store the *name* of the active connection.
		// For now, just disconnect if *any* deletion happens while active.
		// Or, compare based on a unique ID if you add one.
		// Let's keep it simple: if we delete something, ensure active session reflects it
		// if it happened to be the active one (check name in future refactor).
		// For now, a simple disconnect might be okay UX, or do nothing.
		// Let's do nothing for now to avoid complex comparison.
		// If needed later, store active connection name: a.activeConnectionName string
	}
	return nil
}

// --- SQL Execution Method ---

// ExecuteSQL uses the *active session connection* details to execute a query.
func (a *App) ExecuteSQL(query string) (any, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("app context not initialized")
	}
	if a.activeConnection == nil {
		return nil, fmt.Errorf("no active database connection established for this session")
	}
	// fmt.Printf("Executing SQL with active session connection: %+v\n", *a.activeConnection)
	return a.dbService.ExecuteSQL(a.ctx, *a.activeConnection, query)
}

// --- AI Related Methods ---

// ReadClipboard reads text content from the system clipboard.
func (a *App) ReadClipboard() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("app context not initialized")
	}
	return runtime.ClipboardGetText(a.ctx)
}

// ListDatabases retrieves a list of database/schema names accessible by the connection.
func (a *App) ListDatabases() ([]string, error) {
	if a.ctx == nil { return nil, fmt.Errorf("app context not initialized") }
	if a.activeConnection == nil { return nil, fmt.Errorf("no active connection") }

	// Delegate to DatabaseService
	return a.dbService.ListDatabases(a.ctx, *a.activeConnection)
}

// ListTables retrieves a list of table names from the specified database.
// If dbName is empty, it uses the database specified in the active connection details.
func (a *App) ListTables(dbName string) ([]string, error) {
	if a.ctx == nil { return nil, fmt.Errorf("app context not initialized") }
	if a.activeConnection == nil { return nil, fmt.Errorf("no active connection") }

	// Delegate to DatabaseService
	return a.dbService.ListTables(a.ctx, *a.activeConnection, dbName)
}

// GetTableData retrieves data (rows and columns) for a specific table with pagination and filtering.
// filterParams is a placeholder for potential future filtering implementation.
func (a *App) GetTableData(dbName string, tableName string, limit int, offset int, filterParams *map[string]any) (*services.TableDataResponse, error) {
	if a.ctx == nil { return nil, fmt.Errorf("app context not initialized") }
	if a.activeConnection == nil { return nil, fmt.Errorf("no active connection") }

	// Delegate to DatabaseService
	return a.dbService.GetTableData(a.ctx, *a.activeConnection, dbName, tableName, limit, offset, filterParams)
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
	if a.configService == nil {
		return fmt.Errorf("config service not initialized")
	}
	fmt.Printf("Saving theme settings: %+v\n", settings) // Log the received settings
	return a.configService.SaveThemeSettings(settings)
}
