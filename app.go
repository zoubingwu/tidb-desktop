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
	activeConnection *services.ConnectionDetails // Store active connection details
}

// NewApp creates a new App application struct
func NewApp() *App {
	dbService := services.NewDatabaseService()
	aiService, err := services.NewAIService()
	if err != nil {
		fmt.Printf("Error initializing AI Service: %v. AI features disabled.\n", err)
		aiService = nil
	}

	return &App{
		dbService: dbService,
		aiService: aiService,
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

// EstablishConnection tests the connection and saves details on success.
// It also emits an event for the frontend.
func (a *App) EstablishConnection(details services.ConnectionDetails) (bool, error) {
	if a.ctx == nil {
		return false, fmt.Errorf("app context not initialized")
	}
	// Test the connection first
	success, err := a.dbService.TestConnection(a.ctx, details)
	if err != nil {
		return false, fmt.Errorf("connection test failed: %w", err)
	}
	if !success {
		return false, fmt.Errorf("connection test reported failure, please check details")
	}

	// Store the details on successful test
	a.activeConnection = &details
	fmt.Printf("Connection details stored: %+v\n", *a.activeConnection) // Log for debugging

	// Emit event to notify frontend
	runtime.EventsEmit(a.ctx, "connection:established", details)

	return true, nil
}

// Disconnect clears the active connection details.
func (a *App) Disconnect() {
	fmt.Println("Disconnecting...")
	a.activeConnection = nil
	// Optionally emit an event if the frontend needs to react specifically
	// runtime.EventsEmit(a.ctx, "connection:disconnected")
}

// GetActiveConnection returns the currently stored connection details (if any).
// Useful if the frontend needs to re-fetch details on reload.
func (a *App) GetActiveConnection() *services.ConnectionDetails {
	return a.activeConnection
}

// ExecuteSQL uses the *stored* active connection details to execute a query.
func (a *App) ExecuteSQL(query string) (any, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("app context not initialized")
	}
	if a.activeConnection == nil {
		return nil, fmt.Errorf("no active database connection established")
	}
	fmt.Printf("Executing SQL with stored connection: %+v\n", *a.activeConnection) // Log for debugging
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
