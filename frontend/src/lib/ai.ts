import { generateText, tool } from "ai";
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

// Use a model known to work well with tools and reasoning
// Consider gpt-4o or claude-3.5-sonnet if gemini-pro struggles
// const model = openrouter.chat("anthropic/claude-3.5-sonnet");
const model = openrouter.chat("google/gemini-2.5-pro-exp-03-25:free");

const dbTools = {
  listDatabases: tool({
    description:
      "List all available databases/schemas in the connected instance. Useful when the user's request isn't specific to the current database.",
    parameters: z.object({}),
    execute: async () => {
      try {
        console.log("Tool Call: listDatabases");
        const dbs = await ListDatabases();
        console.log("Tool Result: listDatabases ->", dbs);
        return { success: true, databases: dbs };
      } catch (error: any) {
        console.error("Error calling ListDatabases:", error);
        return { success: false, error: error.message };
      }
    },
  }),
  listTables: tool({
    description:
      "List all tables within a specific database. Use this if you need to know the tables in a database relevant to the user's request.",
    parameters: z.object({
      dbName: z
        .string()
        .describe("The name of the database for which to list tables."),
    }),
    execute: async ({ dbName }) => {
      try {
        console.log(`Tool Call: listTables (dbName: ${dbName})`);
        const tables = await ListTables(dbName);
        console.log("Tool Result: listTables ->", tables);
        return { success: true, tables: tables };
      } catch (error: any) {
        console.error(`Error calling ListTables for ${dbName}:`, error);
        return { success: false, error: error.message };
      }
    },
  }),
  getTableSchema: tool({
    description:
      "Get the detailed schema (column names, types, constraints, etc.) for a specific table. Essential for constructing queries involving specific columns or understanding table structure.",
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
        console.log(
          `Tool Call: getTableSchema (dbName: ${dbName}, tableName: ${tableName})`,
        );
        const schema: TableSchema = await GetTableSchema(dbName, tableName);
        console.log("Tool Result: getTableSchema ->", schema);
        return {
          success: true,
          databaseName: dbName, // Include db name for clarity
          tableName: schema.name,
          columns: schema.columns,
        };
      } catch (error: any) {
        console.error(
          `Error calling GetTableSchema for ${dbName}.${tableName}:`,
          error,
        );
        return { success: false, error: error.message };
      }
    },
  }),
  executeSql: tool({
    description:
      "Executes a *read-only* SQL query (primarily SELECT) to fetch sample data or check existence, helping to understand data patterns or confirm assumptions before generating the final query. Use LIMIT clauses (e.g., LIMIT 5) to keep results small. **DO NOT use this for INSERT, UPDATE, DELETE, or other modifying operations.**",
    parameters: z.object({
      query: z
        .string()
        .describe(
          "The *safe*, *read-only* SQL SELECT query string to execute (e.g., `SELECT * FROM users LIMIT 3`).",
        ),
    }),
    execute: async ({ query }) => {
      // Add extra safety check here
      if (!query.trim().toUpperCase().startsWith("SELECT")) {
        console.warn(
          `Tool Call Refused: executeSql called with non-SELECT query: ${query}`,
        );
        return {
          success: false,
          error: "This tool can only execute SELECT queries.",
        };
      }
      try {
        console.log(`Tool Call: executeSql (Query: ${query})`);
        const result = await ExecuteSQL(query);
        console.log("Tool Result: executeSql ->", result);
        // Potentially truncate large results before returning to LLM
        const previewResult = JSON.stringify(result).slice(0, 1000); // Limit context size
        return { success: true, resultPreview: previewResult };
      } catch (error: any) {
        console.error(`Error executing safe SQL query \"${query}\":`, error);
        const errorMessage = error.message || "Unknown execution error";
        return { success: false, error: errorMessage };
      }
    },
  }),
};

// --- Zod Schema for the Agent's Final Answer ---
const sqlAgentResponseSchema = z.object({
  query: z
    .string()
    .describe(
      "The generated SQL query. Can be empty if the request is unsafe, ambiguous, or cannot be translated.",
    ),
  explanation: z
    .string()
    .describe(
      "A brief explanation of the generated query, the reasoning process, or why a query couldn't be generated (e.g., safety concerns, ambiguity).",
    ),
  requiresConfirmation: z
    .boolean()
    .describe(
      "Set to true if the generated query performs potentially destructive actions (UPDATE, DELETE, INSERT, ALTER, DROP) or complex SELECTs that might have unintended consequences. Always true for non-SELECT queries.",
    ),
  type: z
    .enum([
      "SELECT",
      "UPDATE",
      "DELETE",
      "INSERT",
      "ALTER",
      "DROP",
      "OTHER",
      "NONE",
    ])
    .describe(
      "The type of the generated SQL query. 'NONE' if no query was generated.",
    ),
});

// --- Define the return type based on the Zod schema ---
export type SqlAgentResponse = z.infer<typeof sqlAgentResponseSchema>;

// --- The "Answer" Tool ---
// This tool doesn't execute anything; its purpose is to structure the final output.
const finalAnswerTool = tool({
  description:
    "Use this tool *only* as the final step to provide the generated SQL query and related information.",
  parameters: sqlAgentResponseSchema,
  // No execute function means calling this tool terminates the agent's run.
});

// --- The New Agent Function ---
export const generateSqlAgent = async (
  userPrompt: string,
  currentDbName?: string | null,
  currentTableName?: string | null,
): Promise<SqlAgentResponse> => {
  console.log(
    `Starting generateSqlAgent for prompt: "${userPrompt}" (DB: ${currentDbName}, Table: ${currentTableName})`,
  );

  // Combine dbTools and the final answer tool
  const agentTools = {
    ...dbTools,
    provideFinalAnswer: finalAnswerTool,
  };

  // --- System Prompt for the Agent ---
  const systemPrompt = `
You are an expert AI database assistant. Your goal is to translate the user's natural language request into a precise SQL query (compatible with MySQL/TiDB).

**Act as an agent:** Reason step-by-step. Use the available tools sequentially to gather necessary context *before* formulating the final query.

**Context:**
- Current Database: ${currentDbName || "Not specified"}
- Current Table: ${currentTableName || "Not specified"}
- User Request: "${userPrompt}"

**Workflow:**
1.  **Analyze Request:** Understand the user's intent. Identify potential target databases, tables, and columns.
2.  **Gather Context (Use Tools):**
    *   If the database or table is unclear or not the current one, use \`listDatabases\` or \`listTables\`.
    *   To understand table structure (columns, types), use \`getTableSchema\` for *all* relevant tables (especially for JOINs).
    *   Optionally, use \`executeSql\` with a *safe, limited SELECT query* (e.g., \`SELECT col FROM tbl LIMIT 3\`) ONLY if you need to understand data formats or relationships better. **Never use \`executeSql\` for modifying data.**
3.  **Formulate Query:** Based on the request and gathered context, construct the SQL query. Use backticks (\\\`) for identifiers and single quotes ('') for string literals.
4.  **Assess Safety & Type:** Determine the query type (SELECT, UPDATE, DELETE, INSERT, ALTER, DROP, OTHER). Set \`requiresConfirmation\` to \`true\` for ALL non-SELECT queries or complex SELECTs.
5.  **Handle Ambiguity/Risk:** If the request is unclear, ambiguous, or potentially very dangerous (e.g., "delete everything", UPDATE/DELETE without clear WHERE clause), DO NOT generate the query. Instead, provide an explanation and set query to "" and type to "NONE".
6.  **Final Output:** Use the \`provideFinalAnswer\` tool *only once* at the very end to return the result, including the query, explanation, confirmation flag, and type.

**Example Interaction (Simplified):**
User: "Show me the first 5 customers from California in the users table"
Agent Steps:
- Thought: Need schema for 'users' table. Assume current DB.
- Call \`getTableSchema\` for 'users'.
- Result: Columns are 'id', 'name', 'state', 'signup_date'.
- Thought: Formulate SELECT query.
- Call \`provideFinalAnswer\` with: { query: "SELECT * FROM \\\`users\\\` WHERE \\\`state\\\` = 'California' LIMIT 5;", explanation: "...", requiresConfirmation: false, type: "SELECT" }

**Respond ONLY by calling the \`provideFinalAnswer\` tool.**
`.trim();

  try {
    // --- Run the Agent ---
    const { toolCalls, finishReason, text, usage } = await generateText({
      model,
      system: systemPrompt,
      prompt: userPrompt,
      tools: agentTools,
      toolChoice: "required", // Force the final response into the answer tool
      maxSteps: 8, // Allow multiple tool calls (adjust as needed)
      // Optional: Callback for observing steps
      onStepFinish: ({ text, toolCalls, toolResults, finishReason, usage }) => {
        console.log("--- Step Finished ---");
        console.log("Finish Reason:", finishReason);
        if (text) console.log("Text:", text);
        if (toolCalls)
          console.log("Tool Calls:", JSON.stringify(toolCalls, null, 2));
        if (toolResults)
          console.log("Tool Results:", JSON.stringify(toolResults, null, 2));
        console.log("Usage:", usage);
        console.log("-------------------------");
      },
    });

    console.log("Agent finished. Reason:", finishReason);
    console.log("Final tool calls:", JSON.stringify(toolCalls, null, 2));
    console.log("Usage:", usage);

    // Extract the arguments from the final 'provideFinalAnswer' tool call
    const finalCall = toolCalls?.find(
      (call) => call.toolName === "provideFinalAnswer",
    );

    if (finalCall && finalCall.type === "tool-call") {
      // Validate the arguments against the schema
      const validationResult = sqlAgentResponseSchema.safeParse(finalCall.args);
      if (validationResult.success) {
        console.log("Final Answer Parsed:", validationResult.data);
        return validationResult.data;
      } else {
        console.error(
          "Agent Error: Final answer tool arguments failed validation:",
          validationResult.error,
        );
        // Fallback response if validation fails
        return {
          query: "",
          explanation:
            "Error: AI agent failed to provide a valid final answer structure. Validation failed: " +
            validationResult.error.message,
          requiresConfirmation: true,
          type: "NONE",
        };
      }
    } else {
      console.error(
        "Agent Error: Did not receive the expected 'provideFinalAnswer' tool call.",
      );
      // Fallback response if the expected tool call is missing
      const fallbackExplanation =
        "Error: AI agent did not conclude with the expected final answer structure.";
      // Attempt to extract any text generated as a potential explanation
      const rawText = typeof text === "string" ? text : "";
      return {
        query: "",
        explanation: rawText
          ? `${fallbackExplanation}\nAgent raw text output: ${rawText}`
          : fallbackExplanation,
        requiresConfirmation: true,
        type: "NONE",
      };
    }
  } catch (error: any) {
    console.error("Error during generateSqlAgent execution:", error);
    return {
      query: "",
      explanation: `An unexpected error occurred: ${error.message}`,
      requiresConfirmation: true,
      type: "NONE",
    };
  }
};

// --- Example Usage (replace with your actual call) ---
// async function testAgent() {
//   const db = 'testDB';
//   const table = 'users';
//   const prompt = `update the user with id 5 set their status to 'inactive'`;
//   // const prompt = "list all tables in the 'information_schema' database";
//   // const prompt = "show me 3 products with price > 100 order by name";
//   // const prompt = "drop the customers table"; // Example of a dangerous request
//   // const prompt = "what tables are there?"; // Example requiring tool use

//   const result = await generateSqlAgent(prompt, db, table);
//   console.log('\n--- FINAL AGENT RESULT ---');
//   console.log(JSON.stringify(result, null, 2));
//   console.log('--------------------------');
// }

// testAgent(); // Uncomment to run a test
