import { generateObject, generateText, tool } from "ai";
import { createOpenRouter } from "@openrouter/ai-sdk-provider";
import { z } from "zod";
import {
  ListDatabases,
  ListTables,
  GetTableSchema,
  ExecuteSQL,
} from "wailsjs/go/main/App";
import { services } from "wailsjs/go/models";

type TableSchema = services.TableSchema;

const openrouter = createOpenRouter({
  apiKey: import.meta.env.VITE_OPENROUTER_API_KEY,
});

// Use a model known to work well with tools, adjust if needed
const model = openrouter.chat("google/gemini-2.5-pro-exp-03-25:free");

const dbTools = {
  listDatabases: tool({
    description:
      "List all available databases/schemas in the connected instance.",
    parameters: z.object({}), // No parameters needed
    execute: async () => {
      try {
        const dbs = await ListDatabases();
        return { success: true, databases: dbs };
      } catch (error: any) {
        console.error("Error calling ListDatabases:", error);
        return { success: false, error: error.message };
      }
    },
  }),
  listTables: tool({
    description: "List all tables within a specific database.",
    parameters: z.object({
      dbName: z
        .string()
        .describe("The name of the database for which to list tables."),
    }),
    execute: async ({ dbName }) => {
      try {
        const tables = await ListTables(dbName);
        return { success: true, tables: tables };
      } catch (error: any) {
        console.error("Error calling ListTables:", error);
        return { success: false, error: error.message };
      }
    },
  }),
  getTableSchema: tool({
    description:
      "Get the detailed schema (column names, types, etc.) for a specific table in a specific database.",
    parameters: z.object({
      dbName: z
        .string()
        .describe("The name of the database containing the table."),
      tableName: z
        .string()
        .describe("The name of the table for which to get the schema."),
    }),
    execute: async ({ dbName, tableName }) => {
      try {
        const schema: TableSchema = await GetTableSchema(dbName, tableName);

        return {
          success: true,
          tableName: schema.name,
          columns: schema.columns,
        };
      } catch (error: any) {
        console.error("Error calling GetTableSchema:", error);
        return { success: false, error: error.message };
      }
    },
  }),
  executeSql: tool({
    description:
      "Executes a given SQL query against the currently active database connection. Use this for SELECT queries to fetch data or potentially for other SQL commands IF the user explicitly confirms the action. Be very careful with non-SELECT statements.",
    parameters: z.object({
      query: z.string().describe("The SQL query string to execute."),
    }),
    execute: async ({ query }) => {
      try {
        // WARNING: Executing AI-generated SQL directly is risky.
        // User confirmation is assumed to happen elsewhere.
        const result = await ExecuteSQL(query);
        return { success: true, result: result };
      } catch (error: any) {
        console.error(`Error executing SQL query \"${query}\":`, error);
        // Attempt to return a structured error if possible
        const errorMessage = error.message || "Unknown execution error";
        return { success: false, error: errorMessage };
      }
    },
  }),
};

export const inferConnectionDetails = async (textFromClipboard: string) => {
  const { object } = await generateObject({
    model,
    prompt: `
    Analyze the following text and extract database connection details. Respond ONLY with a JSON object containing the keys "host", "port", "user", "password", "dbName", and "useTLS" (boolean, true if TLS/SSL is mentioned or implied or it is tidbcloud.com, otherwise false). If a value is not found, use an empty string "" for string fields or false for the boolean.

    Input Text:
    """
    ${textFromClipboard}
    """

    JSON Output:
    `.trim(),
    schema: z.object({
      host: z.string(),
      port: z.string(),
      user: z.string(),
      password: z.string(),
      dbName: z.string(),
      useTLS: z.boolean(),
    }),
  });

  return object;
};

// Updated function to generate WHERE clause using tools if needed
export const filterTableData = async (
  userPrompt: string,
  currentDbName: string | null, // Pass current DB context if available
  currentTableName: string | null, // Pass current Table context if available
): Promise<string> => {
  // Return the where clause string

  // --- Prompt for generateText ---
  let prompt = `
You are an AI assistant that helps users filter data in a database table by generating SQL WHERE clause conditions.
The user is asking to filter data. Their request is: "${userPrompt}"

Current context:
- Connected Database: ${currentDbName || "Not specified (Use listDatabases if needed)"}
- Current Table: ${currentTableName || "Not specified (Use listTables and getTableSchema if needed)"}

Your goal is to generate ONLY the SQL conditions (the part that comes *after* WHERE) based on the user's request.

Instructions:
1.  Analyze the user's request: "${userPrompt}".
2.  If the current table context (${currentTableName}) is missing or unclear from the request, use the 'listTables' and 'getTableSchema' tools to identify the relevant table and its columns. You might need 'listDatabases' first if the database context (${currentDbName}) is also unclear.
3.  Once you have the relevant table schema (column names and types), translate the user's filtering request into SQL conditions.
4.  Use standard SQL syntax compatible with MySQL/TiDB.
5.  Use backticks around column names (e.g., \\\`user_id\\\`).
6.  Use single quotes for string literals (e.g., 'active').
7.  Combine multiple conditions using 'AND' by default unless the user specifies 'OR'.
8.  If the request is unclear, ambiguous, cannot be translated to SQL conditions for the identified columns, or seems unsafe (e.g., requests data modification or involves system tables), respond with ONLY the text: "QUERY_UNSAFE_OR_UNCLEAR".
9.  After using any necessary tools and formulating the conditions, respond with ONLY the generated SQL WHERE clause content (without the WHERE keyword). If no filtering is needed or possible, respond with an empty string "".

Example User Request: "show users created this year with status active"
Example Final AI Response (after potentially using getTableSchema): \\\`creation_date\\\` >= '2024-01-01' AND \\\`status\\\` = 'active'

Example User Request: "find products where the price is more than 100 or the stock is zero"
Example Final AI Response: \\\`price\\\` > 100 OR \\\`stock_quantity\\\` = 0

Example User Request: "delete all users"
Example Final AI Response: QUERY_UNSAFE_OR_UNCLEAR

User request: "${userPrompt}"
Begin analysis and tool use if necessary. Finally, provide ONLY the SQL WHERE clause content or "QUERY_UNSAFE_OR_UNCLEAR".
  `.trim();

  // --- Generate Text with Tools ---
  const { text, toolResults } = await generateText({
    model,
    tools: dbTools,
    prompt: prompt,
    maxSteps: 5,
  });

  console.log("AI Response Text:", text);
  console.log("Tool Results:", toolResults);

  // Basic validation, return empty string if AI indicated issues or response is empty
  if (text === "QUERY_UNSAFE_OR_UNCLEAR" || text.trim() === "") {
    return "";
  }

  // Return the generated text, assuming it's the WHERE clause content
  return text.trim();
};
