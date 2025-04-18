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

// ExecuteSQL runs a query and returns results or execution status.
func (s *DatabaseService) ExecuteSQL(ctx context.Context, details ConnectionDetails, query string) (any, error) {
	db, err := getDBConnection(details)
	if err != nil {
		return nil, fmt.Errorf("connection setup failed: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(query)
	if err == nil {
		defer rows.Close()
		columns, err := rows.Columns()
		if err != nil {
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
				return nil, fmt.Errorf("failed to scan row: %w", err)
			}

			rowMap := make(map[string]any)
			for i, col := range columns {
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

	result, execErr := db.Exec(query)
	if execErr != nil {
		// Return a combined error if both Query and Exec failed.
		return nil, fmt.Errorf("query failed: Query error (%v), Exec error (%v)", err, execErr)
	}

	rowsAffected, _ := result.RowsAffected()
	lastInsertId, _ := result.LastInsertId()

	return map[string]any{
		"rowsAffected": rowsAffected,
		"lastInsertId": lastInsertId,
	}, nil
}

// --- Database Schema/Data Inspection Methods ---

// TableColumn represents metadata for a table column.
type TableColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// TableDataResponse holds data and column definitions for a table query.
type TableDataResponse struct {
	Columns   []TableColumn    `json:"columns"`
	Rows      []map[string]any `json:"rows"`
	TotalRows *int64           `json:"totalRows,omitempty"`
}

// ListDatabases retrieves a list of database/schema names accessible by the connection.
func (s *DatabaseService) ListDatabases(ctx context.Context, details ConnectionDetails) ([]string, error) {
	query := "SHOW DATABASES;"
	result, err := s.ExecuteSQL(ctx, details, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list databases: %w", err)
	}

	dbRows, ok := result.([]map[string]any)
	if !ok {
		// Attempt to handle single-column results (less common but possible).
		log.Printf("Debug: Unexpected result format (%T) when listing databases, attempting single-column parse.", result)
		if rows, ok := result.([]any); ok {
			var dbNames []string
			for _, row := range rows {
				if dbMap, ok := row.(map[string]any); ok {
					for _, v := range dbMap {
						if name, ok := v.(string); ok {
							if name != "information_schema" && name != "performance_schema" && name != "mysql" && name != "sys" {
								dbNames = append(dbNames, name)
							}
							break
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
func (s *DatabaseService) ListTables(ctx context.Context, details ConnectionDetails, dbName string) ([]string, error) {
	targetDB := dbName
	if targetDB == "" {
		targetDB = details.DBName
	}
	if targetDB == "" {
		return nil, fmt.Errorf("no database specified or configured in the connection details")
	}

	query := fmt.Sprintf("SHOW TABLES FROM `%s`;", targetDB)
	result, err := s.ExecuteSQL(ctx, details, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list tables from database '%s': %w", targetDB, err)
	}

	tableRows, ok := result.([]map[string]any)
	if !ok {
		log.Printf("Debug: Unexpected result format (%T) when listing tables from %s.", targetDB, result)
		if result == nil {
			return []string{}, nil // Assume no tables found
		}
		return nil, fmt.Errorf("unexpected result format when listing tables from database '%s'", targetDB)
	}
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

	// 1. Get column information.
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
	dataResult, err := s.ExecuteSQL(ctx, details, dataQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch data for table '%s.%s': %w", targetDB, tableName, err)
	}
	dataRows, ok := dataResult.([]map[string]any)
	if !ok {
		log.Printf("Warning: Unexpected result type (%T) or nil returned for data query on %s.%s, assuming empty.", dataResult, targetDB, tableName)
		dataRows = []map[string]any{} // Ensure empty slice
	}

	// 5. Get Total Row Count (with the same filters).
	var totalRows *int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) as total FROM `%s`.`%s`%s;", targetDB, tableName, whereClause)
	countResult, countErr := s.ExecuteSQL(ctx, details, countQuery)
	if countErr == nil {
		if countRows, ok := countResult.([]map[string]any); ok && len(countRows) > 0 {
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
			log.Printf("Warning: COUNT(*) query for %s.%s returned unexpected format: %T", targetDB, tableName, countResult)
		}
	} else {
		log.Printf("Warning: Failed to get total row count for table %s.%s: %v", targetDB, tableName, countErr)
	}

	// 6. Construct the response.
	resp := &TableDataResponse{
		Columns:   columns,
		Rows:      dataRows,
		TotalRows: totalRows,
	}

	return resp, nil
}


