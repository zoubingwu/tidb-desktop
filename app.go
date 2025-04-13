package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/zoubingwu/tidb-desktop/services"
)

// App struct holds application context and services
type App struct {
	ctx              context.Context
	dbService        *services.DatabaseService
	aiService        *services.AIService
	configService    *services.ConfigService       // Add Config Service
	activeConnection *services.ConnectionDetails // Still useful for current session
}

// NewApp creates a new App application struct
func NewApp() *App {
	dbService := services.NewDatabaseService()
	aiService, err := services.NewAIService()
	if err != nil {
		fmt.Printf("Error initializing AI Service: %v. AI features disabled.\n", err)
		aiService = nil
	}
	configService, err := services.NewConfigService() // Initialize Config Service
	if err != nil {
		// This is more critical, perhaps panic or return error if config cannot be handled
		panic(fmt.Sprintf("FATAL: Failed to initialize Config Service: %v", err))
	}

	return &App{
		dbService:     dbService,
		aiService:     aiService,
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

// ConnectUsingDetails establishes the *current active* connection using provided details.
// It does NOT save the connection permanently. Use SaveConnection for that.
// Returns the connection details on success for frontend confirmation.
func (a *App) ConnectUsingDetails(details services.ConnectionDetails) (*services.ConnectionDetails, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("app context not initialized")
	}
	// Test the connection first
	success, err := a.dbService.TestConnection(a.ctx, details)
	if err != nil {
		return nil, fmt.Errorf("connection test failed: %w", err)
	}
	if !success {
		return nil, fmt.Errorf("connection test reported failure, please check details")
	}

	// Store as the *active* connection for this session
	a.activeConnection = &details
	fmt.Printf("Session connection activated: %+v\n", *a.activeConnection)

	// Emit event to notify frontend the active session is ready
	runtime.EventsEmit(a.ctx, "connection:established", details)

	return &details, nil // Return details on success
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

// InferConnectionDetailsFromClipboard reads clipboard and asks AI to infer details.
func (a *App) InferConnectionDetailsFromClipboard() (*services.ConnectionDetails, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("app context not initialized")
	}
	if a.aiService == nil {
		return nil, fmt.Errorf("AI service is not available (check API key or initialization errors)")
	}

	clipboardText, err := a.ReadClipboard()
	if err != nil {
		return nil, fmt.Errorf("failed to read clipboard: %w", err)
	}

	if strings.TrimSpace(clipboardText) == "" {
		return nil, fmt.Errorf("clipboard is empty")
	}

	return a.aiService.InferConnectionDetails(a.ctx, clipboardText)
}

// Greet function remains as an example binding
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}
