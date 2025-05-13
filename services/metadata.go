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
	Name           string   `json:"name"`
	ColumnNames    []string `json:"columnNames"`
	RefTableName   string   `json:"refTableName"`
	RefColumnNames []string `json:"refColumnNames"`
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
	Graph         map[string][]Edge `json:"graph,omitempty"`         // Adjacency list representation
	DBComment     string            `json:"dbComment,omitempty"`     // Comment from database
	AIDescription string            `json:"aiDescription,omitempty"` // Description from AI
}

// ConnectionMetadata represents the complete metadata for a connection
type ConnectionMetadata struct {
	ConnectionName string                      `json:"connectionName"`
	LastExtracted  time.Time                   `json:"lastExtracted"`
	Databases      map[string]DatabaseMetadata `json:"databases"`
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
		"information_schema": true,
		"performance_schema": true,
		"metrics_schema":     true, // TiDB specific
		"mysql":              true,
		"sys":                true,
	}

	// Convert to lowercase for case-insensitive comparison
	return systemDBs[strings.ToLower(dbName)]
}

// ExtractMetadata extracts metadata from the database and stores it.
// If optionalDbName is provided, metadata for that specific database is extracted and
// merged into the existing connection metadata. Otherwise, a full extraction is performed.
func (s *MetadataService) ExtractMetadata(ctx context.Context, connectionName string, optionalDbName ...string) (*ConnectionMetadata, error) {
	Info("ExtractMetadata: Starting for connection: %s, Optional DBs: %v", connectionName, optionalDbName)

	connDetails, exists, err := s.configService.GetConnection(connectionName)
	if err != nil {
		Error("ExtractMetadata: Failed to get connection details for %s: %v", connectionName, err)
		return nil, fmt.Errorf("failed to get connection details for %s: %w", connectionName, err)
	}
	if !exists {
		Error("ExtractMetadata: Connection %s not found", connectionName)
		return nil, fmt.Errorf("connection %s not found", connectionName)
	}

	var connMetadata *ConnectionMetadata
	var userDatabases []string // Databases to actually process
	var targetDbName string    // Only set if optionalDbName is provided

	isPartialExtraction := len(optionalDbName) > 0 && optionalDbName[0] != ""

	if isPartialExtraction {
		targetDbName = optionalDbName[0]
		Info("ExtractMetadata: Partial mode for connection '%s', database '%s'", connectionName, targetDbName)

		existingConnMetadata, loadErr := s.loadMetadata(connectionName)
		if loadErr != nil { // loadMetadata returns (nil, nil) if file not exist, error otherwise
			Error("ExtractMetadata: Failed to load existing metadata for connection '%s' to merge: %v", connectionName, loadErr)
			return nil, fmt.Errorf("failed to load existing metadata for %s to merge: %w", connectionName, loadErr)
		}

		if existingConnMetadata != nil {
			connMetadata = existingConnMetadata
			Info("ExtractMetadata: Loaded existing metadata for connection '%s' to merge with.", connectionName)
		} else {
			Info("ExtractMetadata: No existing metadata found for connection '%s'. New metadata file will be created including database '%s'.", connectionName, targetDbName)
			connMetadata = &ConnectionMetadata{
				ConnectionName: connectionName,
				Databases:      make(map[string]DatabaseMetadata),
				// LastExtracted will be set before storing
			}
		}
		userDatabases = []string{targetDbName} // Process only the specified database
	} else {
		Info("ExtractMetadata: Full mode for connection '%s'. All user databases will be processed.", connectionName)
		connMetadata = &ConnectionMetadata{
			ConnectionName: connectionName,
			Databases:      make(map[string]DatabaseMetadata), // Overwrite with fresh data
			// LastExtracted will be set before storing
		}

		allDatabases, errDbList := s.dbService.ListDatabases(ctx, connDetails)
		if errDbList != nil {
			Error("ExtractMetadata: Failed to list databases for connection %s: %v", connectionName, errDbList)
			return nil, fmt.Errorf("failed to list databases for %s: %w", connectionName, errDbList)
		}

		userDatabases = make([]string, 0, len(allDatabases))
		for _, dbName := range allDatabases {
			if !isSystemDatabase(dbName) {
				userDatabases = append(userDatabases, dbName)
			}
		}

		if len(userDatabases) == 0 {
			Info("ExtractMetadata: No user databases found for full extraction on connection '%s'. Storing empty metadata.", connectionName)
			connMetadata.LastExtracted = time.Now()
			if errStore := s.storeMetadata(connMetadata); errStore != nil {
				Error("ExtractMetadata: Failed to store empty metadata for connection %s: %v", connectionName, errStore)
				return connMetadata, fmt.Errorf("failed to store empty metadata for %s: %w", connectionName, errStore)
			}
			Info("ExtractMetadata: Successfully stored empty metadata for connection '%s'.", connectionName)
			return connMetadata, nil
		}
	}

	Info("ExtractMetadata: Processing %d database(s) for connection '%s': %v", len(userDatabases), connectionName, userDatabases)

	type dbResult struct {
		dbName   string
		metadata DatabaseMetadata
		err      error
	}
	dbResultsChan := make(chan dbResult, len(userDatabases))

	for _, dbName := range userDatabases {
		// dbName is passed to the goroutine, creating a new instance for each.
		go func(currentDbName string) {
			Info("ExtractMetadata: Goroutine started for database: %s (Connection: %s)", currentDbName, connectionName)
			connDetailsCopy := connDetails // Copy base connection details
			connDetailsCopy.DBName = currentDbName

			tables, tableErr := s.dbService.ListTables(ctx, connDetailsCopy, currentDbName)
			if tableErr != nil {
				dbResultsChan <- dbResult{dbName: currentDbName, err: fmt.Errorf("failed to list tables for database %s: %w", currentDbName, tableErr)}
				return
			}

			if len(tables) == 0 {
				Info("ExtractMetadata: No tables found in database: %s, creating empty metadata entry.", currentDbName)
				dbResultsChan <- dbResult{dbName: currentDbName, metadata: DatabaseMetadata{
					Name:   currentDbName,
					Tables: []Table{},
					Graph:  make(map[string][]Edge),
				}}
				return
			}

			dbMetadata := DatabaseMetadata{
				Name:   currentDbName,
				Tables: make([]Table, 0, len(tables)),
				Graph:  make(map[string][]Edge),
			}

			dbCommentQuery := fmt.Sprintf(`
				SELECT SCHEMA_COMMENT
				FROM information_schema.SCHEMATA
				WHERE SCHEMA_NAME = '%s';`, currentDbName)

			if result, execErr := s.dbService.ExecuteSQL(ctx, connDetailsCopy, dbCommentQuery); execErr == nil && len(result.Rows) > 0 {
				if comment, ok := result.Rows[0]["SCHEMA_COMMENT"].(string); ok && comment != "" {
					dbMetadata.DBComment = comment
				}
			} // Errors fetching DB comment are non-fatal for metadata extraction

			type tableResult struct {
				table Table
				err   error
			}
			tableResultsChan := make(chan tableResult, len(tables))

			for _, tableName := range tables {
				go func(currentTableName string) {
					Info("ExtractMetadata: Goroutine started for table: %s.%s", currentDbName, currentTableName)
					table := Table{
						Name:        currentTableName,
						Columns:     make([]Column, 0),
						ForeignKeys: make([]ForeignKey, 0),
						Indexes:     make([]Index, 0),
					}

					tableSchema, schemaErr := s.dbService.GetTableSchema(ctx, connDetailsCopy, currentDbName, currentTableName)
					if schemaErr != nil {
						tableResultsChan <- tableResult{err: fmt.Errorf("failed to get schema for table %s in database %s: %w", currentTableName, currentDbName, schemaErr)}
						return
					}

					tableCommentQuery := fmt.Sprintf(`
						SELECT TABLE_COMMENT
						FROM information_schema.TABLES
						WHERE TABLE_SCHEMA = '%s'
						AND TABLE_NAME = '%s';`, currentDbName, currentTableName)

					if result, tableCommentErr := s.dbService.ExecuteSQL(ctx, connDetailsCopy, tableCommentQuery); tableCommentErr == nil && len(result.Rows) > 0 {
						if comment, ok := result.Rows[0]["TABLE_COMMENT"].(string); ok && comment != "" {
							table.DBComment = comment
						}
					} // Errors fetching table comment are non-fatal

					columnCommentsQuery := fmt.Sprintf(`
						SELECT COLUMN_NAME, COLUMN_COMMENT
						FROM information_schema.COLUMNS
						WHERE TABLE_SCHEMA = '%s'
						AND TABLE_NAME = '%s';`, currentDbName, currentTableName)

					columnComments := make(map[string]string)
					if result, colCommentErr := s.dbService.ExecuteSQL(ctx, connDetailsCopy, columnCommentsQuery); colCommentErr == nil {
						for _, row := range result.Rows {
							if colName, ok := row["COLUMN_NAME"].(string); ok {
								if comment, okComment := row["COLUMN_COMMENT"].(string); okComment && comment != "" {
									columnComments[colName] = comment
								}
							}
						}
					} // Errors fetching column comments are non-fatal

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

					fkQuery := fmt.Sprintf(`
						SELECT CONSTRAINT_NAME, COLUMN_NAME, REFERENCED_TABLE_NAME, REFERENCED_COLUMN_NAME
						FROM information_schema.KEY_COLUMN_USAGE
						WHERE TABLE_SCHEMA = '%s' AND TABLE_NAME = '%s' AND REFERENCED_TABLE_NAME IS NOT NULL;`, currentDbName, currentTableName)

					fkResult, fkErr := s.dbService.ExecuteSQL(ctx, connDetailsCopy, fkQuery)
					if fkErr == nil && fkResult != nil && len(fkResult.Rows) > 0 {
						fkMap := make(map[string]*ForeignKey)
						for _, row := range fkResult.Rows {
							constraintName, _ := row["CONSTRAINT_NAME"].(string)
							columnName, _ := row["COLUMN_NAME"].(string)
							refTableName, _ := row["REFERENCED_TABLE_NAME"].(string)
							refColumnName, _ := row["REFERENCED_COLUMN_NAME"].(string)

							if fk, ok := fkMap[constraintName]; ok {
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
						for _, fk := range fkMap {
							table.ForeignKeys = append(table.ForeignKeys, *fk)
						}
					} // Errors fetching FKs are non-fatal for basic table metadata

					indexQuery := fmt.Sprintf(`
						SELECT INDEX_NAME, COLUMN_NAME, NON_UNIQUE
						FROM information_schema.STATISTICS
						WHERE TABLE_SCHEMA = '%s' AND TABLE_NAME = '%s' ORDER BY INDEX_NAME, SEQ_IN_INDEX;`, currentDbName, currentTableName)

					indexResult, indexErr := s.dbService.ExecuteSQL(ctx, connDetailsCopy, indexQuery)
					if indexErr == nil && indexResult != nil && len(indexResult.Rows) > 0 {
						indexMap := make(map[string]*Index)
						for _, row := range indexResult.Rows {
							indexName, _ := row["INDEX_NAME"].(string)
							columnName, _ := row["COLUMN_NAME"].(string)
							var nonUniqueVal int64
							switch v := row["NON_UNIQUE"].(type) {
							case int64:
								nonUniqueVal = v // MySQL/TiDB typically use 0 or 1 (numeric)
							case float64:
								nonUniqueVal = int64(v) // Handle if it comes as float
							case string:
								if v == "1" {
									nonUniqueVal = 1
								} // Handle if it comes as string "1"
							}
							isNonUnique := nonUniqueVal == 1

							if idx, ok := indexMap[indexName]; ok {
								idx.ColumnNames = append(idx.ColumnNames, columnName)
							} else {
								indexMap[indexName] = &Index{
									Name:        indexName,
									ColumnNames: []string{columnName},
									IsUnique:    !isNonUnique,
								}
							}
						}
						for _, idx := range indexMap {
							table.Indexes = append(table.Indexes, *idx)
						}
					} // Errors fetching Indexes are non-fatal

					tableResultsChan <- tableResult{table: table}
				}(tableName) // Pass tableName to the goroutine
			}

			processedTablesMap := make(map[string]Table, len(tables))
			var firstTableError error
			for i := 0; i < len(tables); i++ {
				result := <-tableResultsChan
				if result.err != nil {
					errMsg := result.err
					Error("ExtractMetadata: Error processing table during collection for database '%s': %v", currentDbName, errMsg)
					firstTableError = errMsg
					dbResultsChan <- dbResult{dbName: currentDbName, err: firstTableError}
					return
				}
				processedTablesMap[result.table.Name] = result.table
			}

			for _, tableNameFromList := range tables {
				tableData, found := processedTablesMap[tableNameFromList]
				if !found {
					errMsg := fmt.Errorf("internal logic error: table '%s' from initial list not found in processed map for database '%s'", tableNameFromList, currentDbName)
					Error("ExtractMetadata: %v", errMsg)
					dbResultsChan <- dbResult{dbName: currentDbName, err: errMsg}
					return
				}
				dbMetadata.Tables = append(dbMetadata.Tables, tableData)
			}

			for _, table := range dbMetadata.Tables {
				for _, fk := range table.ForeignKeys {
					if len(fk.ColumnNames) > 0 && len(fk.RefColumnNames) > 0 {
						dbMetadata.Graph[table.Name] = append(dbMetadata.Graph[table.Name], Edge{
							ToTable:    fk.RefTableName,
							FromColumn: fk.ColumnNames[0],
							ToColumn:   fk.RefColumnNames[0],
						})
					}
				}
			}

			dbResultsChan <- dbResult{dbName: currentDbName, metadata: dbMetadata}
		}(dbName) // Pass dbName to the goroutine (it becomes currentDbName inside)
	}

	var firstExtractionError error
	successfulTempMetadata := make(map[string]DatabaseMetadata)

	for i := 0; i < len(userDatabases); i++ {
		result := <-dbResultsChan
		if result.err != nil {
			if firstExtractionError == nil {
				firstExtractionError = result.err
			}
		} else {
			if firstExtractionError == nil { // Only collect if no global error flag is set
				successfulTempMetadata[result.dbName] = result.metadata
			}
		}
	}

	if firstExtractionError != nil {
		Error("ExtractMetadata: Failed overall for connection '%s' due to first error: %v. No metadata changes will be stored.", connectionName, firstExtractionError)
		return nil, firstExtractionError
	}

	for dbNameKey, metaValue := range successfulTempMetadata {
		connMetadata.Databases[dbNameKey] = metaValue
	}

	connMetadata.LastExtracted = time.Now()

	if err := s.storeMetadata(connMetadata); err != nil {
		Error("ExtractMetadata: Failed to store metadata for connection %s: %v", connectionName, err)
		return nil, fmt.Errorf("failed to store metadata for %s: %w", connectionName, err)
	}

	if isPartialExtraction {
		Info("ExtractMetadata: Successfully refreshed and stored metadata for database '%s' in connection '%s'.", targetDbName, connectionName)
	} else {
		Info("ExtractMetadata: Successfully performed full extraction and stored metadata for connection '%s'. Processed %d database(s): %v", connectionName, len(userDatabases), userDatabases)
	}

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

	Info("Successfully loaded metadata for connection: %s", metadata.ConnectionName)
	return &metadata, nil
}

// GenerateSimplifiedDDL generates a simplified DDL for a table
func (s *MetadataService) GenerateSimplifiedDDL(table Table) string {
	var ddl strings.Builder
	ddl.WriteString(fmt.Sprintf("CREATE TABLE %s (\n", table.Name))

	for i, col := range table.Columns {
		ddl.WriteString(fmt.Sprintf("  %s %s", col.Name, col.DataType))
		if !col.IsNullable {
			ddl.WriteString(" NOT NULL")
		}
		if col.AutoIncrement {
			ddl.WriteString(" AUTO_INCREMENT")
		}
		if col.DefaultValue != nil {
			if strVal, ok := col.DefaultValue.(string); ok {
				ddl.WriteString(fmt.Sprintf(" DEFAULT '%s'", strings.ReplaceAll(strVal, "'", "''")))
			} else {
				ddl.WriteString(fmt.Sprintf(" DEFAULT %v", col.DefaultValue))
			}
		}
		if col.DBComment != "" {
			ddl.WriteString(fmt.Sprintf(" COMMENT '%s'", strings.ReplaceAll(col.DBComment, "'", "''")))
		}
		if i < len(table.Columns)-1 {
			ddl.WriteString(",\n")
		} else {
			ddl.WriteString("\n") // Newline after the last column definition
		}
	}

	var pkColumns []string
	for _, col := range table.Columns {
		if col.IsPrimaryKey {
			pkColumns = append(pkColumns, col.Name)
		}
	}
	if len(pkColumns) > 0 {
		ddl.WriteString(fmt.Sprintf(",\n  PRIMARY KEY (%s)\n", strings.Join(pkColumns, ", ")))
	}

	ddl.WriteString(")")
	if table.DBComment != "" {
		ddl.WriteString(fmt.Sprintf(" COMMENT='%s'", strings.ReplaceAll(table.DBComment, "'", "''")))
	}
	ddl.WriteString(";")
	return ddl.String()
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
