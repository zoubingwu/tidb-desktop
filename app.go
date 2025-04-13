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

// TableColumn represents metadata for a table column
type TableColumn struct {
	Name string `json:"name"`
	Type string `json:"type"` // e.g., "VARCHAR", "INT", "TIMESTAMP", etc. - simplified for now
}

// TableDataResponse holds data and column definitions
type TableDataResponse struct {
	Columns []TableColumn   `json:"columns"`
	Rows    []map[string]any `json:"rows"`
	TotalRows *int64         `json:"totalRows,omitempty"` // Optional: For pagination
}

// ListDatabases retrieves a list of database/schema names accessible by the connection.
func (a *App) ListDatabases() ([]string, error) {
	if a.ctx == nil { return nil, fmt.Errorf("app context not initialized") }
	if a.activeConnection == nil { return nil, fmt.Errorf("no active connection") }

	// Query varies slightly between DBs (e.g., MySQL vs PostgreSQL)
	// SHOW DATABASES; is common for MySQL/TiDB
	query := "SHOW DATABASES;"

	result, err := a.ExecuteSQL(query) // Use the existing ExecuteSQL
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
	// Or maybe the frontend should pre-select it if available.
	if a.activeConnection.DBName != "" {
		found := false
		for _, name := range dbNames {
			if name == a.activeConnection.DBName {
				found = true
				break
			}
		}
		if !found {
			// Add it if not found by SHOW DATABASES (might happen due to permissions)
			// Or maybe just rely on the frontend to pre-select it if available
			dbNames = append([]string{a.activeConnection.DBName}, dbNames...) // Prepend it
		}
	}

	return dbNames, nil
}

// ListTables retrieves a list of table names from the specified database.
// If dbName is empty, it uses the database specified in the active connection details.
func (a *App) ListTables(dbName string) ([]string, error) {
	if a.ctx == nil { return nil, fmt.Errorf("app context not initialized") }
	if a.activeConnection == nil { return nil, fmt.Errorf("no active connection") }

	targetDB := dbName
	if targetDB == "" {
		targetDB = a.activeConnection.DBName // Use DB from connection if provided
	}

	if targetDB == "" {
		// If still empty, we can't proceed.
		// Consider fetching default DB first? Or just error out.
		return nil, fmt.Errorf("no database selected or specified in connection details")
	}

	// Query to list tables from the target database
	query := fmt.Sprintf("SHOW TABLES FROM `%s`;", targetDB)

	result, err := a.ExecuteSQL(query)
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
		if result == nil || len(tableRows) == 0 {
			return []string{}, nil // Return empty list if no tables found
		}
		return nil, fmt.Errorf("unexpected result format when listing tables from database '%s'", targetDB)
	}

	var tableNames []string
	columnKey := ""
	if len(tableRows) > 0 {
		for k := range tableRows[0] {
			columnKey = k // Assume first column is table name
			break
		}
	}

	if columnKey == "" && len(tableRows) > 0 {
		return nil, fmt.Errorf("could not determine table name column key from SHOW TABLES result for database '%s'", targetDB)
	}

	for _, row := range tableRows {
		if name, ok := row[columnKey].(string); ok {
			tableNames = append(tableNames, name)
		}
	}

	return tableNames, nil
}

// GetTableData retrieves paginated data and column info for a specific table.
func (a *App) GetTableData(tableName string, limit int, offset int, filterParams *map[string]interface{}) (*TableDataResponse, error) {
	if a.ctx == nil { return nil, fmt.Errorf("app context not initialized") }
	if a.activeConnection == nil { return nil, fmt.Errorf("no active connection") }
	if tableName == "" { return nil, fmt.Errorf("table name cannot be empty") }

	// 1. Get Column Info (Example using information_schema - adjust if needed)
	// Ensure the DBName is handled correctly (either part of connection or table name)
	dbName := a.activeConnection.DBName
	if dbName == "" {
		// Attempt to infer from active connection or return error
		// This part needs refinement based on how you manage DB context
		return nil, fmt.Errorf("database name is required to fetch column info")
	}
	colQuery := fmt.Sprintf(
		"SELECT COLUMN_NAME, DATA_TYPE FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = '%s' AND TABLE_NAME = '%s' ORDER BY ORDINAL_POSITION;",
		dbName, tableName,
	)

	colResult, err := a.ExecuteSQL(colQuery)
	if err != nil { return nil, fmt.Errorf("failed to get column info for table '%s': %w", tableName, err) }

	colRows, ok := colResult.([]map[string]any)
	if !ok { return nil, fmt.Errorf("unexpected result format for column info") }

	var columns []TableColumn
	for _, row := range colRows {
		colName, nameOk := row["COLUMN_NAME"].(string)
		colType, typeOk := row["DATA_TYPE"].(string)
		if nameOk && typeOk {
			columns = append(columns, TableColumn{Name: colName, Type: colType})
		}
	}
	if len(columns) == 0 {
		// Check if table exists first might be better
		existsQuery := fmt.Sprintf("SELECT COUNT(*) as count FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = '%s' AND TABLE_NAME = '%s'", dbName, tableName)
		existsResult, err := a.ExecuteSQL(existsQuery)
		// Process existsResult... if count is 0, return specific error
		if err == nil {
			if rows, ok := existsResult.([]map[string]any); ok && len(rows) > 0 {
				if count, ok := rows[0]["count"].(int64); ok { // Type might be int64, int, etc. depending on DB driver
					if count == 0 {
						return nil, fmt.Errorf("table '%s' does not exist in schema '%s'", tableName, dbName)
					}
				}
				// If count extraction fails, fall through to the generic "no columns found" error
			}
		} // If error fetching count, also fall through

		// If table exists but no columns were found (or count check failed)
		return nil, fmt.Errorf("no columns found for table '%s' in schema '%s'", tableName, dbName)
	}

	// 2. Build query with filters
	var whereClause string

	// Process filters if they exist
	if filterParams != nil {
		filters, filtersExist := (*filterParams)["filters"]
		if filtersExist {
			// Cast filters to expected type (array of filter objects)
			if filtersArr, ok := filters.([]interface{}); ok && len(filtersArr) > 0 {
				conditions := []string{}

				for _, filter := range filtersArr {
					if filterMap, ok := filter.(map[string]interface{}); ok {
						columnId, hasColumnId := filterMap["columnId"].(string)
						operator, hasOperator := filterMap["operator"].(string)
						filterType, hasType := filterMap["type"].(string)
						values, hasValues := filterMap["values"].([]interface{})

						if hasColumnId && hasOperator && hasType && hasValues && len(values) > 0 {
							condition := ""

							// Map frontend filter operators to SQL operators
							switch filterType {
							case "text":
								if operator == "contains" {
									condition = fmt.Sprintf("`%s` LIKE '%%%s%%'", columnId, values[0])
								} else if operator == "does not contain" {
									condition = fmt.Sprintf("`%s` NOT LIKE '%%%s%%'", columnId, values[0])
								}
							case "number":
								switch operator {
								case "is":
									condition = fmt.Sprintf("`%s` = %v", columnId, values[0])
								case "is not":
									condition = fmt.Sprintf("`%s` != %v", columnId, values[0])
								case "is greater than":
									condition = fmt.Sprintf("`%s` > %v", columnId, values[0])
								case "is greater than or equal to":
									condition = fmt.Sprintf("`%s` >= %v", columnId, values[0])
								case "is less than":
									condition = fmt.Sprintf("`%s` < %v", columnId, values[0])
								case "is less than or equal to":
									condition = fmt.Sprintf("`%s` <= %v", columnId, values[0])
								case "is between":
									if len(values) >= 2 {
										condition = fmt.Sprintf("`%s` BETWEEN %v AND %v", columnId, values[0], values[1])
									}
								case "is not between":
									if len(values) >= 2 {
										condition = fmt.Sprintf("`%s` NOT BETWEEN %v AND %v", columnId, values[0], values[1])
									}
								}
							case "date":
								// Date handling requires converting the string date to proper format
								// This is simplified and may need adjustment based on your DB type
								switch operator {
								case "is":
									condition = fmt.Sprintf("DATE(`%s`) = DATE('%s')", columnId, values[0])
								case "is not":
									condition = fmt.Sprintf("DATE(`%s`) != DATE('%s')", columnId, values[0])
								case "is before":
									condition = fmt.Sprintf("DATE(`%s`) < DATE('%s')", columnId, values[0])
								case "is on or after":
									condition = fmt.Sprintf("DATE(`%s`) >= DATE('%s')", columnId, values[0])
								case "is after":
									condition = fmt.Sprintf("DATE(`%s`) > DATE('%s')", columnId, values[0])
								case "is on or before":
									condition = fmt.Sprintf("DATE(`%s`) <= DATE('%s')", columnId, values[0])
								case "is between":
									if len(values) >= 2 {
										condition = fmt.Sprintf("DATE(`%s`) BETWEEN DATE('%s') AND DATE('%s')",
											columnId, values[0], values[1])
									}
								case "is not between":
									if len(values) >= 2 {
										condition = fmt.Sprintf("DATE(`%s`) NOT BETWEEN DATE('%s') AND DATE('%s')",
											columnId, values[0], values[1])
									}
								}
							case "option":
								switch operator {
								case "is", "is any of":
									if strVal, ok := values[0].(string); ok {
										condition = fmt.Sprintf("`%s` = '%s'", columnId, strVal)
									}
								case "is not", "is none of":
									if strVal, ok := values[0].(string); ok {
										condition = fmt.Sprintf("`%s` != '%s'", columnId, strVal)
									}
								}
							case "multiOption":
								if multiValues, ok := values[0].([]interface{}); ok && len(multiValues) > 0 {
									valueStrings := make([]string, len(multiValues))
									for i, v := range multiValues {
										if strVal, ok := v.(string); ok {
											valueStrings[i] = fmt.Sprintf("'%s'", strVal)
										}
									}

									if len(valueStrings) > 0 {
										valuesStr := strings.Join(valueStrings, ", ")
										switch operator {
										case "include", "include any of":
											condition = fmt.Sprintf("`%s` IN (%s)", columnId, valuesStr)
										case "exclude", "exclude if any of":
											condition = fmt.Sprintf("`%s` NOT IN (%s)", columnId, valuesStr)
										// For other operators like "include all of", "exclude if all",
										// more complex logic would be needed
										}
									}
								}
							}

							if condition != "" {
								conditions = append(conditions, condition)
							}
						}
					}
				}

				if len(conditions) > 0 {
					whereClause = " WHERE " + strings.Join(conditions, " AND ")
				}
			}
		}
	}

	// 2. Get Table Data with filters applied
	dataQuery := fmt.Sprintf("SELECT * FROM `%s`%s LIMIT %d OFFSET %d;",
		tableName, whereClause, limit, offset)

	dataResult, err := a.ExecuteSQL(dataQuery)
	if err != nil { return nil, fmt.Errorf("failed to get data for table '%s': %w", tableName, err) }

	dataRows, ok := dataResult.([]map[string]any)
	if !ok {
		// Handle cases where the result isn't rows (e.g., row count from DELETE/UPDATE)
		// For a SELECT *, this shouldn't happen unless the table is empty or error occurred.
		// Check if dataResult is nil maybe?
		fmt.Printf("Debug: Unexpected dataResult format for SELECT: %T\n", dataResult)
		// If the table might legitimately be empty, return empty rows instead of erroring
		if dataResult == nil || len(dataRows) == 0 { // Assuming empty table returns nil or empty slice
			dataRows = []map[string]any{} // Ensure it's an empty slice
		} else {
			return nil, fmt.Errorf("unexpected result format for table data")
		}
	}

	// 3. Get Total Row Count for Pagination
	// Apply the same filters to the count query
	countQuery := fmt.Sprintf("SELECT COUNT(*) as total FROM `%s`%s;", tableName, whereClause)
	countResult, countErr := a.ExecuteSQL(countQuery) // Execute count query separately
	var totalRows *int64 = nil // Use pointer to distinguish 0 from not fetched

	if countErr == nil {
		if countRows, ok := countResult.([]map[string]any); ok && len(countRows) > 0 {
			// Type assertion depends on DB driver (e.g., int64, float64)
			// Use a robust way to convert to int64 if possible
			if totalValRaw, ok := countRows[0]["total"]; ok {
				 // Example conversion (adjust based on actual type)
				 switch v := totalValRaw.(type) {
				 case int64:
					totalRows = &v
				 case int:
					temp := int64(v)
					totalRows = &temp
				 case float64: // Some drivers might return float
					 temp := int64(v)
					 totalRows = &temp
				 // Add other potential types if needed
				 default:
					 fmt.Printf("Warning: Unexpected type for COUNT(*): %T\n", v)
				 }
			}
		}
	} else {
		 fmt.Printf("Warning: Failed to get total row count for table '%s': %v\n", tableName, countErr)
	}

	return &TableDataResponse{
		Columns: columns,
		Rows:    dataRows,
		TotalRows: totalRows, // Add totalRows to the response
	}, nil
}
