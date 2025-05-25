import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { LoadingTypewriter } from "@/components/ui/loading-typewriter";
import { generateSqlAgent } from "@/lib/ai";
import { useMemoizedFn } from "ahooks";
import { type CoreMessage, type Tool } from "ai";
import {
  CheckCircle2Icon,
  Loader,
  PlayIcon,
  SendHorizonal,
} from "lucide-react";
import React, {
  KeyboardEvent,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import Markdown from "react-markdown";
import TextareaAutosize from "react-textarea-autosize";
import { toast } from "sonner";

// Expanded message type to better represent stream states
type DisplayBlock = {
  id: string;
  type: "user" | "ai-thinking" | "ai-text" | "ai-tool-call" | "error" | "sql";
  content: string;
  meta?: any;
  status?: "started" | "finished";
};

const useGenerateSQLAgent = (tools: Record<string, Tool>) => {
  const [isLoading, setIsLoading] = useState(false);
  const [displayBlocks, setDisplayBlocks] = useState<DisplayBlock[]>([]);
  const [messages, setMessages] = useState<CoreMessage[]>([]);
  const uniqueId = useId();
  const appendMessage = useMemoizedFn((message: CoreMessage) => {
    setMessages((prev) => [...prev, message]);
  });
  const appendDisplayBlock = useMemoizedFn((block: DisplayBlock) => {
    setDisplayBlocks((prev) => [...prev, block]);
  });

  const handleSubmit = useMemoizedFn(async (userPrompt: string) => {
    const userBlock: DisplayBlock = {
      id: `${uniqueId}-${Date.now()}-user`,
      type: "user",
      content: userPrompt,
    };

    appendMessage({
      role: "user",
      content: userPrompt,
    });

    appendDisplayBlock(userBlock);
    setIsLoading(true);

    let currentThinkingBlockId: string | null = null;
    let currentTextBlockId: string | null = null;
    let accumulatedText = "";

    try {
      const agentStream = generateSqlAgent(userPrompt, messages, tools);

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
                appendMessage({
                  role: "assistant",
                  content: [
                    {
                      type: "tool-call",
                      toolCallId: call.toolCallId,
                      toolName: call.toolName,
                      args: call.args,
                    },
                  ],
                });

                appendDisplayBlock({
                  id: `${uniqueId}-${call.toolCallId}-call`,
                  type: "ai-tool-call",
                  status: "started",
                  content: `Tool ${call.toolName} call started`,
                  meta: call,
                });

                // display sql query
                if (call.toolName === "executeSql") {
                  appendDisplayBlock({
                    id: `${uniqueId}-${call.toolCallId}-sql`,
                    type: "sql",
                    content: (call.args as any).query,
                    meta: {
                      ...(call.args as any),
                    },
                  });
                }
              });
            }

            if (toolResults?.length) {
              toolResults.forEach((result) => {
                // Add tool result as tool message
                appendMessage({
                  role: "tool",
                  content: [
                    {
                      type: "tool-result",
                      toolCallId: result.toolCallId,
                      toolName: result.toolName,
                      result: result.result,
                    },
                  ],
                });

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
        }
      }
    } catch (error: any) {
      console.error("Error processing AI stream:", error);
      if (currentThinkingBlockId) {
        setDisplayBlocks((prev) =>
          prev.filter((b) => b.id !== currentThinkingBlockId),
        );
      }
      appendDisplayBlock({
        id: `${uniqueId}-${Date.now()}-catch-error`,
        type: "error",
        content: `An unexpected error occurred: ${error.message || "Unknown error"}`,
      });
    } finally {
      if (currentThinkingBlockId) {
        setDisplayBlocks((prev) =>
          prev.filter((b) => b.id !== currentThinkingBlockId),
        );
      }
      setIsLoading(false);
    }
  });

  return {
    handleSubmit,
    messages: displayBlocks,
    isLoading,
  };
};

interface AIPanelProps {
  onApplyQueryFromAI: (query: string, dbName: string) => void;
  opened?: boolean;
  isExecutingSQLFromAI?: boolean;
  tools: Record<string, Tool>;
}

export const AIPanel = ({
  onApplyQueryFromAI,
  opened,
  isExecutingSQLFromAI,
  tools,
}: AIPanelProps) => {
  const [inputValue, setInputValue] = useState("");
  const scrollAreaRef = useRef<HTMLDivElement>(null);
  const [maxRows, setMaxRows] = useState(2);

  const {
    handleSubmit: handleSubmitPrompt,
    messages,
    isLoading,
  } = useGenerateSQLAgent(tools);

  useEffect(() => {
    if (scrollAreaRef.current) {
      const container = scrollAreaRef.current;
      requestAnimationFrame(() => {
        container.scrollTop = container.scrollHeight;
      });
    }
  }, [messages]);

  // Set maxRows for the TextareaAutosize component.
  // When the panel is opened, maxRows is initially set to 2.
  // Then, using requestAnimationFrame, it's updated to 10 after the initial render.
  // This prevents the textarea from immediately rendering at its maximum height (10 rows)
  // upon opening the panel, providing a smoother visual expansion.
  useEffect(() => {
    requestAnimationFrame(() => {
      setMaxRows(opened ? 10 : 2);
    });
  }, [opened]);

  const handleSubmit = async (e?: React.FormEvent<HTMLFormElement>) => {
    if (e) e.preventDefault();
    if (!inputValue.trim() || isLoading) return;

    const userPrompt = inputValue;
    setInputValue("");
    await handleSubmitPrompt(userPrompt);
  };

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault(); // Prevent newline
      handleSubmit();
    }
  };

  const renderMessage = (message: DisplayBlock, index: number) => {
    let baseClasses = "rounded-md break-words text-sm w-full";
    if (index > 0) {
      baseClasses += " my-2";
    } else {
      baseClasses += " mb-2";
    }
    switch (message.type) {
      case "user":
        return (
          <div className={`user ${baseClasses} bg-muted mb-2 p-2 select-text!`}>
            {message.content}
          </div>
        );
      case "ai-thinking":
        return (
          <div
            className={`ai-thinking ${baseClasses} text-muted-foreground italic flex gap-1 text-xs`}
          >
            <Loader className="size-3 animate-spin flex-shrink-0" />
            <span>{message.content}</span>
          </div>
        );
      case "ai-text":
        return (
          <div
            className={`ai-text ${baseClasses} force-select-text markdown-body`}
          >
            <Markdown>{message.content as string}</Markdown>
          </div>
        );
      case "ai-tool-call":
        const metaContent = message.meta;

        return (
          <div
            className={`ai-tool-call ${baseClasses} text-muted-foreground text-xs py-0`}
          >
            <Collapsible>
              <CollapsibleTrigger>
                <div className="cursor-pointer flex items-start gap-1">
                  {message.status === "started" && (
                    <Loader className="size-3 flex-shrink-0 animate-spin relative top-[1px]" />
                  )}

                  {message.status === "finished" && (
                    <CheckCircle2Icon className="size-3 flex-shrink-0 relative top-[2px] text-green-600" />
                  )}

                  <p className="text-left">{message.content as string}</p>
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
      case "error":
        return (
          <div className={`error ${baseClasses} my-2 text-red-600`}>
            <Markdown>{message.content}</Markdown>
          </div>
        );
      case "sql":
        return (
          <div className={`sql ${baseClasses} my-2 force-select-text`}>
            <div className="markdown-body">
              <div className="rounded relative">
                <pre className="whitespace-pre-wrap p-2">{message.content}</pre>
                <Button
                  size="icon"
                  onClick={() => {
                    toast(
                      "This action is irreversible, are you really sure you want to execute this query?",
                      {
                        action: (
                          <Button
                            size="sm"
                            variant="destructive"
                            className="text-xs"
                            onClick={() => {
                              onApplyQueryFromAI(
                                message.meta.result.query,
                                message.meta.result.dbName,
                              );
                              toast.dismiss();
                            }}
                          >
                            Confirm
                          </Button>
                        ),
                      },
                    );
                  }}
                  className="absolute bottom-2 right-2 size-5"
                  disabled={isExecutingSQLFromAI}
                >
                  {isExecutingSQLFromAI ? (
                    <Loader className="size-3 animate-spin" />
                  ) : (
                    <PlayIcon className="size-3" />
                  )}
                </Button>
              </div>
            </div>
          </div>
        );
      default:
        return null;
    }
  };

  return (
    <div
      ref={scrollAreaRef}
      className="h-full flex flex-col px-4 py-2 overflow-auto bg-muted/50"
    >
      {messages.length > 0 && (
        <div className="flex-1">
          {messages.map((message, index) => (
            <React.Fragment key={message.id}>
              {renderMessage(message, index)}
            </React.Fragment>
          ))}
        </div>
      )}

      <div className="bg-background rounded-md overflow-hidden text-sm flex-shrink-0">
        <TextareaAutosize
          value={inputValue}
          onChange={(e) => setInputValue(e.target.value)}
          placeholder="e.g., list users created last month in db.users"
          disabled={isLoading}
          className="w-full resize-none p-2 outline-0 placeholder:text-neutral-400"
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
              <LoadingTypewriter className="text-xs">
                Generating
              </LoadingTypewriter>
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
