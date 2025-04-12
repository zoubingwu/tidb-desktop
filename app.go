package main

import (
	"context"
	"fmt"

	"github.com/zoubingwu/tidb-desktop/services"
)

// App struct holds application context and services
type App struct {
	ctx       context.Context
	dbService *services.DatabaseService
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		dbService: services.NewDatabaseService(), // Initialize the DB service
	}
}

// startup is called when the app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// Perform any setup here
}

// shutdown is called when the app terminates.
func (a *App) shutdown(ctx context.Context) {
	// Perform cleanup here
}

// --- Exposed Methods ---

// TestConnection attempts to connect to the database.
// Returns true on success, error message otherwise.
func (a *App) TestConnection(details services.ConnectionDetails) (bool, error) {
	return a.dbService.TestConnection(a.ctx, details)
}

// ExecuteSQL connects to the DB and executes the given SQL query.
// Returns results (e.g., as []map[string]interface{} for SELECT, or affected rows for others) or an error.
func (a *App) ExecuteSQL(details services.ConnectionDetails, query string) (interface{}, error) {
	return a.dbService.ExecuteSQL(a.ctx, details, query)
}

// Greet function remains as an example binding
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}
