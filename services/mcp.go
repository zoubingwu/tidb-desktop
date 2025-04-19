package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// MCPService handles MCP server setup and tool registration.
type MCPService struct {
	mcpServer        *server.MCPServer
	dbService        *DatabaseService
	activeConnection *ConnectionDetails
	mu               sync.Mutex
}

// NewMCPService creates a new MCP service instance and registers tools.
func NewMCPService(dbService *DatabaseService) (*MCPService, error) {
	if dbService == nil {
		log.Println("Warning: MCPService initialized with nil DatabaseService. DB tools will fail.")
	}

	s := server.NewMCPServer(
		"TiDB Desktop Agent",
		"0.1.0",
		server.WithResourceCapabilities(true, true),
		server.WithLogging(),
		server.WithRecovery(),
	)

	mcpSvc := &MCPService{
		mcpServer: s,
		dbService: dbService,
	}

	// Register all tools
	addListTablesTool(mcpSvc, s)
	addExecuteQueryTool(mcpSvc, s)
	addExecuteStatementTool(mcpSvc, s)
	addShowCreateTableTool(mcpSvc, s)
	addGetConnectionInfoTool(mcpSvc, s)
	addCalculatorTool(mcpSvc, s) // Keep example tool

	return mcpSvc, nil
}


func addListTablesTool(mcpSvc *MCPService, s *server.MCPServer) {
	tool := mcp.NewTool("list_tables",
		mcp.WithDescription("Show all tables in a specific database for the active connection."),
		mcp.WithString("database_name",
			mcp.Required(),
			mcp.Description("The name of the database/schema to list tables from."),
		),
	)
	handler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if mcpSvc.dbService == nil {
			return mcp.NewToolResultError("DatabaseService not available"), nil
		}
		activeConn := mcpSvc.getActiveConnection()
		if activeConn == nil {
			return mcp.NewToolResultError("No active database connection established."), nil
		}

		dbName, ok := request.Params.Arguments["database_name"].(string)
		if !ok || dbName == "" {
			return mcp.NewToolResultError("missing or invalid 'database_name' argument"), nil
		}

		tableNames, err := mcpSvc.dbService.ListTables(ctx, *activeConn, dbName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to list tables from '%s': %v", dbName, err)), nil
		}

		jsonData, err := json.Marshal(tableNames)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to marshal table names to JSON: %v", err)), nil
		}
		return mcp.NewToolResultText(string(jsonData)), nil
	}
	s.AddTool(tool, handler)
}

func addExecuteQueryTool(mcpSvc *MCPService, s *server.MCPServer) {
	tool := mcp.NewTool("execute_query",
		mcp.WithDescription("Execute a SQL SELECT, SHOW, DESCRIBE, or EXPLAIN query against a specific database using the active connection. Use 'execute_statement' for INSERT/UPDATE/DELETE etc."),
		mcp.WithString("database_name",
			mcp.Required(),
			mcp.Description("The database context for the query. If empty, uses the connection's default."),
		),
		mcp.WithString("sql_query",
			mcp.Required(),
			mcp.Description("The SQL SELECT/SHOW/DESCRIBE/EXPLAIN query string to execute. Should ideally include LIMIT."),
		),
	)
	handler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if mcpSvc.dbService == nil {
			return mcp.NewToolResultError("DatabaseService not available"), nil
		}
		activeConn := mcpSvc.getActiveConnection()
		if activeConn == nil {
			return mcp.NewToolResultError("No active database connection established."), nil
		}

		dbName, _ := request.Params.Arguments["database_name"].(string)
		sql, ok := request.Params.Arguments["sql_query"].(string)
		if !ok || sql == "" {
			return mcp.NewToolResultError("missing or invalid 'sql_query' argument"), nil
		}

		upperSQL := strings.TrimSpace(strings.ToUpper(sql))
		if !strings.HasPrefix(upperSQL, "SELECT") &&
			!strings.HasPrefix(upperSQL, "SHOW") &&
			!strings.HasPrefix(upperSQL, "DESC") &&
			!strings.HasPrefix(upperSQL, "EXPLAIN") {
			return mcp.NewToolResultError("this tool is only for SELECT, SHOW, DESCRIBE, EXPLAIN queries. Use 'execute_statement' for modifications."), nil
		}

		connToUse := *activeConn
		if dbName != "" {
			log.Printf("MCP execute_query: Targeting database '%s' (query should be qualified or connection default matches)", dbName)
		}

		result, err := mcpSvc.dbService.ExecuteSQL(ctx, connToUse, sql)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to execute query '%s': %v", sql, err)), nil
		}

		jsonData, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			log.Printf("MCP execute_query: Failed to marshal result to JSON (%v), returning text.", err)
			return mcp.NewToolResultText(fmt.Sprintf("Result (non-JSON): %+v", result)), nil
		}
		return mcp.NewToolResultText(string(jsonData)), nil
	}
	s.AddTool(tool, handler)
}

func addExecuteStatementTool(mcpSvc *MCPService, s *server.MCPServer) {
	tool := mcp.NewTool("execute_statement",
		mcp.WithDescription("Execute a single SQL non-query statement (INSERT, UPDATE, DELETE, CREATE, DROP, etc.) against a specific database using the active connection. Use 'execute_query' for SELECT/SHOW etc."),
		mcp.WithString("database_name",
			mcp.Required(),
			mcp.Description("The database context for the statement. If empty, uses the connection's default."),
		),
		mcp.WithString("sql_statement",
			mcp.Required(),
			mcp.Description("A single SQL statement string to execute."),
		),
	)
	handler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if mcpSvc.dbService == nil {
			return mcp.NewToolResultError("DatabaseService not available"), nil
		}
		activeConn := mcpSvc.getActiveConnection()
		if activeConn == nil {
			return mcp.NewToolResultError("No active database connection established."), nil
		}

		dbName, _ := request.Params.Arguments["database_name"].(string)
		sql, ok := request.Params.Arguments["sql_statement"].(string)
		if !ok || sql == "" {
			return mcp.NewToolResultError("missing or invalid 'sql_statement' argument"), nil
		}

		connToUse := *activeConn
		if dbName != "" {
			log.Printf("MCP execute_statement: Targeting database '%s' (statement should be qualified or connection default matches)", dbName)
		}

		upperSQL := strings.TrimSpace(strings.ToUpper(sql))
		if strings.HasPrefix(upperSQL, "SELECT") || strings.HasPrefix(upperSQL, "SHOW") || strings.HasPrefix(upperSQL, "DESC") || strings.HasPrefix(upperSQL, "EXPLAIN") {
			return mcp.NewToolResultError("Use execute_query tool for SELECT/SHOW/DESCRIBE/EXPLAIN statements."), nil
		}

		result, err := mcpSvc.dbService.ExecuteSQL(ctx, connToUse, sql)
		if err != nil {
			log.Printf("MCP execute_statement: Error executing statement (%s): %v", sql, err)
			return mcp.NewToolResultError(fmt.Sprintf("Error executing statement: %v", err)), nil
		}

		if resultMap, ok := result.(map[string]any); ok {
			jsonData, err := json.MarshalIndent(resultMap, "", "  ")
			if err != nil {
				log.Printf("MCP execute_statement: Failed to marshal result map: %v", err)
				return mcp.NewToolResultText(fmt.Sprintf("Execution Result (non-JSON): %+v", resultMap)), nil
			}
			return mcp.NewToolResultText(string(jsonData)), nil
		} else {
			log.Printf("MCP execute_statement: Unexpected result type for statement (%s): %T", sql, result)
			return mcp.NewToolResultError(fmt.Sprintf("Unexpected result format after execution: %T", result)), nil
		}
	}
	s.AddTool(tool, handler)
}

func addShowCreateTableTool(mcpSvc *MCPService, s *server.MCPServer) {
	tool := mcp.NewTool("show_create_table",
		mcp.WithDescription("Show the CREATE TABLE statement for a table in a specific database using the active connection."),
		mcp.WithString("database_name",
			mcp.Required(),
			mcp.Description("The name of the database/schema containing the table."),
		),
		mcp.WithString("table_name",
			mcp.Required(),
			mcp.Description("The name of the table."),
		),
	)
	handler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if mcpSvc.dbService == nil {
			return mcp.NewToolResultError("DatabaseService not available"), nil
		}
		activeConn := mcpSvc.getActiveConnection()
		if activeConn == nil {
			return mcp.NewToolResultError("No active database connection established."), nil
		}

		dbName, ok := request.Params.Arguments["database_name"].(string)
		if !ok || dbName == "" {
			return mcp.NewToolResultError("missing or invalid 'database_name' argument"), nil
		}
		tblName, ok := request.Params.Arguments["table_name"].(string)
		if !ok || tblName == "" {
			return mcp.NewToolResultError("missing or invalid 'table_name' argument"), nil
		}

		connToUse := *activeConn
		query := fmt.Sprintf("SHOW CREATE TABLE `%s`.`%s`;", dbName, tblName)

		result, err := mcpSvc.dbService.ExecuteSQL(ctx, connToUse, query)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get create table for '%s.%s': %v", dbName, tblName, err)), nil
		}

		rows, ok := result.([]map[string]any)
		if !ok || len(rows) != 1 {
			log.Printf("MCP show_create_table: Unexpected result format: %T, Len: %d", result, len(rows))
			return mcp.NewToolResultError(fmt.Sprintf("unexpected result format for SHOW CREATE TABLE '%s.%s'", dbName, tblName)), nil
		}

		createStmt, ok := rows[0]["Create Table"].(string)
		if !ok {
			for k, v := range rows[0] {
				if strings.EqualFold(k, "Create Table") {
					if stmt, okConv := v.(string); okConv {
						createStmt = stmt
						ok = true
						break
					}
				}
			}
		}

		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("could not extract create statement from result for '%s.%s'", dbName, tblName)), nil
		}
		return mcp.NewToolResultText(createStmt), nil
	}
	s.AddTool(tool, handler)
}

func addGetConnectionInfoTool(mcpSvc *MCPService, s *server.MCPServer) {
	tool := mcp.NewTool("get_connection_info",
		mcp.WithDescription("Get details about the current active database connection (host, port, user, database)."),
	)
	handler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		activeConn := mcpSvc.getActiveConnection()
		if activeConn == nil {
			return mcp.NewToolResultError("No active database connection established."), nil
		}

		info := map[string]string{
			"host":     activeConn.Host,
			"port":     activeConn.Port,
			"user":     activeConn.User,
			"database": activeConn.DBName,
			"useTLS":   fmt.Sprintf("%t", activeConn.UseTLS),
		}
		jsonData, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to marshal connection info to JSON: %v", err)), nil
		}
		return mcp.NewToolResultText(string(jsonData)), nil
	}
	s.AddTool(tool, handler)
}

func addCalculatorTool(mcpSvc *MCPService, s *server.MCPServer) {
	tool := mcp.NewTool("calculate",
		mcp.WithDescription("Perform basic arithmetic operations"),
		mcp.WithString("operation",
			mcp.Required(),
			mcp.Description("The operation to perform (add, subtract, multiply, divide)"),
			mcp.Enum("add", "subtract", "multiply", "divide"),
		),
		mcp.WithNumber("x", mcp.Required(), mcp.Description("First number")),
		mcp.WithNumber("y", mcp.Required(), mcp.Description("Second number")),
	)
	handler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		op, _ := request.Params.Arguments["operation"].(string)
		x, xOK := request.Params.Arguments["x"].(float64)
		y, yOK := request.Params.Arguments["y"].(float64)
		if !xOK || !yOK {
			return mcp.NewToolResultError("Invalid number arguments"), nil
		}
		var result float64
		switch op {
		case "add":
			result = x + y
		case "subtract":
			result = x - y
		case "multiply":
			result = x * y
		case "divide":
			if y == 0 {
				return mcp.NewToolResultError("cannot divide by zero"), nil
			}
			result = x / y
		default:
			return mcp.NewToolResultError(fmt.Sprintf("unknown operation: %s", op)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("%.2f", result)), nil
	}
	s.AddTool(tool, handler)
}

// --- Service Lifecycle Methods ---

// SetActiveConnection safely updates the active connection details for the service.
func (s *MCPService) SetActiveConnection(details *ConnectionDetails) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeConnection = details
	if details != nil {
		log.Printf("MCPService: Active connection set to: %s@%s:%s/%s", details.User, details.Host, details.Port, details.DBName)
	} else {
		log.Printf("MCPService: Active connection cleared.")
	}
}

// getActiveConnection safely retrieves the current active connection.
func (s *MCPService) getActiveConnection() *ConnectionDetails {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeConnection == nil {
		return nil
	}
	detailsCopy := *s.activeConnection
	return &detailsCopy
}

// Start runs the MCP server, typically blocking until completion or error.
func (s *MCPService) Start() error {
	log.Println("Starting MCP Server via Stdio...")
	if err := server.ServeStdio(s.mcpServer); err != nil {
		log.Printf("MCP Server error: %v\n", err)
		return fmt.Errorf("MCP server failed: %w", err)
	}
	log.Println("MCP Server finished.")
	return nil
}
