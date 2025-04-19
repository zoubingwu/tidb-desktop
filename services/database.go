package services

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"log"
	"strings"

	mysql "github.com/go-sql-driver/mysql"
)

// ConnectionDetails defines the structure for DB connection info from the frontend
// Moved definition here and exported it.
type ConnectionDetails struct {
	Name     string     `json:"name,omitempty"` // Added: Name for the connection
	Host     string     `json:"host"`
	Port     string     `json:"port"`
	User     string     `json:"user"`
	Password string     `json:"password"`
	DBName   string     `json:"dbName"`
	UseTLS   bool       `json:"useTLS"` // Optional flag for explicit TLS control
	LastUsed string `json:"lastUsed,omitempty"` // Added: Timestamp of last connection
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
func (s *DatabaseService) ExecuteSQL(ctx context.Context, details ConnectionDetails, query string) (any, error) {
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

		var results []map[string]any
		for rows.Next() {
            // Create slices for scanning, handling various data types
			values := make([]any, len(columns))
            scanArgs := make([]any, len(columns))
            for i := range values {
                scanArgs[i] = &values[i]
            }

			err = rows.Scan(scanArgs...)
			if err != nil {
				return nil, fmt.Errorf("failed to scan row: %w", err)
			}

			rowMap := make(map[string]any)
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

	return map[string]any{
		"rowsAffected": rowsAffected,
		"lastInsertId": lastInsertId,
	}, nil
}

// --- Methods for interacting with the connected database ---

// TableColumn represents metadata for a table column
// Moved from app.go
type TableColumn struct {
	Name string `json:"name"`
	Type string `json:"type"` // e.g., "VARCHAR", "INT", "TIMESTAMP", etc. - simplified for now
}

// TableDataResponse holds data and column definitions
// Moved from app.go
type TableDataResponse struct {
	Columns   []TableColumn    `json:"columns"`
	Rows      []map[string]any `json:"rows"`
	TotalRows *int64           `json:"totalRows,omitempty"` // Optional: For pagination
}

// ListDatabases retrieves a list of database/schema names accessible by the connection.
func (s *DatabaseService) ListDatabases(ctx context.Context, details ConnectionDetails) ([]string, error) {
	// Query varies slightly between DBs (e.g., MySQL vs PostgreSQL)
	// SHOW DATABASES; is common for MySQL/TiDB
	query := "SHOW DATABASES;"

	result, err := s.ExecuteSQL(ctx, details, query) // Use ExecuteSQL from this service
	if err != nil {
		return nil, fmt.Errorf("failed to list databases: %w", err)
	}

	dbRows, ok := result.([]map[string]any)
	if !ok {
		// Handle case where result might be different (e.g., single column result)
		// This part might need adjustment based on actual driver behavior
		fmt.Printf("Debug: Unexpected result format when listing databases: %T\n", result)
		// Try to handle single column result?
		if rows, ok := result.([]any); ok {
			var dbNames []string
			for _, row := range rows {
				if dbMap, ok := row.(map[string]any); ok {
					for _, v := range dbMap { // Get the first value assuming it's the name
						if name, ok := v.(string); ok {
							// Filter out system databases if desired
							if name != "information_schema" && name != "performance_schema" && name != "mysql" && name != "sys" {
								dbNames = append(dbNames, name)
							}
							break // Only take the first value
						}
					}
				}
			}
			if len(dbNames) > 0 {
				return dbNames, nil
			}
		}
		return nil, fmt.Errorf("unexpected result format when listing databases")
	}

	var dbNames []string
	// The column name might vary (e.g., 'Database')
	columnKey := ""
	if len(dbRows) > 0 {
		for k := range dbRows[0] {
			columnKey = k // Assume the first column contains the name
			break
		}
	}

	if columnKey == "" && len(dbRows) > 0 {
		return nil, fmt.Errorf("could not determine database name column key from SHOW DATABASES result")
	}

	for _, row := range dbRows {
		if name, ok := row[columnKey].(string); ok {
			// Filter out common system databases
			if name != "information_schema" && name != "performance_schema" && name != "mysql" && name != "sys" {
				dbNames = append(dbNames, name)
			}
		}
	}

	// If the connection details specify a DB, ensure it's in the list or add it?
	// This logic might be better handled in the App layer or frontend based on UX needs.
	// For now, keep the service focused on returning what the DB provides.
	// if details.DBName != "" {
	//  found := false
	//  for _, name := range dbNames {
	//      if name == details.DBName {
	//          found = true
	//          break
	//      }
	//  }
	//  if !found {
	//      // Prepend it if not found (might happen due to permissions)
	//      dbNames = append([]string{details.DBName}, dbNames...)
	//  }
	// }

	return dbNames, nil
}

// ListTables retrieves a list of table names from the specified database.
// If dbName is empty, it uses the database specified in the ConnectionDetails.
func (s *DatabaseService) ListTables(ctx context.Context, details ConnectionDetails, dbName string) ([]string, error) {
	targetDB := dbName
	if targetDB == "" {
		targetDB = details.DBName // Use DB from connection if not overridden
	}

	if targetDB == "" {
		return nil, fmt.Errorf("no database specified or configured in the connection details")
	}

	// Query to list tables from the target database
	query := fmt.Sprintf("SHOW TABLES FROM `%s`;", targetDB)

	result, err := s.ExecuteSQL(ctx, details, query)
	if err != nil {
		// Check if error is due to database not existing? Could provide better feedback.
		// Example: Check for MySQL error code 1049 (Unknown database)
		// if mysqlErr, ok := err.(*mysql.MySQLError); ok && mysqlErr.Number == 1049 {
		//  return nil, fmt.Errorf("database '%s' not found", targetDB)
		// }
		return nil, fmt.Errorf("failed to list tables from database '%s': %w", targetDB, err)
	}

	tableRows, ok := result.([]map[string]any)
	if !ok {
		fmt.Printf("Debug: Unexpected result format when listing tables from %s: %T\n", targetDB, result)
		// Handle empty result or other formats if necessary
		if result == nil { // Check if result is nil (e.g., ExecuteSQL returned nil, nil)
			return []string{}, nil // Return empty list if no tables found or result was nil map/slice
		}
		// If result is not nil and not the expected type, it's an error
		return nil, fmt.Errorf("unexpected result format when listing tables from database '%s'", targetDB)
	}
	if len(tableRows) == 0 {
		return []string{}, nil // No tables found
	}


	var tableNames []string
	columnKey := ""
	if len(tableRows) > 0 {
		for k := range tableRows[0] {
			columnKey = k // Assume first column is table name
			break
		}
	}

	if columnKey == "" {
		// This should ideally not happen if tableRows is not empty, but handle defensively
		return nil, fmt.Errorf("could not determine table name column key from SHOW TABLES result in '%s'", targetDB)
	}

	for _, row := range tableRows {
		if name, ok := row[columnKey].(string); ok {
			tableNames = append(tableNames, name)
		}
	}

	return tableNames, nil
}


// GetTableData retrieves data (rows and columns) for a specific table with pagination and filtering.
func (s *DatabaseService) GetTableData(ctx context.Context, details ConnectionDetails, dbName string, tableName string, limit int, offset int, filterParams *map[string]any) (*TableDataResponse, error) {

	targetDB := dbName
	if targetDB == "" {
		targetDB = details.DBName
	}
	if targetDB == "" {
		return nil, fmt.Errorf("database name is required either explicitly or in connection details")
	}
	if tableName == "" {
		return nil, fmt.Errorf("table name is required")
	}

	// 1. Get column information first (needed for response structure)
	// Use INFORMATION_SCHEMA for more robust column type fetching if possible,
	// but DESCRIBE is simpler for now.
	descQuery := fmt.Sprintf("DESCRIBE `%s`.`%s`;", targetDB, tableName)
	descResult, err := s.ExecuteSQL(ctx, details, descQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to describe table '%s.%s': %w", targetDB, tableName, err)
	}

	descRows, ok := descResult.([]map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected result format when describing table '%s.%s'", targetDB, tableName)
	}

	var columns []TableColumn
	for _, row := range descRows {
		colName, _ := row["Field"].(string) // Field name from DESCRIBE
		colType, _ := row["Type"].(string)  // Type definition from DESCRIBE
		if colName != "" {
			columns = append(columns, TableColumn{Name: colName, Type: colType})
		}
	}
	if len(columns) == 0 {
		// This might happen if the table exists but DESCRIBE fails somehow, or table is empty struct?
		// Or maybe the table doesn't exist - DESCRIBE might error earlier in that case.
		log.Printf("Warning: No columns found for table %s.%s after DESCRIBE query.", targetDB, tableName)
		// Return empty response instead of error? Depends on desired behavior.
		// Let's return empty for now, assuming table might exist but is unusual.
		return &TableDataResponse{Columns: []TableColumn{}, Rows: []map[string]any{}}, nil
		// return nil, fmt.Errorf("no columns found for table '%s.%s'", targetDB, tableName)
	}

	// 2. Construct the SELECT query with filtering and pagination
	selectCols := "*" // Select all columns for simplicity, could be specific later
	// Be cautious with user-provided table/db names - escaping is done via backticks
	query := fmt.Sprintf("SELECT %s FROM `%s`.`%s`", selectCols, targetDB, tableName)

	// Basic WHERE clause construction (needs improvement for security and complexity)
	// WARNING: This simple concatenation is vulnerable to SQL injection if filterParams keys/values
	// come directly from user input without proper sanitization or prepared statements.
	// For internal use or trusted input, it might be okay, but needs care.
	// A proper implementation should use parameterized queries.
	whereClauses := []string{}
	if filterParams != nil {
		for key, value := range *filterParams {
			// TODO: Sanitize key? Escape value? Use placeholders?
			// Simple string equality for now
			whereClauses = append(whereClauses, fmt.Sprintf("`%s` = '%v'", key, value)) // VERY UNSAFE - demo only
		}
	}
	if len(whereClauses) > 0 {
		query += " WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Add LIMIT and OFFSET
	if limit <= 0 {
		limit = 100 // Default limit
	}
	query += fmt.Sprintf(" LIMIT %d", limit)
	if offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", offset)
	}
	query += ";"

	// 3. Execute the SELECT query
	dataResult, err := s.ExecuteSQL(ctx, details, query)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch data for table '%s.%s': %w", targetDB, tableName, err)
	}

	dataRows, ok := dataResult.([]map[string]any)
	if !ok {
		// Handle case where ExecuteSQL returns non-row data (e.g., execution summary), though unlikely for SELECT
		return nil, fmt.Errorf("unexpected result format when fetching data for '%s.%s'", targetDB, tableName)
	}

	// 4. Construct the response
	// TODO: Add total row count if needed (requires a separate COUNT(*) query)
	resp := &TableDataResponse{
		Columns: columns,
		Rows:    dataRows,
		// TotalRows: &total, // Add total count later
	}

	return resp, nil
}


