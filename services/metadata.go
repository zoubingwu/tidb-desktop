package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	ConnectionID   string                      `json:"connectionId"`   // Connection ID for file storage
	ConnectionName string                      `json:"connectionName"` // Display name
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
	configService  *ConfigService
	dbService      *DatabaseService
	metadataDir    string
	cachedMetadata map[string]*ConnectionMetadata
	mu             sync.RWMutex
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
		configService:  configService,
		dbService:      dbService,
		metadataDir:    metadataDir,
		cachedMetadata: make(map[string]*ConnectionMetadata),
	}, nil
}

// getMetadataFilePath returns the path to the metadata file for a given connection ID
func (s *MetadataService) getMetadataFilePath(connectionID string) string {
	fileName := fmt.Sprintf("%s.json", connectionID)
	return filepath.Join(s.metadataDir, fileName)
}

// isSystemDatabase returns true if the given database name is a system database
func isSystemDatabase(dbName string) bool {
	systemDBs := map[string]bool{
		"information_schema":  true,
		"performance_schema":  true,
		"metrics_schema":      true, // TiDB specific
		"lightning_task_info": true, // TiDB specific
		"mysql":               true,
		"sys":                 true,
	}

	// Convert to lowercase for case-insensitive comparison
	return systemDBs[strings.ToLower(dbName)]
}

// deepCopyConnectionMetadata creates a deep copy of ConnectionMetadata.
// Uses JSON marshal/unmarshal for simplicity. Can be optimized if performance is critical.
func (s *MetadataService) deepCopyConnectionMetadata(original *ConnectionMetadata) *ConnectionMetadata {
	if original == nil {
		return nil
	}
	bytes, err := json.Marshal(original)
	if err != nil {
		LogError("deepCopyConnectionMetadata: Failed to marshal: %v", err)
		// Depending on how critical, might panic or return nil/error
		return nil // Or a new empty one
	}
	var copy ConnectionMetadata
	err = json.Unmarshal(bytes, &copy)
	if err != nil {
		LogError("deepCopyConnectionMetadata: Failed to unmarshal: %v", err)
		return nil // Or a new empty one
	}
	return &copy
}

// UpdateAIDescription updates the AI-generated description for a database component
type DescriptionTarget struct {
	Type       string `json:"type"`       // "database", "table", or "column"
	TableName  string `json:"tableName"`  // Required for table and column
	ColumnName string `json:"columnName"` // Required for column
}

// performExtractionAndCacheUpdate_UNLOCKED assumes caller holds the write lock on s.mu.
// This is the core extraction logic that fetches data from the DB and updates the cache.
func (s *MetadataService) performExtractionAndCacheUpdate_UNLOCKED(ctx context.Context, connectionID string, optionalDbName ...string) (*ConnectionMetadata, error) {
	LogInfo("performExtractionAndCacheUpdate_UNLOCKED: Starting for connection ID: %s, Optional DBs: %v", connectionID, optionalDbName)

	connDetails, existsP, errP := s.configService.GetConnection(connectionID)
	if errP != nil {
		LogError("performExtractionAndCacheUpdate_UNLOCKED: Failed to get conn details for %s: %v", connectionID, errP)
		return nil, fmt.Errorf("failed to get connection details for %s: %w", connectionID, errP)
	}
	if !existsP {
		LogError("performExtractionAndCacheUpdate_UNLOCKED: Connection %s not found", connectionID)
		return nil, fmt.Errorf("connection %s not found", connectionID)
	}

	var currentConnMetadataToBuildUpon *ConnectionMetadata
	isPartialExtraction := len(optionalDbName) > 0 && optionalDbName[0] != ""
	targetDbName := ""
	if isPartialExtraction {
		targetDbName = optionalDbName[0]
	}

	if isPartialExtraction {
		LogInfo("performExtractionAndCacheUpdate_UNLOCKED: Partial mode for connection ID '%s', database '%s'", connectionID, targetDbName)
		existingMetaInCache, foundInCache := s.cachedMetadata[connectionID]
		if foundInCache {
			LogInfo("performExtractionAndCacheUpdate_UNLOCKED: Using existing metadata from cache for merge: %s", connectionID)
			currentConnMetadataToBuildUpon = s.deepCopyConnectionMetadata(existingMetaInCache)
		} else {
			loadedFromFile, loadErr := s.loadMetadataFromFile(connectionID) // loadMetadataFromFile does not use cache
			if loadErr != nil {
				LogError("performExtractionAndCacheUpdate_UNLOCKED: Failed to load existing metadata from file for %s to merge: %v. Proceeding as if new.", connectionID, loadErr)
			} else if loadedFromFile != nil {
				LogInfo("performExtractionAndCacheUpdate_UNLOCKED: Loaded existing metadata from file for %s to merge.", connectionID)
				currentConnMetadataToBuildUpon = loadedFromFile
			}
		}
		if currentConnMetadataToBuildUpon == nil {
			LogInfo("performExtractionAndCacheUpdate_UNLOCKED: No existing metadata for partial update on %s. Creating new structure.", connectionID)
			currentConnMetadataToBuildUpon = &ConnectionMetadata{
				ConnectionID:   connectionID,     // Set connection ID
				ConnectionName: connDetails.Name, // Use connection name from details
				Databases:      make(map[string]DatabaseMetadata),
			}
		}
	} else {
		LogInfo("performExtractionAndCacheUpdate_UNLOCKED: Full mode for connection ID '%s'.", connectionID)
		currentConnMetadataToBuildUpon = &ConnectionMetadata{
			ConnectionID:   connectionID,     // Set connection ID
			ConnectionName: connDetails.Name, // Use connection name from details
			Databases:      make(map[string]DatabaseMetadata),
		}
	}

	var userDatabasesToProcess []string
	if isPartialExtraction {
		if targetDbName == "" {
			LogError("performExtractionAndCacheUpdate_UNLOCKED: Partial extraction for %s but targetDbName is empty.", connectionID)
			return nil, fmt.Errorf("internal error: partial extraction for %s with empty target DB", connectionID)
		}
		userDatabasesToProcess = []string{targetDbName}
	} else {
		allDatabases, errDbList := s.dbService.ListDatabases(ctx, connDetails)
		if errDbList != nil {
			LogError("performExtractionAndCacheUpdate_UNLOCKED: Failed to list databases for %s: %v", connectionID, errDbList)
			return nil, fmt.Errorf("failed to list databases for %s: %w", connectionID, errDbList)
		}
		userDatabasesToProcess = make([]string, 0, len(allDatabases))
		for _, dbName := range allDatabases {
			if !isSystemDatabase(dbName) {
				userDatabasesToProcess = append(userDatabasesToProcess, dbName)
			}
		}
		if len(userDatabasesToProcess) == 0 {
			LogInfo("performExtractionAndCacheUpdate_UNLOCKED: No user databases for full extraction on '%s'. Caching empty metadata.", connectionID)
			currentConnMetadataToBuildUpon.LastExtracted = time.Now()
			s.cachedMetadata[connectionID] = currentConnMetadataToBuildUpon
			return currentConnMetadataToBuildUpon, nil
		}
	}

	LogInfo("performExtractionAndCacheUpdate_UNLOCKED: Processing %d database(s) for connection '%s': %v", len(userDatabasesToProcess), connectionID, userDatabasesToProcess)

	type dbResult struct {
		dbName   string
		metadata DatabaseMetadata
		err      error
	}
	dbResultsChan := make(chan dbResult, len(userDatabasesToProcess))

	for _, dbNameToProcess := range userDatabasesToProcess {
		go func(currentDbName string) {
			LogInfo("performExtractionAndCacheUpdate_UNLOCKED: Goroutine started for database: %s (Connection: %s)", currentDbName, connectionID)
			connDetailsCopy := connDetails // Copy base connection details from outer scope
			connDetailsCopy.DBName = currentDbName

			tables, tableErr := s.dbService.ListTables(ctx, connDetailsCopy, currentDbName)
			if tableErr != nil {
				dbResultsChan <- dbResult{dbName: currentDbName, err: fmt.Errorf("failed to list tables for database %s: %w", currentDbName, tableErr)}
				return
			}

			if len(tables) == 0 {
				LogInfo("performExtractionAndCacheUpdate_UNLOCKED: No tables found in database: %s, creating empty metadata entry.", currentDbName)
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
			} // Errors fetching DB comment are non-fatal

			type tableResult struct {
				table Table
				err   error
			}
			tableResultsChan := make(chan tableResult, len(tables))

			for _, tableName := range tables {
				go func(currentTableName string) {
					LogInfo("performExtractionAndCacheUpdate_UNLOCKED: Goroutine started for table: %s.%s", currentDbName, currentTableName)
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
					} // Errors fetching FKs are non-fatal

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
								nonUniqueVal = v
							case float64:
								nonUniqueVal = int64(v)
							case string:
								if v == "1" {
									nonUniqueVal = 1
								}
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
				}(tableName)
			}

			processedTablesMap := make(map[string]Table, len(tables))
			var firstTableError error
			for i := 0; i < len(tables); i++ {
				result := <-tableResultsChan
				if result.err != nil {
					errMsg := result.err
					LogError("performExtractionAndCacheUpdate_UNLOCKED: Error processing table for database '%s': %v", currentDbName, errMsg)
					// Propagate the first error encountered for a table within this DB's processing
					if firstTableError == nil {
						firstTableError = errMsg
					}
				}
				// Even if an error occurred for one table, we collect successful ones for this DB,
				// but the DB processing will be marked as failed if firstTableError is set.
				if result.err == nil {
					processedTablesMap[result.table.Name] = result.table
				}
			}

			if firstTableError != nil {
				dbResultsChan <- dbResult{dbName: currentDbName, err: firstTableError}
				return
			}

			// Ensure tables are added in the original order from ListTables
			for _, tableNameFromList := range tables {
				tableData, found := processedTablesMap[tableNameFromList]
				if !found {
					// This case should ideally not happen if all tables processed successfully without error above
					// and no error was reported. If a table is missing, it implies an issue.
					errMissingTable := fmt.Errorf("internal logic error: table '%s' not found in processed map for db '%s'", tableNameFromList, currentDbName)
					LogError("performExtractionAndCacheUpdate_UNLOCKED: %v", errMissingTable)
					dbResultsChan <- dbResult{dbName: currentDbName, err: errMissingTable}
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
		}(dbNameToProcess)
	}

	var firstOverallExtractionError error
	successfulTempMetadata := make(map[string]DatabaseMetadata)

	for i := 0; i < len(userDatabasesToProcess); i++ {
		result := <-dbResultsChan
		if result.err != nil {
			LogError("performExtractionAndCacheUpdate_UNLOCKED: Error processing database %s: %v", result.dbName, result.err)
			if firstOverallExtractionError == nil {
				firstOverallExtractionError = result.err
			}
		} else {
			if firstOverallExtractionError == nil {
				successfulTempMetadata[result.dbName] = result.metadata
			}
		}
	}

	if firstOverallExtractionError != nil {
		LogError("performExtractionAndCacheUpdate_UNLOCKED: Failed overall for connection '%s' due to first error: %v. Cache NOT updated with partial/failed results.", connectionID, firstOverallExtractionError)
		return nil, firstOverallExtractionError
	}

	if currentConnMetadataToBuildUpon.Databases == nil {
		currentConnMetadataToBuildUpon.Databases = make(map[string]DatabaseMetadata)
	}
	for dbNameKey, metaValue := range successfulTempMetadata {
		currentConnMetadataToBuildUpon.Databases[dbNameKey] = metaValue
	}

	currentConnMetadataToBuildUpon.LastExtracted = time.Now()
	s.cachedMetadata[connectionID] = currentConnMetadataToBuildUpon // Update cache

	if isPartialExtraction {
		LogInfo("performExtractionAndCacheUpdate_UNLOCKED: Successfully refreshed in-memory metadata for database '%s' in connection '%s'.", targetDbName, connectionID)
	} else {
		LogInfo("performExtractionAndCacheUpdate_UNLOCKED: Successfully performed full extraction and updated in-memory metadata for connection '%s'. Processed %d database(s).", connectionID, len(userDatabasesToProcess))
	}
	return currentConnMetadataToBuildUpon, nil
}

// ExtractMetadata (Public API) refreshes the cache for one or all DBs of a connection.
// It handles locking and calls the internal unlocked extraction function.
func (s *MetadataService) ExtractMetadata(ctx context.Context, connectionID string, optionalDbName ...string) (*ConnectionMetadata, error) {
	LogInfo("ExtractMetadata (Public): Request to refresh metadata for connection ID: %s, Optional DBs: %v", connectionID, optionalDbName)
	s.mu.Lock()
	defer s.mu.Unlock()

	newOrUpdatedMeta, err := s.performExtractionAndCacheUpdate_UNLOCKED(ctx, connectionID, optionalDbName...)
	if err != nil {
		// Error already logged by performExtractionAndCacheUpdate_UNLOCKED
		return nil, err
	}
	LogInfo("ExtractMetadata (Public): In-memory metadata updated for %s. Call SaveMetadata to persist.", connectionID)
	return newOrUpdatedMeta, nil
}

// GetMetadata retrieves metadata for a connection.
// It loads from file or extracts if not in cache or stale.
func (s *MetadataService) GetMetadata(ctx context.Context, connectionID string) (*ConnectionMetadata, error) {
	LogInfo("GetMetadata: Request for connection ID: %s", connectionID)

	s.mu.RLock()
	cachedMeta, foundInCache := s.cachedMetadata[connectionID]
	isFresh := foundInCache && time.Since(cachedMeta.LastExtracted) < StaleMetadataThreshold
	s.mu.RUnlock()

	if isFresh {
		LogInfo("GetMetadata: Using fresh metadata from cache for connection ID: %s (age: %v)", connectionID, time.Since(cachedMeta.LastExtracted))
		return cachedMeta, nil
	}

	s.mu.Lock() // Acquire write lock to load or extract
	defer s.mu.Unlock()

	// Double-check cache after acquiring write lock
	cachedMeta, foundInCache = s.cachedMetadata[connectionID]
	if foundInCache && time.Since(cachedMeta.LastExtracted) < StaleMetadataThreshold {
		LogInfo("GetMetadata: Fresh metadata found in cache (after lock) for connection ID: %s", connectionID)
		return cachedMeta, nil
	}

	if foundInCache {
		LogInfo("GetMetadata: Cached metadata for %s is stale (age: %v). Will attempt load/extract.", connectionID, time.Since(cachedMeta.LastExtracted))
	} else {
		LogInfo("GetMetadata: Metadata for %s not in cache. Will attempt load/extract.", connectionID)
	}

	loadedFromFile, loadErr := s.loadMetadataFromFile(connectionID)
	if loadErr != nil {
		LogError("Error loading metadata from file for %s: %v. Will proceed to extraction.", connectionID, loadErr)
	} else if loadedFromFile != nil {
		if time.Since(loadedFromFile.LastExtracted) < StaleMetadataThreshold {
			LogInfo("GetMetadata: Loaded fresh metadata from file for %s. Updating cache.", connectionID)
			s.cachedMetadata[connectionID] = loadedFromFile
			return loadedFromFile, nil
		}
		LogInfo("GetMetadata: Metadata from file for %s is stale (%v old). Will proceed to extraction.", connectionID, time.Since(loadedFromFile.LastExtracted))
	} else {
		LogInfo("GetMetadata: No metadata file found for %s. Will proceed to extraction.", connectionID)
	}

	LogInfo("GetMetadata: Proceeding to extract/refresh metadata for connection ID %s (will update cache).", connectionID)
	// Calls the UNLOCKED version as we already hold the lock.
	// Passing no arguments for optionalDbName means full extraction for this connection.
	extractedMeta, extractErr := s.performExtractionAndCacheUpdate_UNLOCKED(ctx, connectionID)
	if extractErr != nil {
		LogError("Failed to extract/refresh metadata for %s: %v", connectionID, extractErr)
		// Consider returning stale data if available and extraction fails?
		// For now, an error means we couldn't get fresh data.
		return nil, fmt.Errorf("failed to extract/refresh metadata for %s: %w", connectionID, extractErr)
	}

	LogInfo("GetMetadata: Successfully extracted/refreshed metadata for %s, now in cache.", connectionID)
	return extractedMeta, nil // This is the newly extracted and cached metadata
}

// storeMetadataToFile saves the metadata to a file (formerly storeMetadata).
// This function does NOT interact with the cache.
func (s *MetadataService) storeMetadataToFile(metadata *ConnectionMetadata) error {
	filePath := s.getMetadataFilePath(metadata.ConnectionID)
	LogInfo("Storing metadata to file: %s", filePath)

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return LogError("Failed to marshal metadata: %v", err)
	}

	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return LogError("Failed to write metadata file: %v", err)
	}

	LogInfo("Successfully stored metadata to file for connection ID: %s", metadata.ConnectionID)
	return nil
}

// loadMetadataFromFile loads metadata from a file (formerly loadMetadata).
// This function does NOT interact with the cache.
func (s *MetadataService) loadMetadataFromFile(connectionID string) (*ConnectionMetadata, error) {
	filePath := s.getMetadataFilePath(connectionID)
	LogInfo("Loading metadata from file: %s", filePath)

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			LogInfo("No existing metadata file found for connection ID: %s at %s", connectionID, filePath)
			return nil, nil // File doesn't exist, return nil without error
		}
		return nil, LogError("Failed to read metadata file %s: %v", filePath, err)
	}

	if len(data) == 0 { // Handle empty file case
		LogInfo("Metadata file %s is empty for connection ID: %s", filePath, connectionID)
		return nil, nil // Treat as if not found or corrupted
	}

	var metadata ConnectionMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, LogError("Failed to unmarshal metadata from file %s: %v", filePath, err)
	}

	// Ensure ConnectionID is set for backward compatibility
	if metadata.ConnectionID == "" {
		metadata.ConnectionID = connectionID
	}

	LogInfo("Successfully loaded metadata from file for connection ID: %s", connectionID)
	return &metadata, nil
}

// SaveMetadata saves the current in-memory metadata for a specific connection ID to its file.
func (s *MetadataService) SaveMetadata(connectionID string) error {
	s.mu.RLock()
	metadataToSave, exists := s.cachedMetadata[connectionID]
	s.mu.RUnlock()

	if !exists {
		LogInfo("SaveMetadata: Metadata for connection ID %s not found in cache, cannot save.", connectionID)
		return fmt.Errorf("metadata for connection ID %s not found in cache", connectionID)
	}

	// It's generally safer to save a copy, especially if the cached item could be modified concurrently
	// by another goroutine between the RUnlock and the actual saving process.
	// However, storeMetadataToFile takes *ConnectionMetadata, so a deep copy is needed if we don't want
	// the save operation to see mid-flight changes from other ops (though unlikely with current structure).
	// For simplicity now, assume metadataToSave is stable enough or Save is called when state is consistent.
	// If high concurrency and modification is a concern, deep copy here:
	// metaCopyForSave := s.deepCopyConnectionMetadata(metadataToSave)
	// return s.storeMetadataToFile(metaCopyForSave)

	LogInfo("SaveMetadata: Attempting to save metadata for connection ID %s to disk.", connectionID)
	return s.storeMetadataToFile(metadataToSave)
}

// UpdateAIDescription updates the AI-generated description for a database component in the cache.
// Call SaveMetadata to persist changes.
func (s *MetadataService) UpdateAIDescription(ctx context.Context, connectionID, dbName string, target DescriptionTarget, description string) error {
	LogInfo("UpdateAIDescription: Request for %s in connection ID: %s, database: %s", target.Type, connectionID, dbName)

	s.mu.Lock() // Lock for modifying cache
	defer s.mu.Unlock()

	connMetadata, existsInCache := s.cachedMetadata[connectionID]
	if !existsInCache {
		// If not in cache, load it. UpdateAIDescription should operate on existing data.
		// If no data file exists, this implies metadata hasn't been extracted yet.
		LogInfo("UpdateAIDescription: Metadata for %s not in cache. Attempting to load from file.", connectionID)
		loadedMeta, err := s.loadMetadataFromFile(connectionID)
		if err != nil {
			LogError("UpdateAIDescription: Failed to load metadata file for %s to update AI desc: %v", connectionID, err)
			return fmt.Errorf("failed to load metadata for %s to update description: %w", connectionID, err)
		}
		if loadedMeta == nil {
			LogError("UpdateAIDescription: Metadata file not found for %s. Cannot update AI desc. Extract metadata first.", connectionID)
			return fmt.Errorf("metadata not found for connection ID %s (file missing), extract first", connectionID)
		}
		s.cachedMetadata[connectionID] = loadedMeta // Add to cache
		connMetadata = loadedMeta
		LogInfo("UpdateAIDescription: Loaded metadata for %s into cache.", connectionID)
	}

	// Work on a copy of the specific database metadata to avoid complex partial updates on the shared cached object.
	// Then, replace the database metadata in the main connection metadata.
	dbMeta, dbExists := connMetadata.Databases[dbName]
	if !dbExists {
		LogError("UpdateAIDescription: Database %s not found in connection ID %s", dbName, connectionID)
		return fmt.Errorf("database %s not found in connection ID %s", dbName, connectionID)
	}

	// Create a mutable copy of the DatabaseMetadata struct.
	// dbMeta is a struct, so this is a shallow copy. Modifying its fields is fine.
	// If dbMeta contained pointers that were modified, a deep copy would be needed.
	// Tables is a slice of structs. Iterating and modifying elements requires care.
	dbMetaCopy := dbMeta // This is a value copy of the struct.
	found := false

	switch target.Type {
	case "database":
		dbMetaCopy.AIDescription = description
		found = true
	case "table":
		for i, table := range dbMetaCopy.Tables { // table is a copy
			if table.Name == target.TableName {
				table.AIDescription = description
				dbMetaCopy.Tables[i] = table // Put modified copy back
				found = true
				break
			}
		}
		if !found {
			LogError("UpdateAIDescription: Table %s not found in DB %s for connection ID %s", target.TableName, dbName, connectionID)
			return fmt.Errorf("table %s not found in database %s", target.TableName, dbName)
		}
	case "column":
		tableIdx := -1
		for i, t := range dbMetaCopy.Tables {
			if t.Name == target.TableName {
				tableIdx = i
				break
			}
		}
		if tableIdx == -1 {
			LogError("UpdateAIDescription: Table %s not found for column update in DB %s, conn ID %s", target.TableName, dbName, connectionID)
			return fmt.Errorf("table %s not found in database %s for column update", target.TableName, dbName)
		}

		// Modify a copy of the table
		tableCopy := dbMetaCopy.Tables[tableIdx]
		for j, col := range tableCopy.Columns { // col is a copy
			if col.Name == target.ColumnName {
				col.AIDescription = description
				tableCopy.Columns[j] = col // Put modified copy back
				found = true
				break
			}
		}
		if found {
			dbMetaCopy.Tables[tableIdx] = tableCopy // Put modified table copy back
		} else {
			LogError("UpdateAIDescription: Column %s not found in table %s, DB %s, conn ID %s", target.ColumnName, target.TableName, dbName, connectionID)
			return fmt.Errorf("column %s not found in table %s", target.ColumnName, target.TableName)
		}
	default:
		LogError("UpdateAIDescription: Invalid target type: %s", target.Type)
		return fmt.Errorf("invalid target type: %s", target.Type)
	}

	connMetadata.Databases[dbName] = dbMetaCopy
	connMetadata.LastExtracted = time.Now() // Mark that the connection metadata has been updated

	// The main connMetadata object in the cache (s.cachedMetadata[connectionID]) has been updated.
	LogInfo("UpdateAIDescription: Successfully updated in-memory AI description for %s. Call SaveMetadata to persist.", target.Type)
	return nil
}

// DeleteConnectionMetadataFile removes the persisted metadata file for a connection ID.
// This is useful if a connection is deleted from the config.
func (s *MetadataService) DeleteConnectionMetadataFile(connectionID string) error {
	s.mu.Lock()                            // Also ensure cache consistency if we decide to remove from cache here
	delete(s.cachedMetadata, connectionID) // Remove from cache
	s.mu.Unlock()

	filePath := s.getMetadataFilePath(connectionID)
	LogInfo("Deleting metadata file: %s", filePath)
	err := os.Remove(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			LogInfo("Metadata file %s not found, nothing to delete.", filePath)
			return nil // Not an error if it doesn't exist
		}
		return LogError("Failed to delete metadata file %s: %v", filePath, err)
	}
	LogInfo("Successfully deleted metadata file %s.", filePath)
	return nil
}
