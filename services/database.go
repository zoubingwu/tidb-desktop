package services

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"strings"

	mysql "github.com/go-sql-driver/mysql"
)

// ConnectionDetails defines the structure for DB connection info from the frontend
// Moved definition here and exported it.
type ConnectionDetails struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	DBName   string `json:"dbName"`
	UseTLS   bool   `json:"useTLS"` // Optional flag for explicit TLS control
}

// DatabaseService handles DB operations
type DatabaseService struct {
	// You could add connection pooling here if needed for performance
}

// NewDatabaseService creates a new DatabaseService
func NewDatabaseService() *DatabaseService {
	return &DatabaseService{}
}

// buildDSN creates the Data Source Name string for the connection
// Modified to return DSN and a boolean indicating if TLS should be used.
func buildDSN(details ConnectionDetails) (string, bool) {
	port := details.Port
	if port == "" {
		port = "4000" // Default TiDB port
	}

	// Base DSN string
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true",
		details.User, details.Password, details.Host, port, details.DBName)

	// Determine if TLS should be used
	// Use explicit flag if provided, otherwise check host for tidbcloud.com
	useTLS := details.UseTLS || strings.Contains(details.Host, ".tidbcloud.com")

	if useTLS {
		// Append the TLS parameter
		dsn += "&tls=tidb" // Use & since parseTime=true is already there
	}

	return dsn, useTLS
}

// getDBConnection handles creating the DB connection, including TLS setup
func getDBConnection(ctx context.Context, details ConnectionDetails) (*sql.DB, error) {
	dsn, useTLS := buildDSN(details)

	if useTLS {
		// Register the TLS config *before* opening the connection.
		// Using the specific host as ServerName is crucial for verification.
		// Note: Registering dynamically like this has potential concurrency issues
		// if many connections are opened simultaneously, but less likely in a desktop app.
		// A more robust approach might involve pre-registering common configs or managing
		// configs externally if needed.
		err := mysql.RegisterTLSConfig("tidb", &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: details.Host,
		})
		// Allow re-registration for potentially different hosts within the app's lifetime
		// This specific error check handles the case where "tidb" is already registered.
		// We want to overwrite it with potentially new ServerName if host changes.
		// This check is specific to how `go-sql-driver/mysql` handles registration.
		if err != nil && !strings.Contains(err.Error(), "already registered") {
            // If it's a different error, return it
			return nil, fmt.Errorf("failed to register TLS config: %w", err)
		}
        // If the error was "already registered", we proceed, effectively overwriting.
	}

	// Open the database connection
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	return db, nil
}

// TestConnection attempts to ping the database
func (s *DatabaseService) TestConnection(ctx context.Context, details ConnectionDetails) (bool, error) {
	db, err := getDBConnection(ctx, details) // Use the helper
	if err != nil {
		// Wrap the error from getDBConnection
		return false, fmt.Errorf("connection setup failed: %w", err)
	}
	defer db.Close()

	// Ping the database to verify connection
	err = db.PingContext(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to ping database: %w", err)
	}
	return true, nil
}

// ExecuteSQL runs a query and returns results or execution status
func (s *DatabaseService) ExecuteSQL(ctx context.Context, details ConnectionDetails, query string) (interface{}, error) {
	db, err := getDBConnection(ctx, details) // Use the helper
	if err != nil {
		// Wrap the error from getDBConnection
		return nil, fmt.Errorf("connection setup failed: %w", err)
	}
	defer db.Close()

	// Try running as a query first (SELECT)
	rows, err := db.QueryContext(ctx, query)
	if err == nil {
		defer rows.Close()
		columns, err := rows.Columns()
		if err != nil {
			return nil, fmt.Errorf("failed to get columns: %w", err)
		}

		var results []map[string]interface{}
		for rows.Next() {
            // Create slices for scanning, handling various data types
			values := make([]interface{}, len(columns))
            scanArgs := make([]interface{}, len(columns))
            for i := range values {
                scanArgs[i] = &values[i]
            }

			err = rows.Scan(scanArgs...)
			if err != nil {
				return nil, fmt.Errorf("failed to scan row: %w", err)
			}

			rowMap := make(map[string]interface{})
			for i, col := range columns {
                // Handle potential nil values and convert byte slices (like strings)
                val := values[i]
                 if b, ok := val.([]byte); ok {
                     rowMap[col] = string(b)
                 } else {
                     rowMap[col] = val
                 }
			}
			results = append(results, rowMap)
		}
        if err = rows.Err(); err != nil {
             return nil, fmt.Errorf("error iterating rows: %w", err)
         }
		return results, nil
	}

	// If QueryContext failed, try ExecContext (INSERT, UPDATE, DELETE, etc.)
	result, execErr := db.ExecContext(ctx, query)
	if execErr != nil {
		// Return a combined error if both attempts failed
		return nil, fmt.Errorf("query failed: Query error (%v), Exec error (%v)", err, execErr)
	}

	rowsAffected, _ := result.RowsAffected()
	lastInsertId, _ := result.LastInsertId() // May not be relevant for TiDB often

	return map[string]interface{}{
		"rowsAffected": rowsAffected,
		"lastInsertId": lastInsertId,
	}, nil
}


