import { generateObject } from "ai";
import { createOpenRouter } from "@openrouter/ai-sdk-provider";
import { z } from "zod";

const openrouter = createOpenRouter({
  apiKey: import.meta.env.VITE_OPENROUTER_API_KEY,
});

const model = openrouter.chat("google/gemini-2.5-pro-exp-03-25:free");

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
