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

// ConnectionDetails defines the structure for DB connection info.
type ConnectionDetails struct {
	Name     string `json:"name,omitempty"`
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	DBName   string `json:"dbName"`
	UseTLS   bool   `json:"useTLS"`
	LastUsed string `json:"lastUsed,omitempty"`
}

// SQLResult defines a standard structure for SQL execution results.
type SQLResult struct {
	Columns      []string         `json:"columns,omitempty"`      // Ordered list of column names for SELECT
	Rows         []map[string]any `json:"rows,omitempty"`         // Used for SELECT queries
	RowsAffected *int64           `json:"rowsAffected,omitempty"` // Used for INSERT/UPDATE/DELETE
	LastInsertId *int64           `json:"lastInsertId,omitempty"` // Used for INSERT
	Message      string           `json:"message,omitempty"`      // Optional message (e.g., for commands like USE)
}

// DatabaseService handles DB operations.
type DatabaseService struct{}

// NewDatabaseService creates a new DatabaseService.
func NewDatabaseService() *DatabaseService {
	return &DatabaseService{}
}

// buildDSN creates the Data Source Name string for the connection.
func buildDSN(details ConnectionDetails) (string, bool) {
	port := details.Port
	if port == "" {
		port = "4000" // Default TiDB port
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true",
		details.User, details.Password, details.Host, port, details.DBName)

	// Determine if TLS should be used based on flag or host.
	useTLS := details.UseTLS || strings.Contains(details.Host, ".tidbcloud.com")

	if useTLS {
		dsn += "&tls=tidb"
	}

	return dsn, useTLS
}

// getDBConnection handles creating the DB connection, including TLS setup.
func getDBConnection(details ConnectionDetails) (*sql.DB, error) {
	dsn, useTLS := buildDSN(details)

	if useTLS {
		// Register TLS config, allowing re-registration for different hosts.
		err := mysql.RegisterTLSConfig("tidb", &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: details.Host,
		})
		if err != nil && !strings.Contains(err.Error(), "already registered") {
			return nil, fmt.Errorf("failed to register TLS config: %w", err)
		}
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	return db, nil
}

// TestConnection attempts to ping the database.
func (s *DatabaseService) TestConnection(ctx context.Context, details ConnectionDetails) (bool, error) {
	db, err := getDBConnection(details)
	if err != nil {
		return false, fmt.Errorf("connection setup failed: %w", err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		return false, fmt.Errorf("failed to ping database: %w", err)
	}
	return true, nil
}

// ExecuteSQL runs a query and returns results or execution status in a structured format.
func (s *DatabaseService) ExecuteSQL(ctx context.Context, details ConnectionDetails, query string) (*SQLResult, error) {
	db, err := getDBConnection(details)
	if err != nil {
		return nil, fmt.Errorf("connection setup failed: %w", err)
	}
	defer db.Close()

	// Attempt to execute as a query first (SELECT, SHOW, DESCRIBE, etc.)
	rows, queryErr := db.QueryContext(ctx, query)
	if queryErr == nil {
		defer rows.Close()
		columns, err := rows.Columns()
		if err != nil {
			// This specific error check is useful for queries like `USE database;` which succeed but return no columns/rows.
			if strings.Contains(err.Error(), "no columns in result set") {
				return &SQLResult{Message: fmt.Sprintf("Command executed successfully: %s", query)}, nil
			}
			return nil, fmt.Errorf("failed to get columns: %w", err)
		}

		var results []map[string]any
		for rows.Next() {
			values := make([]any, len(columns))
			scanArgs := make([]any, len(columns))
			for i := range values {
				scanArgs[i] = &values[i]
			}

			err = rows.Scan(scanArgs...)
			if err != nil {
				// Log the specific row scanning error
				log.Printf("Error scanning row for query [%s]: %v", query, err)
				// Depending on requirements, you might want to return partial results or a specific error
				return nil, fmt.Errorf("failed to scan row: %w", err)
			}

			rowMap := make(map[string]any)
			for i, col := range columns {
				val := values[i]
				// Convert []byte to string for better JSON representation
				if b, ok := val.([]byte); ok {
					rowMap[col] = string(b)
				} else {
					rowMap[col] = val // Keep other types as they are (int, float, null, etc.)
				}
			}
			results = append(results, rowMap)
		}

		// Check for errors encountered during iteration
		if err = rows.Err(); err != nil {
			return nil, fmt.Errorf("error iterating rows: %w", err)
		}

		// Success, return rows and columns
		return &SQLResult{Columns: columns, Rows: results}, nil
	}

	// If db.Query failed, try db.Exec (INSERT, UPDATE, DELETE, etc.)
	result, execErr := db.ExecContext(ctx, query)
	if execErr != nil {
		// If both Query and Exec failed, return a combined or more specific error.
		// The initial queryErr might be more indicative (e.g., syntax error)
		// Or execErr might be more relevant (e.g., constraint violation)
		return nil, fmt.Errorf("SQL execution failed. Query attempt error: [%v]. Exec attempt error: [%v]", queryErr, execErr)
	}

	// Exec succeeded, return affected rows and last insert ID
	rowsAffected, errRA := result.RowsAffected()
	if errRA != nil {
		log.Printf("Warning: could not get RowsAffected for query [%s]: %v", query, errRA)
		// Don't fail the whole operation, just omit RowsAffected
	}

	lastInsertId, errLI := result.LastInsertId()
	if errLI != nil {
		// This error is common if the statement wasn't an INSERT or the table has no auto-increment key
		// Log it but don't treat as fatal error for the operation itself
		log.Printf("Notice: could not get LastInsertId for query [%s] (may not be applicable): %v", query, errLI)
	}

	// Use pointers for optional fields
	var rowsAffectedPtr *int64
	if errRA == nil {
		rowsAffectedPtr = &rowsAffected
	}
	var lastInsertIdPtr *int64
	// Only include LastInsertId if it's likely valid (>= 0) and there was no error getting it
	if errLI == nil && lastInsertId >= 0 {
		lastInsertIdPtr = &lastInsertId
	}

	return &SQLResult{
		RowsAffected: rowsAffectedPtr,
		LastInsertId: lastInsertIdPtr,
		Message:      "Command executed successfully.", // Provide a generic success message for Exec results
	}, nil
}

// --- Database Schema/Data Inspection Methods ---

// TableColumn represents metadata for a table column.
type TableColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// ColumnSchema holds detailed metadata for a table column based on information_schema.
type ColumnSchema struct {
	ColumnName       string         `json:"column_name"`
	ColumnType       string         `json:"column_type"` // Full type definition, e.g., varchar(255)
	CharacterSetName sql.NullString `json:"character_set_name"`
	CollationName    sql.NullString `json:"collation_name"`
	IsNullable       string         `json:"is_nullable"` // "YES" or "NO"
	ColumnDefault    sql.NullString `json:"column_default"`
	Extra            string         `json:"extra"` // e.g., auto_increment, on update CURRENT_TIMESTAMP
	ColumnComment    string         `json:"column_comment"`
}

// TableSchema represents the detailed structure of a table.
type TableSchema struct {
	Name    string         `json:"name"`
	Columns []ColumnSchema `json:"columns"`
}

// TableDataResponse holds data and column definitions for a table query.
type TableDataResponse struct {
	Columns   []TableColumn    `json:"columns"`
	Rows      []map[string]any `json:"rows"`
	TotalRows *int64           `json:"totalRows,omitempty"`
}

// ListDatabases retrieves a list of database/schema names accessible by the connection.
// Note: This function specifically expects rows, so we handle the SQLResult directly.
func (s *DatabaseService) ListDatabases(ctx context.Context, details ConnectionDetails) ([]string, error) {
	query := "SHOW DATABASES;"
	sqlResult, err := s.ExecuteSQL(ctx, details, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list databases: %w", err)
	}

	if sqlResult == nil || sqlResult.Rows == nil {
		// This shouldn't happen for SHOW DATABASES unless there's a connection issue masked earlier
		log.Printf("Debug: Unexpected nil result or nil rows when listing databases.")
		return nil, fmt.Errorf("unexpected result format when listing databases (expected rows)")
	}

	dbRows := sqlResult.Rows
	var dbNames []string
	columnKey := ""
	if len(dbRows) > 0 {
		for k := range dbRows[0] {
			columnKey = k
			break
		}
	}

	if columnKey == "" && len(dbRows) > 0 {
		return nil, fmt.Errorf("could not determine database name column key from SHOW DATABASES result")
	}

	for _, row := range dbRows {
		if name, ok := row[columnKey].(string); ok {
			if name != "information_schema" && name != "performance_schema" && name != "mysql" && name != "sys" {
				dbNames = append(dbNames, name)
			}
		}
	}

	return dbNames, nil
}

// ListTables retrieves a list of table names from the specified database.
// Note: This function specifically expects rows, so we handle the SQLResult directly.
func (s *DatabaseService) ListTables(ctx context.Context, details ConnectionDetails, dbName string) ([]string, error) {
	targetDB := dbName
	if targetDB == "" {
		targetDB = details.DBName
	}
	if targetDB == "" {
		return nil, fmt.Errorf("no database specified or configured in the connection details")
	}

	query := fmt.Sprintf("SHOW TABLES FROM `%s`;", targetDB)
	sqlResult, err := s.ExecuteSQL(ctx, details, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list tables from database '%s': %w", targetDB, err)
	}

	if sqlResult == nil || sqlResult.Rows == nil {
		// SHOW TABLES returns empty result set (not error) if no tables, handle this.
		log.Printf("Debug: SHOW TABLES for '%s' returned nil result or nil rows. Assuming no tables.", targetDB)
		return []string{}, nil // No tables found is not an error
	}

	tableRows := sqlResult.Rows
	if len(tableRows) == 0 {
		return []string{}, nil // No tables found
	}

	var tableNames []string
	columnKey := ""
	if len(tableRows) > 0 {
		for k := range tableRows[0] {
			columnKey = k
			break
		}
	}

	if columnKey == "" {
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
// Note: This function uses ExecuteSQL internally, needs careful handling of results.
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

	// 1. Get column information using DESCRIBE.
	descQuery := fmt.Sprintf("DESCRIBE `%s`.`%s`;", targetDB, tableName)
	descSQLResult, err := s.ExecuteSQL(ctx, details, descQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to describe table '%s.%s': %w", targetDB, tableName, err)
	}
	if descSQLResult == nil || descSQLResult.Rows == nil {
		// Check if table exists before failing completely
		exists, existsErr := s.checkTableExists(ctx, details, targetDB, tableName)
		if existsErr != nil {
			log.Printf("Warning: Failed to verify existence of table %s.%s after DESCRIBE failed: %v", targetDB, tableName, existsErr)
		}
		if !exists {
			return nil, fmt.Errorf("table '%s.%s' not found", targetDB, tableName)
		}
		return nil, fmt.Errorf("unexpected result format when describing table '%s.%s' (expected rows)", targetDB, tableName)
	}

	descRows := descSQLResult.Rows // Extract rows from SQLResult
	var columns []TableColumn
	for _, row := range descRows {
		colName, _ := row["Field"].(string)
		colType, _ := row["Type"].(string)
		if colName != "" {
			columns = append(columns, TableColumn{Name: colName, Type: colType})
		}
	}
	if len(columns) == 0 {
		log.Printf("Warning: No columns found for table %s.%s after DESCRIBE query.", targetDB, tableName)
		return &TableDataResponse{Columns: []TableColumn{}, Rows: []map[string]any{}}, nil // Return empty response
	}

	// 2. Build the WHERE clause from filterParams.
	whereClause := ""
	if filterParams != nil {
		filters, filtersExist := (*filterParams)["filters"]
		if filtersExist {
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
							// WARNING: Parameterize these values for security!
							// Simplified filter mapping - Needs proper escaping/parameterization.
							switch filterType {
							case "text":
								if operator == "contains" {
									condition = fmt.Sprintf("`%s` LIKE '%%%v%%'", columnId, values[0])
								} else if operator == "does not contain" {
									condition = fmt.Sprintf("`%s` NOT LIKE '%%%v%%'", columnId, values[0])
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
								switch operator {
								case "is":
									condition = fmt.Sprintf("DATE(`%s`) = DATE('%v')", columnId, values[0])
								case "is not":
									condition = fmt.Sprintf("DATE(`%s`) != DATE('%v')", columnId, values[0])
								case "is between":
									if len(values) >= 2 {
										condition = fmt.Sprintf("DATE(`%s`) BETWEEN DATE('%v') AND DATE('%v')", columnId, values[0], values[1])
									}
								case "is not between":
									if len(values) >= 2 {
										condition = fmt.Sprintf("DATE(`%s`) NOT BETWEEN DATE('%v') AND DATE('%v')", columnId, values[0], values[1])
									}
								}
							case "option", "multiOption":
								var valueStrings []string
								if multiValues, ok := values[0].([]interface{}); ok {
									for _, v := range multiValues {
										if strVal, ok := v.(string); ok {
											valueStrings = append(valueStrings, fmt.Sprintf("'%s'", strVal))
										}
									}
								} else if strVal, ok := values[0].(string); ok {
									valueStrings = append(valueStrings, fmt.Sprintf("'%s'", strVal))
								}

								if len(valueStrings) > 0 {
									valuesStr := strings.Join(valueStrings, ", ")
									switch operator {
									case "is", "is any of", "include", "include any of":
										condition = fmt.Sprintf("`%s` IN (%s)", columnId, valuesStr)
									case "is not", "is none of", "exclude", "exclude if any of":
										condition = fmt.Sprintf("`%s` NOT IN (%s)", columnId, valuesStr)
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

	// 3. Construct the SELECT query for data rows.
	selectCols := "*"
	dataQuery := fmt.Sprintf("SELECT %s FROM `%s`.`%s`%s", selectCols, targetDB, tableName, whereClause)

	if limit <= 0 {
		limit = 100 // Default limit
	}
	dataQuery += fmt.Sprintf(" LIMIT %d", limit)
	if offset > 0 {
		dataQuery += fmt.Sprintf(" OFFSET %d", offset)
	}
	dataQuery += ";"

	// 4. Execute the data query.
	dataSQLResult, err := s.ExecuteSQL(ctx, details, dataQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch data for table '%s.%s': %w", targetDB, tableName, err)
	}

	var dataRows []map[string]any
	if dataSQLResult != nil && dataSQLResult.Rows != nil {
		dataRows = dataSQLResult.Rows // Use rows if available
	} else {
		log.Printf("Warning: Data query for %s.%s returned no rows or unexpected result type (%T), assuming empty.", targetDB, tableName, dataSQLResult)
		dataRows = []map[string]any{} // Ensure empty slice if no rows returned
	}

	// 5. Get Total Row Count (with the same filters).
	var totalRows *int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) as total FROM `%s`.`%s`%s;", targetDB, tableName, whereClause)
	countSQLResult, countErr := s.ExecuteSQL(ctx, details, countQuery) // Use ExecuteSQL here too
	if countErr == nil && countSQLResult != nil && countSQLResult.Rows != nil && len(countSQLResult.Rows) > 0 {
		countRows := countSQLResult.Rows // Extract rows
		if totalValRaw, ok := countRows[0]["total"]; ok {
			switch v := totalValRaw.(type) {
			case int64:
				totalRows = &v
			case int:
				temp := int64(v)
				totalRows = &temp
			case float64:
				temp := int64(v)
				totalRows = &temp
			default:
				log.Printf("Warning: Unexpected type for COUNT(*) result: %T\n", v)
			}
		} else {
			log.Printf("Warning: COUNT(*) query for %s.%s returned rows but no 'total' column.", targetDB, tableName)
		}
	} else {
		log.Printf("Warning: Failed to get total row count for table %s.%s: %v (Result: %+v)", targetDB, tableName, countErr, countSQLResult)
	}

	// 6. Construct the response.
	resp := &TableDataResponse{
		Columns:   columns,
		Rows:      dataRows,
		TotalRows: totalRows,
	}

	return resp, nil
}

// GetTableSchema retrieves the detailed schema/structure for a specific table using information_schema.
// This needs direct *sql.DB access for Scan handling with nulls, so it doesn't use ExecuteSQL.
func (s *DatabaseService) GetTableSchema(ctx context.Context, details ConnectionDetails, dbName string, tableName string) (*TableSchema, error) {
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

	// Query information_schema for detailed column information
	query := `
		SELECT
			COLUMN_NAME,
			COLUMN_TYPE,
			CHARACTER_SET_NAME,
			COLLATION_NAME,
			IS_NULLABLE,
			COLUMN_DEFAULT,
			EXTRA,
			COLUMN_COMMENT
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		ORDER BY ORDINAL_POSITION;`

	// Need to use the raw *sql.DB connection here to handle potential nulls correctly with Scan
	db, err := getDBConnection(details)
	if err != nil {
		return nil, fmt.Errorf("connection setup failed for GetTableSchema: %w", err)
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, query, targetDB, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to query information_schema.COLUMNS for '%s.%s': %w", targetDB, tableName, err)
	}
	defer rows.Close()

	schema := &TableSchema{
		Name:    tableName,
		Columns: make([]ColumnSchema, 0),
	}

	for rows.Next() {
		var col ColumnSchema
		err := rows.Scan(
			&col.ColumnName,
			&col.ColumnType,
			&col.CharacterSetName,
			&col.CollationName,
			&col.IsNullable,
			&col.ColumnDefault,
			&col.Extra,
			&col.ColumnComment,
		)
		if err != nil {
			log.Printf("Error scanning information_schema row for %s.%s: %v", targetDB, tableName, err)
			// Decide if we should continue or return error. Continuing might give partial results.
			continue // Skip this column
		}
		schema.Columns = append(schema.Columns, col)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating information_schema.COLUMNS results for '%s.%s': %w", targetDB, tableName, err)
	}

	if len(schema.Columns) == 0 {
		// Check if the table actually exists, as Query might succeed but return no rows.
		existsQuery := "SELECT 1 FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? LIMIT 1;"
		var exists int
		existsErr := db.QueryRowContext(ctx, existsQuery, targetDB, tableName).Scan(&exists)
		if existsErr != nil && existsErr != sql.ErrNoRows {
			log.Printf("Warning: Could not verify existence of table %s.%s: %v", targetDB, tableName, existsErr)
		} else if existsErr == sql.ErrNoRows {
			return nil, fmt.Errorf("table '%s.%s' not found", targetDB, tableName)
		}
		// Table exists but has no columns? Or query failed silently?
		log.Printf("Warning: No columns found for table '%s.%s', returning empty schema.", targetDB, tableName)
	}

	return schema, nil
}

// Helper function to check if a table exists (used in GetTableData error handling)
func (s *DatabaseService) checkTableExists(ctx context.Context, details ConnectionDetails, dbName string, tableName string) (bool, error) {
	db, err := getDBConnection(details)
	if err != nil {
		return false, fmt.Errorf("connection setup failed for table existence check: %w", err)
	}
	defer db.Close()

	query := "SELECT 1 FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? LIMIT 1;"
	var exists int
	err = db.QueryRowContext(ctx, query, dbName, tableName).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil // Table does not exist
	}
	if err != nil {
		return false, fmt.Errorf("error checking table existence for '%s.%s': %w", dbName, tableName, err)
	}
	return exists == 1, nil
}


