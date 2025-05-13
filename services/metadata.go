package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Column represents a database column's metadata
type Column struct {
	Name          string `json:"name"`
	DataType      string `json:"dataType"`
	IsNullable    bool   `json:"isNullable"`
	DefaultValue  any    `json:"defaultValue,omitempty"`
	IsPrimaryKey  bool   `json:"isPrimaryKey"`
	AutoIncrement bool   `json:"autoIncrement"`
	DBComment     string `json:"dbComment,omitempty"`     // Comment from database
	AIDescription string `json:"aiDescription,omitempty"` // Description from AI
}

// ForeignKey represents a foreign key relationship
type ForeignKey struct {
	Name            string   `json:"name"`
	ColumnNames     []string `json:"columnNames"`
	RefTableName    string   `json:"refTableName"`
	RefColumnNames  []string `json:"refColumnNames"`
}

// Index represents a database index
type Index struct {
	Name        string   `json:"name"`
	ColumnNames []string `json:"columnNames"`
	IsUnique    bool     `json:"isUnique"`
}

// Table represents a database table's metadata
type Table struct {
	Name          string       `json:"name"`
	Columns       []Column     `json:"columns"`
	ForeignKeys   []ForeignKey `json:"foreignKeys,omitempty"`
	Indexes       []Index      `json:"indexes,omitempty"`
	DBComment     string       `json:"dbComment,omitempty"`     // Comment from database
	AIDescription string       `json:"aiDescription,omitempty"` // Description from AI
}

// DatabaseMetadata represents the metadata for a single database
type DatabaseMetadata struct {
	Name          string            `json:"name"`
	Tables        []Table           `json:"tables"`
	Graph         map[string][]Edge `json:"graph,omitempty"` // Adjacency list representation
	DBComment     string            `json:"dbComment,omitempty"`     // Comment from database
	AIDescription string            `json:"aiDescription,omitempty"` // Description from AI
}

// ConnectionMetadata represents the complete metadata for a connection
type ConnectionMetadata struct {
	ConnectionName string                      `json:"connectionName"`
	LastExtracted  time.Time                   `json:"lastExtracted"`
	Databases     map[string]DatabaseMetadata  `json:"databases"`
}

// Edge represents a relationship between tables in the graph
type Edge struct {
	ToTable    string `json:"toTable"`
	FromColumn string `json:"fromColumn"`
	ToColumn   string `json:"toColumn"`
}

// MetadataService handles database metadata operations
type MetadataService struct {
	configService *ConfigService
	dbService     *DatabaseService
	metadataDir   string
}

// StaleMetadataThreshold is the duration after which metadata is considered stale
const StaleMetadataThreshold = 24 * time.Hour

// NewMetadataService creates a new metadata service
func NewMetadataService(configService *ConfigService, dbService *DatabaseService) (*MetadataService, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	metadataDir := filepath.Join(homeDir, ConfigDirName, MetadataDirName)
	if err := os.MkdirAll(metadataDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create metadata directory: %w", err)
	}

	return &MetadataService{
		configService: configService,
		dbService:     dbService,
		metadataDir:   metadataDir,
	}, nil
}

// getMetadataFilePath returns the path to the metadata file for a given connection
func (s *MetadataService) getMetadataFilePath(connectionName string) string {
	fileName := fmt.Sprintf("%s_metadata.json", connectionName)
	return filepath.Join(s.metadataDir, fileName)
}

// isSystemDatabase returns true if the given database name is a system database
func isSystemDatabase(dbName string) bool {
	systemDBs := map[string]bool{
		"information_schema":  true,
		"performance_schema": true,
		"metrics_schema":    true,  // TiDB specific
		"mysql":              true,
		"sys":               true,
	}

	// Convert to lowercase for case-insensitive comparison
	return systemDBs[strings.ToLower(dbName)]
}

// ExtractMetadata extracts metadata from the database and stores it
func (s *MetadataService) ExtractMetadata(ctx context.Context, connectionName string) (*ConnectionMetadata, error) {
	Info("Starting metadata extraction for connection: %s", connectionName)
	// Get connection details
	connDetails, exists, err := s.configService.GetConnection(connectionName)
	if err != nil {
		Error("failed to get connection details: %v", err)
		return nil, fmt.Errorf("failed to get connection details: %w", err)
	}
	if !exists {
		Error("connection %s not found", connectionName)
		return nil, fmt.Errorf("connection %s not found", connectionName)
	}

	// Create new connection metadata
	connMetadata := &ConnectionMetadata{
		ConnectionName: connectionName,
		LastExtracted: time.Now(),
		Databases:     make(map[string]DatabaseMetadata),
	}

	// Get all databases
	databases, err := s.dbService.ListDatabases(ctx, connDetails)
	if err != nil {
		Error("failed to list databases: %v", err)
		return nil, fmt.Errorf("failed to list databases: %w", err)
	}

	// Filter out system databases
	userDatabases := make([]string, 0, len(databases))
	for _, dbName := range databases {
		if !isSystemDatabase(dbName) {
			userDatabases = append(userDatabases, dbName)
		}
	}
	Info("Found %d user databases for connection %s", len(userDatabases), connectionName)

	// Extract metadata for each database concurrently
	type dbResult struct {
		dbName string
		metadata DatabaseMetadata
		err    error
	}
	dbResults := make(chan dbResult, len(userDatabases))

	for _, dbName := range userDatabases {
		go func(dbName string) {
			Info("Extracting metadata for database: %s", dbName)
			// Use the specified database
			connDetailsCopy := connDetails
			connDetailsCopy.DBName = dbName

			// Get all tables
			tables, err := s.dbService.ListTables(ctx, connDetailsCopy, dbName)
			if err != nil {
				dbResults <- dbResult{dbName: dbName, err: fmt.Errorf("failed to list tables for database %s: %w", dbName, err)}
				return
			}

			if len(tables) == 0 {
				Info("No tables found in database: %s, skipping", dbName)
				dbResults <- dbResult{dbName: dbName, metadata: DatabaseMetadata{
					Name:   dbName,
					Tables: []Table{},
					Graph:  make(map[string][]Edge),
				}}
				return
			}

			dbMetadata := DatabaseMetadata{
				Name:   dbName,
				Tables: make([]Table, 0, len(tables)),
				Graph:  make(map[string][]Edge),
			}

			// Get database comment if available
			dbCommentQuery := fmt.Sprintf(`
				SELECT SCHEMA_COMMENT
				FROM information_schema.SCHEMATA
				WHERE SCHEMA_NAME = '%s';`, dbName)

			if result, err := s.dbService.ExecuteSQL(ctx, connDetailsCopy, dbCommentQuery); err == nil && len(result.Rows) > 0 {
				if comment, ok := result.Rows[0]["SCHEMA_COMMENT"].(string); ok && comment != "" {
					dbMetadata.DBComment = comment
				}
			}

			// Extract metadata for each table concurrently
			type tableResult struct {
				table Table
				err   error
			}
			results := make(chan tableResult, len(tables))

			for _, tableName := range tables {
				go func(tableName string) {
					Info("Extracting schema for table: %s.%s", dbName, tableName)
					table := Table{
						Name:        tableName,
						Columns:     make([]Column, 0),
						ForeignKeys: make([]ForeignKey, 0),
						Indexes:     make([]Index, 0),
					}

					// Get table schema
					tableSchema, err := s.dbService.GetTableSchema(ctx, connDetailsCopy, dbName, tableName)
					if err != nil {
						results <- tableResult{err: fmt.Errorf("failed to get schema for table %s in database %s: %w", tableName, dbName, err)}
						return
					}

					// Get table comment
					tableCommentQuery := fmt.Sprintf(`
						SELECT TABLE_COMMENT
						FROM information_schema.TABLES
						WHERE TABLE_SCHEMA = '%s'
						AND TABLE_NAME = '%s';`, dbName, tableName)

					if result, err := s.dbService.ExecuteSQL(ctx, connDetailsCopy, tableCommentQuery); err == nil && len(result.Rows) > 0 {
						if comment, ok := result.Rows[0]["TABLE_COMMENT"].(string); ok && comment != "" {
							table.DBComment = comment
						}
					}

					// Get column comments
					columnCommentsQuery := fmt.Sprintf(`
						SELECT COLUMN_NAME, COLUMN_COMMENT
						FROM information_schema.COLUMNS
						WHERE TABLE_SCHEMA = '%s'
						AND TABLE_NAME = '%s';`, dbName, tableName)

					columnComments := make(map[string]string)
					if result, err := s.dbService.ExecuteSQL(ctx, connDetailsCopy, columnCommentsQuery); err == nil {
						for _, row := range result.Rows {
							if colName, ok := row["COLUMN_NAME"].(string); ok {
								if comment, ok := row["COLUMN_COMMENT"].(string); ok && comment != "" {
									columnComments[colName] = comment
								}
							}
						}
					}

					// Extract column information
					for _, col := range tableSchema.Columns {
						column := Column{
							Name:          col.ColumnName,
							DataType:      col.ColumnType,
							IsNullable:    col.IsNullable == "YES",
							AutoIncrement: col.Extra == "auto_increment",
							DBComment:     columnComments[col.ColumnName],
						}
						if col.ColumnDefault.Valid {
							column.DefaultValue = col.ColumnDefault.String
						}
						table.Columns = append(table.Columns, column)
					}

					// Extract foreign keys
					fkQuery := fmt.Sprintf(`
						SELECT
							CONSTRAINT_NAME,
							COLUMN_NAME,
							REFERENCED_TABLE_NAME,
							REFERENCED_COLUMN_NAME
						FROM information_schema.KEY_COLUMN_USAGE
						WHERE TABLE_SCHEMA = '%s'
						AND TABLE_NAME = '%s'
						AND REFERENCED_TABLE_NAME IS NOT NULL;`, dbName, tableName)

					fkResult, err := s.dbService.ExecuteSQL(ctx, connDetailsCopy, fkQuery)
					if err == nil && fkResult != nil && len(fkResult.Rows) > 0 {
						fkMap := make(map[string]*ForeignKey)

						for _, row := range fkResult.Rows {
							constraintName := row["CONSTRAINT_NAME"].(string)
							columnName := row["COLUMN_NAME"].(string)
							refTableName := row["REFERENCED_TABLE_NAME"].(string)
							refColumnName := row["REFERENCED_COLUMN_NAME"].(string)

							if fk, exists := fkMap[constraintName]; exists {
								fk.ColumnNames = append(fk.ColumnNames, columnName)
								fk.RefColumnNames = append(fk.RefColumnNames, refColumnName)
							} else {
								fkMap[constraintName] = &ForeignKey{
									Name:           constraintName,
									ColumnNames:    []string{columnName},
									RefTableName:   refTableName,
									RefColumnNames: []string{refColumnName},
								}
							}
						}

						// Convert map to slice
						for _, fk := range fkMap {
							table.ForeignKeys = append(table.ForeignKeys, *fk)
						}
					}

					// Extract indexes
					indexQuery := fmt.Sprintf(`
						SELECT
							INDEX_NAME,
							COLUMN_NAME,
							NON_UNIQUE
						FROM information_schema.STATISTICS
						WHERE TABLE_SCHEMA = '%s'
						AND TABLE_NAME = '%s'
						ORDER BY INDEX_NAME, SEQ_IN_INDEX;`, dbName, tableName)

					indexResult, err := s.dbService.ExecuteSQL(ctx, connDetailsCopy, indexQuery)
					if err == nil && indexResult != nil && len(indexResult.Rows) > 0 {
						indexMap := make(map[string]*Index)

						for _, row := range indexResult.Rows {
							indexName := row["INDEX_NAME"].(string)
							columnName := row["COLUMN_NAME"].(string)
							nonUnique := row["NON_UNIQUE"].(string) == "1"

							if idx, exists := indexMap[indexName]; exists {
								idx.ColumnNames = append(idx.ColumnNames, columnName)
							} else {
								indexMap[indexName] = &Index{
									Name:        indexName,
									ColumnNames: []string{columnName},
									IsUnique:    !nonUnique,
								}
							}
						}

						// Convert map to slice
						for _, idx := range indexMap {
							table.Indexes = append(table.Indexes, *idx)
						}
					}

					results <- tableResult{table: table}
				}(tableName)
			}

			// Collect table results into a temporary map to handle out-of-order arrivals
			// while preserving the ability to reconstruct the original table order.
			processedTablesMap := make(map[string]Table, len(tables))
			for i := 0; i < len(tables); i++ { // Loop exactly len(tables) times to get all results
				result := <-results
				if result.err != nil {
					// If an error occurred fetching details for any table, propagate this error
					// for the entire database metadata extraction.
					dbResults <- dbResult{dbName: dbName, err: result.err}
					return
				}
				// Store successfully processed table details by table name.
				processedTablesMap[result.table.Name] = result.table
			}

			// Now, populate dbMetadata.Tables in the original order defined by the 'tables' slice.
			// dbMetadata.Tables was initialized as make([]Table, 0, len(tables)).
			for _, tableName := range tables {
				tableData, found := processedTablesMap[tableName]
				if !found {
					// This indicates an internal logic error: a table listed was not found
					// in the processed results, despite no error being reported for it.
					// This should ideally not happen if error handling in the loop above is correct.
					errMsg := fmt.Errorf("internal error: table '%s' metadata not found after processing for database '%s'", tableName, dbName)
					Error("%v", errMsg) // Log the specific internal error
					dbResults <- dbResult{dbName: dbName, err: errMsg}
					return
				}
				dbMetadata.Tables = append(dbMetadata.Tables, tableData)
			}

			// Build graph after collecting all tables
			for _, table := range dbMetadata.Tables {
				for _, fk := range table.ForeignKeys {
					dbMetadata.Graph[table.Name] = append(dbMetadata.Graph[table.Name], Edge{
						ToTable:    fk.RefTableName,
						FromColumn: fk.ColumnNames[0],
						ToColumn:   fk.RefColumnNames[0],
					})
				}
			}

			dbResults <- dbResult{dbName: dbName, metadata: dbMetadata}
		}(dbName)
	}

	// Collect database results
	for range userDatabases {
		result := <-dbResults
		if result.err != nil {
			Error("Error extracting database metadata: %v", result.err)
			return nil, result.err
		}
		connMetadata.Databases[result.dbName] = result.metadata
	}

	// Store the metadata
	if err := s.storeMetadata(connMetadata); err != nil {
		Error("failed to store metadata: %v", err)
		return nil, fmt.Errorf("failed to store metadata: %w", err)
	}
	Info("Successfully extracted and stored metadata for connection: %s", connectionName)

	return connMetadata, nil
}

// GetMetadata retrieves metadata for a connection, extracting it if necessary
func (s *MetadataService) GetMetadata(ctx context.Context, connectionName string) (*ConnectionMetadata, error) {
	Info("Getting metadata for connection: %s", connectionName)
	// Try to load existing metadata first
	connMetadata, err := s.loadMetadata(connectionName)
	if err != nil {
		Error("failed to load metadata: %v", err)
		return nil, fmt.Errorf("failed to load metadata: %w", err)
	}

	if connMetadata != nil {
		// Check if metadata is still fresh
		if time.Since(connMetadata.LastExtracted) < StaleMetadataThreshold {
			Info("Using cached metadata for connection: %s (age: %v)", connectionName, time.Since(connMetadata.LastExtracted))
			return connMetadata, nil
		}
		Info("Metadata is stale for connection: %s (age: %v), refreshing...", connectionName, time.Since(connMetadata.LastExtracted))
	}

	// If metadata doesn't exist, is too old, or failed to load, extract it
	return s.ExtractMetadata(ctx, connectionName)
}

// storeMetadata saves the metadata to a file
func (s *MetadataService) storeMetadata(metadata *ConnectionMetadata) error {
	filePath := s.getMetadataFilePath(metadata.ConnectionName)
	Info("Storing metadata to file: %s", filePath)

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		Error("failed to marshal metadata: %v", err)
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0600); err != nil {
		Error("failed to write metadata file: %v", err)
		return fmt.Errorf("failed to write metadata file: %w", err)
	}

	Info("Successfully stored metadata for connection: %s", metadata.ConnectionName)
	return nil
}

// loadMetadata loads metadata from a file
func (s *MetadataService) loadMetadata(connectionName string) (*ConnectionMetadata, error) {
	filePath := s.getMetadataFilePath(connectionName)
	Info("Loading metadata from file: %s", filePath)

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			Info("No existing metadata found for connection: %s", connectionName)
			return nil, nil // File doesn't exist, return nil without error
		}
		Error("failed to read metadata file: %v", err)
		return nil, fmt.Errorf("failed to read metadata file: %w", err)
	}

	var metadata ConnectionMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		Error("failed to unmarshal metadata: %v", err)
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	Info("Successfully loaded metadata for connection: %s", connectionName)
	return &metadata, nil
}

// GenerateSimplifiedDDL generates a simplified DDL for a table
func (s *MetadataService) GenerateSimplifiedDDL(table Table) string {
	var ddl string
	ddl = fmt.Sprintf("CREATE TABLE %s (\n", table.Name)

	// Add columns
	for i, col := range table.Columns {
		ddl += fmt.Sprintf("  %s %s", col.Name, col.DataType)
		if !col.IsNullable {
			ddl += " NOT NULL"
		}
		if col.AutoIncrement {
			ddl += " AUTO_INCREMENT"
		}
		if col.DefaultValue != nil {
			ddl += fmt.Sprintf(" DEFAULT %v", col.DefaultValue)
		}
		if i < len(table.Columns)-1 {
			ddl += ",\n"
		}
	}

	// Add primary key if exists
	var pkColumns []string
	for _, col := range table.Columns {
		if col.IsPrimaryKey {
			pkColumns = append(pkColumns, col.Name)
		}
	}
	if len(pkColumns) > 0 {
		ddl += ",\n  PRIMARY KEY ("
		for i, col := range pkColumns {
			if i > 0 {
				ddl += ", "
			}
			ddl += col
		}
		ddl += ")"
	}

	ddl += "\n);"
	return ddl
}

// UpdateAIDescription updates the AI-generated description for a database component
type DescriptionTarget struct {
	Type       string `json:"type"`       // "database", "table", or "column"
	TableName  string `json:"tableName"`  // Required for table and column
	ColumnName string `json:"columnName"` // Required for column
}

func (s *MetadataService) UpdateAIDescription(ctx context.Context, connectionName, dbName string, target DescriptionTarget, description string) error {
	Info("Updating AI description for %s in connection: %s, database: %s", target.Type, connectionName, dbName)

	connMetadata, err := s.loadMetadata(connectionName)
	if err != nil {
		Error("failed to load metadata: %v", err)
		return fmt.Errorf("failed to load metadata: %w", err)
	}
	if connMetadata == nil {
		Error("metadata not found for connection %s", connectionName)
		return fmt.Errorf("metadata not found for connection %s", connectionName)
	}

	dbMetadata, exists := connMetadata.Databases[dbName]
	if !exists {
		return fmt.Errorf("database %s not found in connection %s", dbName, connectionName)
	}

	switch target.Type {
	case "database":
		dbMetadata.AIDescription = description
	case "table":
		for i, table := range dbMetadata.Tables {
			if table.Name == target.TableName {
				dbMetadata.Tables[i].AIDescription = description
				break
			}
		}
	case "column":
		for i, table := range dbMetadata.Tables {
			if table.Name == target.TableName {
				for j, column := range table.Columns {
					if column.Name == target.ColumnName {
						dbMetadata.Tables[i].Columns[j].AIDescription = description
						break
					}
				}
				break
			}
		}
	default:
		return fmt.Errorf("invalid target type: %s", target.Type)
	}

	connMetadata.Databases[dbName] = dbMetadata
	Info("Successfully updated AI description for %s in connection: %s", target.Type, connectionName)
	return s.storeMetadata(connMetadata)
}