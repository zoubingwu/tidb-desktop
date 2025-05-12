import { createAnthropic } from "@ai-sdk/anthropic";
import { createOpenAI } from "@ai-sdk/openai";
import { createOpenRouter } from "@openrouter/ai-sdk-provider";
import {
  type CoreMessage,
  type ToolCallPart,
  type ToolResultPart,
  generateObject,
  generateText,
  streamText,
  tool,
} from "ai";
import {
  ExecuteSQL,
  GetAIProviderSettings,
  GetTableSchema,
  ListDatabases,
  ListTables,
} from "wailsjs/go/main/App";
import { services } from "wailsjs/go/models";
import { z } from "zod";

type TableSchema = services.TableSchema;

const createModel = async (options?: TestProviderConnectionOptions) => {
  const aiProviderSettings = await GetAIProviderSettings();
  const provider = options?.provider || aiProviderSettings.provider;

  if (provider === "openai") {
    const openai = createOpenAI({
      apiKey: options?.apiKey || aiProviderSettings.openai?.apiKey,
      baseURL: options?.baseURL || aiProviderSettings.openai?.baseURL,
    });
    return openai.chat("gpt-4o");
  }

  if (provider === "anthropic") {
    const anthropic = createAnthropic({
      apiKey: options?.apiKey || aiProviderSettings.anthropic?.apiKey,
      baseURL: options?.baseURL || aiProviderSettings.anthropic?.baseURL,
    });
    return anthropic.languageModel("claude-3-5-sonnet-latest");
  }

  if (provider === "openrouter") {
    const openrouter = createOpenRouter({
      apiKey: options?.apiKey || aiProviderSettings.openrouter?.apiKey,
    });
    return openrouter.chat("anthropic/claude-3.5-sonnet");
  }

  throw new Error("No AI provider selected");
};

type TestProviderConnectionOptions = {
  provider?: services.AIProviderSettings["provider"];
  apiKey?: string;
  baseURL?: string;
};

export const testProviderConnection = async (
  options?: TestProviderConnectionOptions,
) => {
  const model = await createModel(options);
  try {
    const { text } = await generateText({
      model,
      prompt: "Hello!",
    });

    return { success: true, message: text };
  } catch (error: any) {
    console.log("Error testing provider connection:", error);
    return { success: false, error: error.message };
  }
};

export const inferConnectionDetails = async (textFromClipboard: string) => {
  const model = await createModel();
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
        // Yielding intermediate tool results might be noisy, consider if needed
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
  success: z
    .boolean()
    .describe("True if the query was generated successfully."),
});

// --- Define the return type based on the Zod schema ---
export type SqlAgentResponse = z.infer<typeof sqlAgentResponseSchema>;

// --- Define the type for yielded events from the generator ---
export type AgentStreamEvent =
  | {
      type: "step";
      data: {
        text?: string; // Intermediate text/thought from the LLM
        toolCalls?: ToolCallPart[]; // Requested tool calls
        toolResults?: ToolResultPart[]; // Results from executed tools
        finishReason?: string; // Reason the step finished
      };
    }
  | { type: "final"; data: SqlAgentResponse } // The final structured answer
  | { type: "error"; error: string }; // An error occurred

// --- The "Answer" Tool ---
const finalAnswerTool = tool({
  description:
    "Use this tool *only* as the final step to provide the generated SQL query and related information.",
  parameters: sqlAgentResponseSchema,
  // No execute function means calling this tool terminates the agent's run.
});

// --- The New Agent Generator Function ---
export async function* generateSqlAgent(
  userPrompt: string,
  currentDbName?: string | null,
  currentTableName?: string | null,
  conversationHistory: CoreMessage[] = [],
): AsyncGenerator<AgentStreamEvent, void, unknown> {
  const model = await createModel();

  console.log(
    `Starting generateSqlAgent streamer for prompt: "${userPrompt}" (DB: ${currentDbName}, Table: ${currentTableName})`,
  );

  const agentTools = {
    ...dbTools,
    provideFinalAnswer: finalAnswerTool,
  };

  const systemPrompt = `
You are an expert database AI assistant, specialized in helping users interact with their database through natural language. Your primary goal is to understand user queries about their database and provide accurate responses through SQL operations.

<context>
Current Database: ${currentDbName || "Not specified"}
Current Table: ${currentTableName || "Not specified"}
You have access to the complete database schema and can explore relationships between tables
</context>

<capabilities>
1. Generate and execute SQL queries based on natural language requests
2. Explain database structure and relationships
3. Analyze data patterns and provide insights
4. Assist with database operations (SELECT, INSERT, UPDATE, DELETE)
5. Ensure data safety and validate operations
</capabilities>

<tools_available>
- listDatabases: List all available databases
- listTables: List all tables in a database
- getTableSchema: Get detailed table structure
- executeSql: Execute read-only SQL queries
</tools_available>

<operation_guidelines>
1. Understanding Phase:
   - Analyze the user's request carefully
   - Identify the type of operation needed (read/write)
   - Determine which tables and columns are relevant
   - Consider potential data relationships and constraints

2. Information Gathering:
   - Use listDatabases if database context is unclear
   - Use listTables to understand available tables
   - Use getTableSchema to understand table structure
   - Use executeSql with SELECT queries to validate assumptions

3. Query Generation & Execution:
   For READ operations (SELECT):
   - Generate efficient queries with appropriate JOINs and WHERE clauses
   - Use LIMIT when returning large datasets
   - Execute directly if safe

   For WRITE operations (INSERT/UPDATE/DELETE):
   - Always set requiresConfirmation to true
   - Include clear WHERE clauses for UPDATE/DELETE
   - Provide detailed explanation of the changes
   - Wait for user confirmation before execution
</operation_guidelines>

<safety_protocols>
1. Never execute destructive operations without confirmation
2. Validate inputs and handle edge cases
3. Use appropriate quoting for identifiers (\`) and strings ('')
4. Include WHERE clauses in UPDATE/DELETE operations
5. Consider the impact on related tables (foreign keys)
</safety_protocols>

<error_handling>
- If request is ambiguous: Ask for clarification
- If request is unsafe: Explain the risks and suggest alternatives
- If request is invalid: Explain why and suggest corrections
</error_handling>

<response_format>
Your response must include:
1. Clear explanation of what you're doing
2. Generated SQL query (if applicable)
3. Expected impact of the operation
4. Any potential risks or considerations
5. Results or confirmation requirements

Always use the provideFinalAnswer tool with:
- query: The SQL query (empty string if unsafe/unclear)
- explanation: Clear description of the operation and its impact
- requiresConfirmation: Boolean (true for all write operations)
- type: The type of SQL operation
- success: Whether the response successfully addresses the user's request
</response_format>

<examples>
1. User: "Show me all users who joined this month"
   Response: {
     query: "SELECT * FROM \`users\` WHERE MONTH(signup_date) = MONTH(CURRENT_DATE()) AND YEAR(signup_date) = YEAR(CURRENT_DATE())",
     explanation: "This query retrieves all users who signed up in the current month",
     requiresConfirmation: false,
     type: "SELECT"
   }

2. User: "Delete all inactive users"
   Response: {
     query: "DELETE FROM \`users\` WHERE \`last_login\` < DATE_SUB(NOW(), INTERVAL 6 MONTH)",
     explanation: "⚠️ This will permanently delete all users who haven't logged in for 6 months. Please confirm this action.",
     requiresConfirmation: true,
     type: "DELETE"
   }
</examples>

<best_practices>
1. Always validate table and column existence before generating queries
2. Use appropriate SQL syntax for TiDB/MySQL
3. Consider performance implications for large datasets
4. Provide clear explanations for all operations
5. Prioritize data safety and integrity
</best_practices>
`.trim();

  let accumulatedText = ""; // To accumulate text deltas if needed
  let finalCallArgs: any = null; // To store the args for the final answer tool
  let finalAnswerYielded = false; // Flag to ensure final answer is yielded only once

  try {
    // --- Stream the Agent's Response ---
    const { fullStream } = streamText({
      model,
      system: systemPrompt,
      messages: [
        ...conversationHistory,
        {
          role: "user",
          content: userPrompt,
        },
      ],
      tools: agentTools,
      toolChoice: "auto",
      maxSteps: 5,
    });

    // --- Process the Stream ---
    for await (const part of fullStream) {
      // Log every part for debugging
      console.log("Stream Part:", part);

      // Yield intermediate steps based on the stream part type
      switch (part.type) {
        case "text-delta":
          accumulatedText += part.textDelta;
          // Yield text delta as a step - can be noisy, might want to aggregate
          yield {
            type: "step",
            data: { text: part.textDelta }, // Yielding delta directly
          };
          break;

        case "tool-call":
          // Check if it's the final answer tool
          if (part.toolName === "provideFinalAnswer") {
            finalCallArgs = part.args; // Store args for final processing
            // Don't yield yet, wait for stream end or explicit finish
          } else {
            // Yield the request for other tools
            yield {
              type: "step",
              data: { toolCalls: [part] },
            };
          }
          break;

        case "tool-result":
          // Yield the result of a tool execution
          yield {
            type: "step",
            data: { toolResults: [part] },
          };
          break;

        case "finish":
          // Handle the finish event - might contain the final text if no tool was called last
          console.log("Stream Finished. Reason:", part.finishReason);
          console.log("Usage:", part.usage);
          // If there was remaining text and no final tool call args captured, yield it
          if (accumulatedText && !finalCallArgs && !finalAnswerYielded) {
            yield {
              type: "step",
              data: { text: accumulatedText, finishReason: part.finishReason },
            };
          }
          break;

        case "error":
          // Handle stream-level errors
          console.error("Stream Error:", part.error);
          yield { type: "error", error: `Stream error: ${part.error}` };
          finalAnswerYielded = true; // Prevent yielding fallback final answer later
          break;

        default:
          // Handle other potential part types if the API evolves
          console.warn("Unhandled stream part type:", (part as any).type);
      }

      // If we have the final call arguments, process and yield the final answer
      if (finalCallArgs && !finalAnswerYielded) {
        const validationResult =
          sqlAgentResponseSchema.safeParse(finalCallArgs);
        if (validationResult.success) {
          console.log("Final Answer Parsed:", validationResult.data);
          yield { type: "final", data: validationResult.data };
        } else {
          console.error(
            "Agent Error: Final answer tool arguments failed validation:",
            validationResult.error,
          );
          const errorMsg =
            "Error: AI agent failed to provide a valid final answer structure. Validation failed: " +
            validationResult.error.message;
          yield { type: "error", error: errorMsg };
          // Optionally yield a fallback final structure
          yield {
            type: "final",
            data: {
              query: "",
              explanation: errorMsg,
              requiresConfirmation: true,
              type: "NONE",
              success: false,
            },
          };
        }
        finalAnswerYielded = true; // Mark as yielded
        finalCallArgs = null; // Clear args
      }
    }

    // Fallback if the stream finishes without a proper final tool call yield
    if (!finalAnswerYielded) {
      console.error(
        "Agent Warning: Stream finished, but 'provideFinalAnswer' tool call was not processed or yielded.",
      );
      const fallbackExplanation =
        "Error: AI agent did not conclude with the expected final answer structure.";
      const errorMsg = accumulatedText
        ? `${fallbackExplanation}\nAgent final text output: ${accumulatedText}`
        : fallbackExplanation;
      yield { type: "error", error: errorMsg };
      yield {
        type: "final",
        data: {
          query: "",
          explanation: errorMsg,
          requiresConfirmation: true,
          type: "NONE",
          success: false,
        },
      };
    }
  } catch (error: any) {
    console.error("Error during generateSqlAgent stream processing:", error);
    const errorMsg = `An unexpected error occurred: ${error.message}`;
    if (!finalAnswerYielded) {
      // Avoid double error/final yield
      yield { type: "error", error: errorMsg };
      yield {
        type: "final",
        data: {
          query: "",
          explanation: errorMsg,
          requiresConfirmation: true,
          type: "NONE",
          success: false,
        },
      };
    }
  }
}

// --- Example Usage (replace with your actual call) ---
// async function testAgentGenerator() {
//   const db = 'testDB';
//   const table = 'users';
//   const prompt = `show me the 5 newest users`;

//   console.log('\n--- STARTING AGENT GENERATOR ---');
//   try {
//     for await (const event of generateSqlAgent(prompt, db, table)) {
//       console.log(`\n--- Received Event (Type: ${event.type}) ---`);
//       if (event.type === 'step') {
//         console.log('Step Data:', event.data);
//       } else if (event.type === 'final') {
//         console.log('Final Result:', event.data);
//       } else if (event.type === 'error') {
//         console.error('Error Event:', event.error);
//       }
//       console.log('------------------------------------');
//     }
//     console.log('\n--- AGENT GENERATOR FINISHED ---');
//   } catch (e) {
//     console.error("\n--- AGENT GENERATOR FAILED ---", e);
//   }
// }

// testAgentGenerator(); // Uncomment to run a test
