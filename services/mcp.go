package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// MCPService handles MCP server setup and tool registration.
type MCPService struct {
	mcpServer *server.MCPServer
	dbService *DatabaseService // Inject DatabaseService
	// configService *ConfigService // Inject if needed for other tools
}

// NewMCPService creates a new MCP service instance.
func NewMCPService(dbService *DatabaseService /*, configService *ConfigService*/) (*MCPService, error) {
	if dbService == nil {
		fmt.Println("Warning: MCPService initialized with nil DatabaseService. DB tools will fail.")
	}

	// Initialize the MCP server
	s := server.NewMCPServer(
		"TiDB Desktop Agent",
		"0.1.0",
		server.WithResourceCapabilities(true, true),
		server.WithLogging(),
		server.WithRecovery(),
	)

	// --- Example Tool: Add Calculator ---
	calculatorTool := mcp.NewTool("calculate",
		mcp.WithDescription("Perform basic arithmetic operations"),
		mcp.WithString("operation",
			mcp.Required(),
			mcp.Description("The operation to perform (add, subtract, multiply, divide)"),
			mcp.Enum("add", "subtract", "multiply", "divide"),
		),
		mcp.WithNumber("x",
			mcp.Required(),
			mcp.Description("First number"),
		),
		mcp.WithNumber("y",
			mcp.Required(),
			mcp.Description("Second number"),
		),
	)
	calculatorHandler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		op, ok := request.Params.Arguments["operation"].(string)
		if !ok {
			return mcp.NewToolResultError("invalid or missing 'operation' argument"), nil
		}
		x, ok := request.Params.Arguments["x"].(float64)
		if !ok {
			return mcp.NewToolResultError("invalid or missing 'x' argument"), nil
		}
		y, ok := request.Params.Arguments["y"].(float64)
		if !ok {
			return mcp.NewToolResultError("invalid or missing 'y' argument"), nil
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
	s.AddTool(calculatorTool, calculatorHandler)

	// --- Tool: List Tables ---
	listTablesTool := mcp.NewTool("list_tables",
		mcp.WithDescription("Show all tables in a specific database for the active connection."),
		mcp.WithString("database_name",
			mcp.Required(),
			mcp.Description("The name of the database/schema to list tables from."),
		),
	)
	listTablesHandler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if dbService == nil {
			return mcp.NewToolResultError("DatabaseService not available"), nil
		}
		// TODO: Need access to the *active* connection from App struct here.
		dbName, ok := request.Params.Arguments["database_name"].(string)
		if !ok || dbName == "" {
			return mcp.NewToolResultError("missing or invalid 'database_name' argument"), nil
		}

		// Placeholder - needs active connection details
		return mcp.NewToolResultError(fmt.Sprintf("Cannot list tables for '%s': Active connection details unavailable in MCP context.", dbName)), nil
		/*
			// --- Ideal Implementation (requires active connection) ---
			activeConn := getActiveConnectionFromSomewhere(ctx) // Function needed
			if activeConn == nil {
			    return mcp.NewToolResultError("no active database connection"), nil
			}
			query := fmt.Sprintf("SHOW TABLES FROM `%s`;", dbName)
			result, err := dbService.ExecuteSQL(ctx, *activeConn, query)
			if err != nil {
			    return mcp.NewToolResultError(fmt.Sprintf("failed to list tables from '%s': %v", dbName, err)), nil
			}
			tableRows, ok := result.([]map[string]any)
			if !ok {
			    return mcp.NewToolResultError(fmt.Sprintf("unexpected result format listing tables from '%s'", dbName)), nil
			}
			var tableNames []string
			columnKey := ""
			if len(tableRows) > 0 {
			    for k := range tableRows[0] {
			        columnKey = k
			        break
			    }
			}
			for _, row := range tableRows {
			    if name, ok := row[columnKey].(string); ok {
			        tableNames = append(tableNames, name)
			    }
			}
			jsonData, err := json.Marshal(tableNames)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to marshal table names to JSON: %v", err)), nil
			}
			return mcp.NewToolResultJSON(string(jsonData)), nil
		*/
	}
	s.AddTool(listTablesTool, listTablesHandler) // Pass pointer

	// --- Tool: Execute SQL Query ---
	executeQueryTool := mcp.NewTool("execute_query",
		mcp.WithDescription("Execute a SQL SELECT query against a specific database using the active connection. Use 'execute_statement' for INSERT/UPDATE/DELETE etc."),
		mcp.WithString("database_name",
			mcp.Required(),
			mcp.Description("The database context for the query. Can be empty if the query includes the database name."),
		),
		mcp.WithString("sql_query",
			mcp.Required(),
			mcp.Description("The SQL SELECT query string to execute. Should include LIMIT."),
		),
	)
	executeQueryHandler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if dbService == nil {
			return mcp.NewToolResultError("DatabaseService not available"), nil
		}
		// TODO: Needs access to the *active* connection from App struct here.
		dbName, _ := request.Params.Arguments["database_name"].(string) // Optional, might be in query
		sql, ok := request.Params.Arguments["sql_query"].(string)
		if !ok || sql == "" {
			return mcp.NewToolResultError("missing or invalid 'sql_query' argument"), nil
		}

		if !strings.HasPrefix(strings.TrimSpace(strings.ToUpper(sql)), "SELECT") &&
		   !strings.HasPrefix(strings.TrimSpace(strings.ToUpper(sql)), "SHOW") &&
		   !strings.HasPrefix(strings.TrimSpace(strings.ToUpper(sql)), "DESC") &&
		   !strings.HasPrefix(strings.TrimSpace(strings.ToUpper(sql)), "EXPLAIN") {
			return mcp.NewToolResultError("this tool is only for SELECT, SHOW, DESCRIBE, EXPLAIN queries. Use 'execute_statement' for modifications."), nil
		}

		// Placeholder - needs active connection details
		return mcp.NewToolResultError(fmt.Sprintf("Cannot execute query '%s' in db '%s': Active connection details unavailable in MCP context.", sql, dbName)), nil
		/*
			// --- Ideal Implementation (requires active connection) ---
			activeConn := getActiveConnectionFromSomewhere(ctx)
			if activeConn == nil {
			    return mcp.NewToolResultError("no active database connection"), nil
			}
			// Potentially set the DB context if provided and different from connection's default
			connToUse := *activeConn
			if dbName != "" && connToUse.DBName != dbName {
				// Need to execute a 'USE dbName' first, or handle this in dbService
				// Simpler: Assume ExecuteSQL handles the db context if needed, or the query includes it.
				fmt.Printf("Warning: Tool specified db '%s' but connection default is '%s'. Ensure query is qualified or connection allows switching.", dbName, connToUse.DBName)
			}

			result, err := dbService.ExecuteSQL(ctx, connToUse, sql)
			if err != nil {
			    return mcp.NewToolResultError(fmt.Sprintf("failed to execute query '%s': %v", sql, err)), nil
			}

			// Attempt to marshal result to JSON
			jsonData, err := json.MarshalIndent(result, "", "  ") // Pretty print JSON
			if err != nil {
			    // Fallback to simple string representation if JSON fails
			    return mcp.NewToolResultText(fmt.Sprintf("Result (non-JSON): %+v", result)), nil
			}
			return mcp.NewToolResultJSON(string(jsonData)), nil
		*/
	}
	s.AddTool(executeQueryTool, executeQueryHandler) // Pass pointer

	// --- Tool: Show Create Table ---
	showCreateTableTool := mcp.NewTool("show_create_table",
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
	showCreateTableHandler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if dbService == nil {
			return mcp.NewToolResultError("DatabaseService not available"), nil
		}
		// TODO: Needs access to the *active* connection from App struct here.
		dbName, ok := request.Params.Arguments["database_name"].(string)
		if !ok || dbName == "" {
			return mcp.NewToolResultError("missing or invalid 'database_name' argument"), nil
		}
		tblName, ok := request.Params.Arguments["table_name"].(string)
		if !ok || tblName == "" {
			return mcp.NewToolResultError("missing or invalid 'table_name' argument"), nil
		}

		// Placeholder - needs active connection details
		return mcp.NewToolResultError(fmt.Sprintf("Cannot show create table for '%s.%s': Active connection details unavailable in MCP context.", dbName, tblName)), nil
		/*
			// --- Ideal Implementation (requires active connection) ---
			activeConn := getActiveConnectionFromSomewhere(ctx)
			if activeConn == nil {
			    return mcp.NewToolResultError("no active database connection"), nil
			}
			query := fmt.Sprintf("SHOW CREATE TABLE `%s`.`%s`;", dbName, tblName)
			result, err := dbService.ExecuteSQL(ctx, *activeConn, query)
			if err != nil {
			    return mcp.NewToolResultError(fmt.Sprintf("failed to get create table for '%s.%s': %v", dbName, tblName, err)), nil
			}

			// Expect result like: [map[string]any{"Table": "tblName", "Create Table": "CREATE TABLE ..."}]
			rows, ok := result.([]map[string]any)
			if !ok || len(rows) != 1 {
			    return mcp.NewToolResultError(fmt.Sprintf("unexpected result format for SHOW CREATE TABLE '%s.%s'", dbName, tblName)), nil
			}
			createStmt, ok := rows[0]["Create Table"].(string) // Key might vary slightly by DB driver
			if !ok {
				// Try other potential keys if necessary based on testing
				createStmt, ok = rows[0]["CREATE TABLE"].(string)
			}
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("could not extract create statement from result for '%s.%s'", dbName, tblName)), nil
			}
			return mcp.NewToolResultText(createStmt), nil
		*/
	}
	s.AddTool(showCreateTableTool, showCreateTableHandler) // Pass pointer

	// TODO: Add 'execute_statement' tool for INSERT/UPDATE/DELETE/CREATE/DROP etc.
	// TODO: Add tools for user management if feasible/secure.
	// TODO: Add tool to get connection info if needed.

	return &MCPService{
		mcpServer: s,
		dbService: dbService,
	}, nil
}

// Start runs the MCP server, typically blocking until completion or error.
// It uses Stdio for communication as per the example.
func (s *MCPService) Start() error {
	fmt.Println("Starting MCP Server via Stdio...")
	if err := server.ServeStdio(s.mcpServer); err != nil {
		fmt.Printf("MCP Server error: %v\n", err)
		return fmt.Errorf("MCP server failed: %w", err)
	}
	fmt.Println("MCP Server finished.")
	return nil
}

// TODO: Add methods to define tools that interact with other services (ConfigService, DatabaseService)
// Example: A tool to list saved connections would need access to ConfigService.
// NewMCPService might need to accept other services as arguments.
