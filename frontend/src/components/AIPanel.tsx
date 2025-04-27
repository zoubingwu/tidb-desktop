import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { SqlAgentResponse, generateSqlAgent } from "@/lib/ai";
import { type CoreMessage } from "ai";
import { CircleAlert, EyeIcon, Loader, SendHorizonal } from "lucide-react";
import React, {
  useState,
  useRef,
  useEffect,
  useId,
  KeyboardEvent,
} from "react";
import Markdown from "react-markdown";
import TextareaAutosize from "react-textarea-autosize";
import { format } from "sql-formatter";

// Expanded message type to better represent stream states
type DisplayBlock = {
  id: string;
  type:
    | "user"
    | "ai-thinking"
    | "ai-text"
    | "ai-tool-call"
    | "ai-final"
    | "error";
  content: string | React.ReactNode;
  meta?: any;
  status?: "started" | "finished";
};

interface AIPanelProps {
  onApplyQueryFromAI: (query: SqlAgentResponse) => void;
  currentDb?: string;
  currentTable?: string;
  opened?: boolean;
}

export const AIPanel = ({
  onApplyQueryFromAI,
  currentDb,
  currentTable,
  opened,
}: AIPanelProps) => {
  const [inputValue, setInputValue] = useState("");
  const [displayBlocks, setDisplayBlocks] = useState<DisplayBlock[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const scrollAreaRef = useRef<HTMLDivElement>(null);
  const uniqueId = useId();
  const [maxRows, setMaxRows] = useState(2);
  const [conversationHistory, setConversationHistory] = useState<CoreMessage[]>(
    [],
  );

  useEffect(() => {
    if (scrollAreaRef.current) {
      const scrollViewport = scrollAreaRef.current.querySelector(
        "div[data-radix-scroll-area-viewport]",
      );
      if (scrollViewport) {
        requestAnimationFrame(() => {
          scrollViewport.scrollTop = scrollViewport.scrollHeight;
        });
      }
    }
  }, [displayBlocks]);

  useEffect(() => {
    requestAnimationFrame(() => {
      setMaxRows(opened ? 10 : 2);
    });
  }, [opened]);

  const handleSubmit = async (e?: React.FormEvent<HTMLFormElement>) => {
    if (e) e.preventDefault();
    if (!inputValue.trim() || isLoading) return;

    const userPrompt = inputValue;
    const userBlock: DisplayBlock = {
      id: `${uniqueId}-${Date.now()}-user`,
      type: "user",
      content: userPrompt,
    };

    // Add user message to history
    setConversationHistory((prev) => [
      ...prev,
      {
        role: "user",
        content: userPrompt,
      },
    ]);

    setDisplayBlocks((prev) => [...prev, userBlock]);
    setInputValue("");
    setIsLoading(true);

    let currentThinkingBlockId: string | null = null;
    let currentTextBlockId: string | null = null;
    let accumulatedText = "";
    let assistantResponse = ""; // Track full assistant response

    try {
      const agentStream = generateSqlAgent(
        userPrompt,
        currentDb ?? undefined,
        currentTable ?? undefined,
        conversationHistory, // Pass history to generateSqlAgent
      );

      for await (const event of agentStream) {
        if (currentThinkingBlockId) {
          setDisplayBlocks((prev) =>
            prev.filter((b) => b.id !== currentThinkingBlockId),
          );
          currentThinkingBlockId = null;
        }

        switch (event.type) {
          case "step":
            const { text, toolCalls, toolResults } = event.data;

            if (text) {
              accumulatedText += text;
              if (currentTextBlockId) {
                setDisplayBlocks((prev) =>
                  prev.map((b) =>
                    b.id === currentTextBlockId
                      ? { ...b, content: accumulatedText }
                      : b,
                  ),
                );
              } else {
                const newBlockId = `${uniqueId}-${Date.now()}-ai-text`;
                currentTextBlockId = newBlockId;
                setDisplayBlocks((prev) => [
                  ...prev,
                  {
                    id: newBlockId,
                    type: "ai-text",
                    content: accumulatedText,
                  },
                ]);
              }
            } else {
              currentTextBlockId = null;
              accumulatedText = "";
            }

            if (toolCalls?.length || toolResults?.length) {
              currentTextBlockId = null;
              accumulatedText = "";
            }

            if (toolCalls?.length) {
              toolCalls.forEach((call) => {
                // Add tool call as assistant message
                setConversationHistory((prev) => [
                  ...prev,
                  {
                    role: "assistant",
                    content: [
                      {
                        type: "tool-call",
                        toolCallId: call.toolCallId,
                        toolName: call.toolName,
                        args: call.args,
                      },
                    ],
                  },
                ]);

                setDisplayBlocks((prev) => [
                  ...prev,
                  {
                    id: `${uniqueId}-${call.toolCallId}-call`,
                    type: "ai-tool-call",
                    status: "started",
                    content: `Tool ${call.toolName} call started`,
                    meta: call,
                  },
                ]);
              });
            }

            if (toolResults?.length) {
              toolResults.forEach((result) => {
                // Add tool result as tool message
                setConversationHistory((prev) => [
                  ...prev,
                  {
                    role: "tool",
                    content: [
                      {
                        type: "tool-result",
                        toolCallId: result.toolCallId,
                        toolName: result.toolName,
                        result: result.result,
                      },
                    ],
                  },
                ]);

                setDisplayBlocks((prev) =>
                  prev.map((block) => {
                    if (
                      block.type === "ai-tool-call" &&
                      block.status === "started" &&
                      block.meta?.toolCallId === result.toolCallId
                    ) {
                      return {
                        ...block,
                        status: "finished",
                        content: `Tool ${result.toolName} call finished`,
                        meta: result,
                      };
                    }
                    return block;
                  }),
                );
              });
            }
            break;

          case "final":
            const finalResult = event.data;
            assistantResponse = `${finalResult.explanation}\n\nQuery: ${finalResult.query}`;
            setConversationHistory((prev) => [
              ...prev,
              {
                role: "assistant",
                content: assistantResponse,
              },
            ]);
            setDisplayBlocks((prev) => [
              ...prev,
              {
                id: `${uniqueId}-${Date.now()}-final`,
                type: "ai-final",
                content: (
                  <div>
                    <Markdown>{finalResult.explanation}</Markdown>
                    {finalResult.query && (
                      <pre className="p-2 mt-2 bg-gray-100 dark:bg-gray-800 rounded text-sm overflow-x-auto">
                        {format(finalResult.query, { language: "tidb" })}
                      </pre>
                    )}
                  </div>
                ),
                meta: finalResult,
              },
            ]);
            onApplyQueryFromAI(finalResult);
            break;

          case "error":
            setDisplayBlocks((prev) => [
              ...prev,
              {
                id: `${uniqueId}-${Date.now()}-error`,
                type: "error",
                content: event.error,
              },
            ]);
            break;
        }
      }
    } catch (error: any) {
      console.error("Error processing AI stream:", error);
      if (currentThinkingBlockId) {
        setDisplayBlocks((prev) =>
          prev.filter((b) => b.id !== currentThinkingBlockId),
        );
      }
      setDisplayBlocks((prev) => [
        ...prev,
        {
          id: `${uniqueId}-${Date.now()}-catch-error`,
          type: "error",
          content: `An unexpected error occurred: ${error.message || "Unknown error"}`,
        },
      ]);
    } finally {
      if (currentThinkingBlockId) {
        setDisplayBlocks((prev) =>
          prev.filter((b) => b.id !== currentThinkingBlockId),
        );
      }
      setIsLoading(false);
    }
  };

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault(); // Prevent newline
      // We need to cast the event target or use a different approach
      // if handleSubmit requires the form event.
      // Since handleSubmit doesn't strictly need it here, we can call it directly.
      handleSubmit(); // Submit form
    }
  };

  console.log("display blocks", displayBlocks);

  const renderBlockContent = (block: DisplayBlock) => {
    const baseClasses = "rounded-md break-words text-sm w-full";
    switch (block.type) {
      case "user":
        return (
          <div className={`user ${baseClasses} bg-muted mb-2 p-2 select-text!`}>
            {block.content}
          </div>
        );
      case "ai-thinking":
        return (
          <div
            className={`ai-thinking ${baseClasses} text-muted-foreground italic flex gap-1 text-xs`}
          >
            <Loader className="size-3 animate-spin flex-shrink-0" />
            <span>{block.content}</span>
          </div>
        );
      case "ai-text":
        return (
          <div className={`ai-text ${baseClasses} my-2 force-select-text`}>
            <Markdown>{block.content as string}</Markdown>
          </div>
        );
      case "ai-tool-call":
        const isFinished = block.status === "finished";
        const metaContent = isFinished ? block.meta?.result : block.meta;
        return (
          <div
            className={`ai-tool-call ${baseClasses} text-muted-foreground text-xs py-0`}
          >
            <Collapsible>
              <CollapsibleTrigger>
                <div className="cursor-pointer flex items-start gap-1">
                  <EyeIcon className="size-3 flex-shrink-0" />
                  <p>{block.content as string}</p>
                </div>
              </CollapsibleTrigger>
              <CollapsibleContent>
                <pre className="mt-2 p-2 bg-gray-100 dark:bg-gray-800 rounded text-xs max-h-[200px] overflow-auto">
                  {JSON.stringify(metaContent, null, 2)}
                </pre>
              </CollapsibleContent>
            </Collapsible>
          </div>
        );
      case "ai-final":
        return (
          <div className={`ai-final ${baseClasses} my-2 force-select-text`}>
            {block.content as string}
          </div>
        );
      case "error":
        return (
          <div
            className={`error ${baseClasses} bg-destructive/10 text-destructive flex items-start gap-2 p-2`}
          >
            <CircleAlert className="w-4 h-4 flex-shrink-0 mt-0.5" />
            <span>{block.content as string}</span>
          </div>
        );
      default:
        return null;
    }
  };

  return (
    <div ref={scrollAreaRef} className="h-full p-4 overflow-auto bg-muted/50">
      {displayBlocks.map((block) => (
        <React.Fragment key={block.id}>
          {renderBlockContent(block)}
        </React.Fragment>
      ))}

      <div className="bg-background rounded-md overflow-hidden text-sm mt-2">
        <TextareaAutosize
          value={inputValue}
          onChange={(e) => setInputValue(e.target.value)}
          placeholder="e.g., list users created last month in db.users"
          disabled={isLoading}
          className="w-full resize-none p-2 outline-0 placeholder:text-muted-foreground/50"
          autoComplete="off"
          autoCorrect="off"
          minRows={2}
          maxRows={maxRows}
          onKeyDown={handleKeyDown}
          autoFocus
        />
        <div className="flex justify-between border-t border-muted p-2">
          {isLoading ? (
            <div className="flex items-center gap-1 text-muted-foreground">
              <Loader className="size-3 animate-spin flex-shrink-0" />
              <span className="text-xs">Generating...</span>
            </div>
          ) : (
            <div />
          )}

          <Button
            type="submit"
            size="icon"
            disabled={isLoading || !inputValue.trim()}
            aria-label="Send message"
            onClick={() => handleSubmit()}
          >
            <SendHorizonal className="size-3" />
          </Button>
        </div>
      </div>
    </div>
  );
};
