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
	configService *ConfigService
	dbService     *DatabaseService
	metadataDir   string
	// Simple in-memory storage per connection
	metadata map[string]*ConnectionMetadata
	mu       sync.RWMutex
}

// DescriptionTarget for updating AI descriptions
type DescriptionTarget struct {
	Type       string `json:"type"`       // "database", "table", or "column"
	TableName  string `json:"tableName"`  // Required for table and column
	ColumnName string `json:"columnName"` // Required for column
}

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
		metadata:      make(map[string]*ConnectionMetadata),
	}, nil
}

// LoadMetadata loads metadata from file into memory for a connection
func (s *MetadataService) LoadMetadata(ctx context.Context, connectionID string) (*ConnectionMetadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get connection details for the name
	connDetails, exists, err := s.configService.GetConnection(connectionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection details: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("connection not found: %s", connectionID)
	}

	filePath := s.getMetadataFilePath(connectionID)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, create empty structure - extraction will be triggered by frontend events
			LogInfo("No metadata file found for connection %s, creating empty structure", connectionID)
			metadata := &ConnectionMetadata{
				ConnectionID:   connectionID,
				ConnectionName: connDetails.Name,
				LastExtracted:  time.Time{}, // Zero time indicates never extracted
				Databases:      make(map[string]DatabaseMetadata),
			}
			s.metadata[connectionID] = metadata
			return metadata, nil
		}
		return nil, fmt.Errorf("failed to read metadata file: %w", err)
	}

	var metadata ConnectionMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	// Ensure fields are set for backward compatibility
	if metadata.ConnectionID == "" {
		metadata.ConnectionID = connectionID
	}
	if metadata.ConnectionName == "" {
		metadata.ConnectionName = connDetails.Name
	}

	s.metadata[connectionID] = &metadata
	LogInfo("Loaded metadata for connection: %s", connectionID)
	return &metadata, nil
}

// GetMetadata returns the in-memory metadata for a connection
func (s *MetadataService) GetMetadata(ctx context.Context, connectionID string) (*ConnectionMetadata, error) {
	s.mu.RLock()
	metadata, exists := s.metadata[connectionID]
	s.mu.RUnlock()

	if !exists {
		return s.LoadMetadata(ctx, connectionID)
	}

	return metadata, nil
}

// SaveMetadata saves the in-memory metadata to file
func (s *MetadataService) SaveMetadata(connectionID string) error {
	s.mu.RLock()
	metadata, exists := s.metadata[connectionID]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("metadata not found in memory for connection: %s", connectionID)
	}

	filePath := s.getMetadataFilePath(connectionID)
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}

	LogInfo("Saved metadata for connection: %s", connectionID)
	return nil
}

// ExtractMetadata performs fresh extraction from database and updates memory
func (s *MetadataService) ExtractMetadata(ctx context.Context, connectionID string, optionalDbName ...string) (*ConnectionMetadata, error) {
	connDetails, exists, err := s.configService.GetConnection(connectionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection details: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("connection not found: %s", connectionID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Get or create metadata structure
	metadata, exists := s.metadata[connectionID]
	if !exists {
		metadata = &ConnectionMetadata{
			ConnectionID:   connectionID,
			ConnectionName: connDetails.Name,
			Databases:      make(map[string]DatabaseMetadata),
		}
		s.metadata[connectionID] = metadata
	}

	// Determine which databases to extract
	var databasesToExtract []string
	if len(optionalDbName) > 0 && optionalDbName[0] != "" {
		// Partial extraction for specific database
		databasesToExtract = []string{optionalDbName[0]}
		LogInfo("Extracting metadata for database: %s", optionalDbName[0])
	} else {
		// Full extraction - get all user databases
		allDatabases, err := s.dbService.ListDatabases(ctx, connDetails)
		if err != nil {
			return nil, fmt.Errorf("failed to list databases: %w", err)
		}

		for _, dbName := range allDatabases {
			if !isSystemDatabase(dbName) {
				databasesToExtract = append(databasesToExtract, dbName)
			}
		}
		LogInfo("Extracting metadata for %d databases", len(databasesToExtract))
	}

	// Extract metadata for each database
	for _, dbName := range databasesToExtract {
		dbMetadata, err := s.extractDatabaseMetadata(ctx, connDetails, dbName)
		if err != nil {
			return nil, fmt.Errorf("failed to extract metadata for database %s: %w", dbName, err)
		}
		metadata.Databases[dbName] = *dbMetadata
	}

	metadata.LastExtracted = time.Now()
	LogInfo("Extraction completed for connection: %s", connectionID)
	return metadata, nil
}

// UpdateAIDescription updates AI description in memory
func (s *MetadataService) UpdateAIDescription(ctx context.Context, connectionID, dbName string, target DescriptionTarget, description string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	metadata, exists := s.metadata[connectionID]
	if !exists {
		return fmt.Errorf("metadata not loaded for connection: %s", connectionID)
	}

	dbMeta, dbExists := metadata.Databases[dbName]
	if !dbExists {
		return fmt.Errorf("database %s not found in metadata", dbName)
	}

	switch target.Type {
	case "database":
		dbMeta.AIDescription = description
		metadata.Databases[dbName] = dbMeta

	case "table":
		found := false
		for i, table := range dbMeta.Tables {
			if table.Name == target.TableName {
				table.AIDescription = description
				dbMeta.Tables[i] = table
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("table %s not found", target.TableName)
		}
		metadata.Databases[dbName] = dbMeta

	case "column":
		found := false
		for i, table := range dbMeta.Tables {
			if table.Name == target.TableName {
				for j, col := range table.Columns {
					if col.Name == target.ColumnName {
						col.AIDescription = description
						table.Columns[j] = col
						found = true
						break
					}
				}
				if found {
					dbMeta.Tables[i] = table
					break
				}
			}
		}
		if !found {
			return fmt.Errorf("column %s not found in table %s", target.ColumnName, target.TableName)
		}
		metadata.Databases[dbName] = dbMeta

	default:
		return fmt.Errorf("invalid target type: %s", target.Type)
	}

	LogInfo("Updated AI description for %s", target.Type)
	return nil
}

// DeleteConnectionMetadata removes metadata from memory and file
func (s *MetadataService) DeleteConnectionMetadata(connectionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.metadata, connectionID)

	filePath := s.getMetadataFilePath(connectionID)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete metadata file: %w", err)
	}

	LogInfo("Deleted metadata for connection: %s", connectionID)
	return nil
}

// Helper methods

func (s *MetadataService) getMetadataFilePath(connectionID string) string {
	fileName := fmt.Sprintf("%s.json", connectionID)
	return filepath.Join(s.metadataDir, fileName)
}

func isSystemDatabase(dbName string) bool {
	systemDBs := map[string]bool{
		"information_schema":  true,
		"performance_schema":  true,
		"metrics_schema":      true,
		"lightning_task_info": true,
		"mysql":               true,
		"sys":                 true,
	}
	return systemDBs[strings.ToLower(dbName)]
}

func (s *MetadataService) extractDatabaseMetadata(ctx context.Context, connDetails ConnectionDetails, dbName string) (*DatabaseMetadata, error) {
	connDetailsCopy := connDetails
	connDetailsCopy.DBName = dbName

	tables, err := s.dbService.ListTables(ctx, connDetailsCopy, dbName)
	if err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}

	dbMetadata := &DatabaseMetadata{
		Name:   dbName,
		Tables: make([]Table, 0, len(tables)),
		Graph:  make(map[string][]Edge),
	}

	// Get database comment
	dbCommentQuery := fmt.Sprintf(`
		SELECT SCHEMA_COMMENT
		FROM information_schema.SCHEMATA
		WHERE SCHEMA_NAME = '%s'`, dbName)

	if result, err := s.dbService.ExecuteSQL(ctx, connDetailsCopy, dbCommentQuery); err == nil && len(result.Rows) > 0 {
		if comment, ok := result.Rows[0]["SCHEMA_COMMENT"].(string); ok && comment != "" {
			dbMetadata.DBComment = comment
		}
	}

	// Extract table metadata
	for _, tableName := range tables {
		table, err := s.extractTableMetadata(ctx, connDetailsCopy, dbName, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to extract table %s: %w", tableName, err)
		}
		dbMetadata.Tables = append(dbMetadata.Tables, *table)

		// Build graph edges from foreign keys
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

	return dbMetadata, nil
}

func (s *MetadataService) extractTableMetadata(ctx context.Context, connDetails ConnectionDetails, dbName, tableName string) (*Table, error) {
	table := &Table{
		Name:        tableName,
		Columns:     make([]Column, 0),
		ForeignKeys: make([]ForeignKey, 0),
		Indexes:     make([]Index, 0),
	}

	// Get table schema
	tableSchema, err := s.dbService.GetTableSchema(ctx, connDetails, dbName, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to get table schema: %w", err)
	}

	// Get table comment
	tableCommentQuery := fmt.Sprintf(`
		SELECT TABLE_COMMENT
		FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = '%s' AND TABLE_NAME = '%s'`, dbName, tableName)

	if result, err := s.dbService.ExecuteSQL(ctx, connDetails, tableCommentQuery); err == nil && len(result.Rows) > 0 {
		if comment, ok := result.Rows[0]["TABLE_COMMENT"].(string); ok && comment != "" {
			table.DBComment = comment
		}
	}

	// Get column comments
	columnCommentsQuery := fmt.Sprintf(`
		SELECT COLUMN_NAME, COLUMN_COMMENT
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = '%s' AND TABLE_NAME = '%s'`, dbName, tableName)

	columnComments := make(map[string]string)
	if result, err := s.dbService.ExecuteSQL(ctx, connDetails, columnCommentsQuery); err == nil {
		for _, row := range result.Rows {
			if colName, ok := row["COLUMN_NAME"].(string); ok {
				if comment, okComment := row["COLUMN_COMMENT"].(string); okComment && comment != "" {
					columnComments[colName] = comment
				}
			}
		}
	}

	// Build columns
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

	// Get foreign keys
	fkQuery := fmt.Sprintf(`
		SELECT CONSTRAINT_NAME, COLUMN_NAME, REFERENCED_TABLE_NAME, REFERENCED_COLUMN_NAME
		FROM information_schema.KEY_COLUMN_USAGE
		WHERE TABLE_SCHEMA = '%s' AND TABLE_NAME = '%s' AND REFERENCED_TABLE_NAME IS NOT NULL`, dbName, tableName)

	if fkResult, err := s.dbService.ExecuteSQL(ctx, connDetails, fkQuery); err == nil && len(fkResult.Rows) > 0 {
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
	}

	// Get indexes
	indexQuery := fmt.Sprintf(`
		SELECT INDEX_NAME, COLUMN_NAME, NON_UNIQUE
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = '%s' AND TABLE_NAME = '%s'
		ORDER BY INDEX_NAME, SEQ_IN_INDEX`, dbName, tableName)

	if indexResult, err := s.dbService.ExecuteSQL(ctx, connDetails, indexQuery); err == nil && len(indexResult.Rows) > 0 {
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
	}

	return table, nil
}

// saveMetadataToFile saves metadata to file without locking (internal helper)
func (s *MetadataService) saveMetadataToFile(metadata *ConnectionMetadata) error {
	filePath := s.getMetadataFilePath(metadata.ConnectionID)
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}

	LogInfo("Saved metadata to file for connection: %s", metadata.ConnectionID)
	return nil
}
