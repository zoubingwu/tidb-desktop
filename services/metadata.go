package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	Name        string       `json:"name"`
	Columns     []Column     `json:"columns"`
	ForeignKeys []ForeignKey `json:"foreignKeys,omitempty"`
	Indexes     []Index      `json:"indexes,omitempty"`
}

// DatabaseMetadata represents the complete metadata for a database
type DatabaseMetadata struct {
	Name           string            `json:"name"`
	Tables         []Table           `json:"tables"`
	LastExtracted  time.Time         `json:"lastExtracted"`
	ConnectionName string            `json:"connectionName"`
	Graph          map[string][]Edge `json:"graph,omitempty"` // Adjacency list representation
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

// getMetadataFilePath returns the path to the metadata file for a given connection and database
func (s *MetadataService) getMetadataFilePath(connectionName, dbName string) string {
	fileName := fmt.Sprintf("%s_%s_metadata.json", connectionName, dbName)
	return filepath.Join(s.metadataDir, fileName)
}

// ExtractMetadata extracts metadata from the database and stores it
func (s *MetadataService) ExtractMetadata(ctx context.Context, connectionName, dbName string) (*DatabaseMetadata, error) {
	// Get connection details
	connDetails, exists, err := s.configService.GetConnection(connectionName)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection details: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("connection %s not found", connectionName)
	}

	// Use the specified database
	connDetails.DBName = dbName

	// Get all tables
	tables, err := s.dbService.ListTables(ctx, connDetails, dbName)
	if err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}

	metadata := &DatabaseMetadata{
		Name:           dbName,
		Tables:         make([]Table, 0, len(tables)),
		LastExtracted:  time.Now(),
		ConnectionName: connectionName,
		Graph:          make(map[string][]Edge),
	}

	// Extract metadata for each table
	for _, tableName := range tables {
		tableSchema, err := s.dbService.GetTableSchema(ctx, connDetails, dbName, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get schema for table %s: %w", tableName, err)
		}

		table := Table{
			Name:        tableName,
			Columns:     make([]Column, 0, len(tableSchema.Columns)),
			ForeignKeys: make([]ForeignKey, 0),
			Indexes:     make([]Index, 0),
		}

		// Extract column information
		for _, col := range tableSchema.Columns {
			column := Column{
				Name:          col.ColumnName,
				DataType:      col.ColumnType,
				IsNullable:    col.IsNullable == "YES",
				AutoIncrement: col.Extra == "auto_increment",
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

		fkResult, err := s.dbService.ExecuteSQL(ctx, connDetails, fkQuery)
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

				// Add to graph representation
				metadata.Graph[tableName] = append(metadata.Graph[tableName], Edge{
					ToTable:    refTableName,
					FromColumn: columnName,
					ToColumn:   refColumnName,
				})
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

		indexResult, err := s.dbService.ExecuteSQL(ctx, connDetails, indexQuery)
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

		metadata.Tables = append(metadata.Tables, table)
	}

	// Store the metadata
	if err := s.storeMetadata(metadata); err != nil {
		return nil, fmt.Errorf("failed to store metadata: %w", err)
	}

	return metadata, nil
}

// GetMetadata retrieves metadata for a database, extracting it if necessary
func (s *MetadataService) GetMetadata(ctx context.Context, connectionName, dbName string) (*DatabaseMetadata, error) {
	// Try to load existing metadata first
	metadata, err := s.loadMetadata(connectionName, dbName)
	if err == nil && metadata != nil {
		// Check if metadata is still fresh (e.g., less than 1 hour old)
		if time.Since(metadata.LastExtracted) < time.Hour {
			return metadata, nil
		}
	}

	// If metadata doesn't exist, is too old, or failed to load, extract it
	return s.ExtractMetadata(ctx, connectionName, dbName)
}

// storeMetadata saves the metadata to a file
func (s *MetadataService) storeMetadata(metadata *DatabaseMetadata) error {
	filePath := s.getMetadataFilePath(metadata.ConnectionName, metadata.Name)

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}

	return nil
}

// loadMetadata loads metadata from a file
func (s *MetadataService) loadMetadata(connectionName, dbName string) (*DatabaseMetadata, error) {
	filePath := s.getMetadataFilePath(connectionName, dbName)

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // File doesn't exist, return nil without error
		}
		return nil, fmt.Errorf("failed to read metadata file: %w", err)
	}

	var metadata DatabaseMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

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