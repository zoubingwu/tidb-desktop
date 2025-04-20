import React, { useState, useRef, useEffect, useId } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { ScrollArea } from "@/components/ui/scroll-area";
import { generateSqlAgent, SqlAgentResponse } from "@/lib/ai";
import {
  SendHorizonal,
  CircleAlert,
  SparkleIcon,
  Loader2,
  EyeIcon,
} from "lucide-react";
import { format } from "sql-formatter";
import { TooltipTrigger } from "./ui/tooltip";

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
  content: string | React.ReactNode; // Allow React nodes for better formatting
  meta?: any; // Store raw tool call/result data if needed
  status?: "started" | "finished"; // Added status for the combined type
};

interface DataTableFilterAIProps {
  onApplyQueryFromAI: (query: SqlAgentResponse) => void;
  currentDb?: string;
  currentTable?: string;
}

export const DataTableFilterAI = ({
  onApplyQueryFromAI,
  currentDb,
  currentTable,
}: DataTableFilterAIProps) => {
  const [isOpen, setIsOpen] = useState(false);
  const [inputValue, setInputValue] = useState("");
  const [displayBlocks, setDisplayBlocks] = useState<DisplayBlock[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const scrollAreaRef = useRef<HTMLDivElement>(null);
  const uniqueId = useId(); // For generating keys

  useEffect(() => {
    // Scroll to bottom when blocks change
    if (scrollAreaRef.current) {
      const scrollViewport = scrollAreaRef.current.querySelector(
        "div[data-radix-scroll-area-viewport]",
      );
      if (scrollViewport) {
        // Use requestAnimationFrame for smoother scrolling after render
        requestAnimationFrame(() => {
          scrollViewport.scrollTop = scrollViewport.scrollHeight;
        });
      }
    }
  }, [displayBlocks]);

  const handleSubmit = async (e?: React.FormEvent<HTMLFormElement>) => {
    if (e) e.preventDefault();
    if (!inputValue.trim() || isLoading) return;

    const userPrompt = inputValue;
    const userBlock: DisplayBlock = {
      id: `${uniqueId}-${Date.now()}-user`,
      type: "user",
      content: userPrompt,
    };
    setDisplayBlocks((prev) => [...prev, userBlock]);
    setInputValue("");
    setIsLoading(true);

    let currentThinkingBlockId: string | null = null;
    let currentTextBlockId: string | null = null;
    let accumulatedText = "";

    try {
      const agentStream = generateSqlAgent(
        userPrompt,
        currentDb ?? undefined, // Pass undefined if null/empty
        currentTable ?? undefined,
      );

      for await (const event of agentStream) {
        // Remove "Thinking..." block when the first real event arrives
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
                // Update existing text block
                setDisplayBlocks((prev) =>
                  prev.map((b) =>
                    b.id === currentTextBlockId
                      ? { ...b, content: accumulatedText }
                      : b,
                  ),
                );
              } else {
                // Create new text block
                const newBlockId = `${uniqueId}-${Date.now()}-ai-text`;
                currentTextBlockId = newBlockId; // Track the ID of the block we are currently adding text to
                setDisplayBlocks((prev) => [
                  ...prev,
                  // Use the newBlockId which is guaranteed to be a string here
                  {
                    id: newBlockId,
                    type: "ai-text",
                    content: accumulatedText,
                  },
                ]);
              }
            } else {
              // If a step event arrives without text, it signifies the end of the previous text block.
              // Reset tracker vars so the next text delta starts a new block.
              currentTextBlockId = null;
              accumulatedText = "";
            }

            // Reset text tracking if tool calls/results arrive, forcing a new text block after
            if (toolCalls?.length || toolResults?.length) {
              currentTextBlockId = null;
              accumulatedText = "";
            }

            if (toolCalls?.length) {
              toolCalls.forEach((call) => {
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
            // Display final explanation/query in a block
            setDisplayBlocks((prev) => [
              ...prev,
              {
                id: `${uniqueId}-${Date.now()}-final`,
                type: "ai-final",
                content: (
                  <div>
                    <p>{finalResult.explanation}</p>
                    {finalResult.query && (
                      <pre className="mt-2 p-2 bg-gray-100 dark:bg-gray-800 rounded text-sm overflow-x-auto">
                        {format(finalResult.query, { language: "tidb" })}
                      </pre>
                    )}
                  </div>
                ),
                meta: finalResult,
              },
            ]);
            // Apply the query
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
      // Ensure thinking block is removed on error
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
      // Ensure thinking block is removed finally
      if (currentThinkingBlockId) {
        setDisplayBlocks((prev) =>
          prev.filter((b) => b.id !== currentThinkingBlockId),
        );
      }
      setIsLoading(false);
    }
  };

  const renderBlockContent = (block: DisplayBlock) => {
    const baseClasses =
      "max-w-[366px] rounded-md break-words text-sm p-2 w-full select-text!";
    switch (block.type) {
      case "user":
        return (
          <div className={`user ${baseClasses} bg-muted/50 mb-2`}>
            {block.content}
          </div>
        );
      case "ai-thinking":
        return (
          <div
            className={`ai-thinking ${baseClasses}  text-muted-foreground italic flex items-center gap-2 text-xs`}
          >
            <Loader2 className="size-3 animate-spin flex-shrink-0" />
            <span>{block.content}</span>
          </div>
        );
      case "ai-text":
        return <div className={`ai-text ${baseClasses}`}>{block.content}</div>;
      case "ai-tool-call":
        const isFinished = block.status === "finished";
        const metaContent = isFinished ? block.meta?.result : block.meta;
        return (
          <div
            className={`ai-tool-call-result ${baseClasses} text-muted-foreground text-xs py-0 ${isFinished ? "" : "mb-1"}`}
          >
            <Collapsible>
              <CollapsibleTrigger>
                <div className="cursor-pointer flex items-center gap-1">
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
          <div className={`ai-final ${baseClasses} `}>{block.content}</div>
        );
      case "error":
        return (
          <div
            className={`error ${baseClasses} bg-destructive/10 text-destructive flex items-start gap-2`}
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
    <Popover open={isOpen} onOpenChange={setIsOpen}>
      <PopoverTrigger asChild>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="icon">
            <SparkleIcon className="size-4" />
            <span className="sr-only">AI Filter</span>
          </Button>
        </TooltipTrigger>
      </PopoverTrigger>
      <PopoverContent className="w-[400px] p-0" align="end">
        <div className="flex flex-col w-full">
          <ScrollArea
            className="flex-grow p-4 w-full h-[60vh] max-h-[700px]"
            ref={scrollAreaRef}
          >
            {displayBlocks.map((block) => (
              <React.Fragment key={block.id}>
                {renderBlockContent(block)}
              </React.Fragment>
            ))}
            {isLoading &&
              !displayBlocks.some((b) => b.type === "ai-thinking") &&
              renderBlockContent({
                id: `${uniqueId}-initial-thinking`,
                type: "ai-thinking",
                content: "Thinking...",
              })}
          </ScrollArea>
          <div className="p-2  bg-background rounded-b-md">
            <form onSubmit={handleSubmit} className="flex items-center gap-2">
              <Input
                value={inputValue}
                onChange={(e) => setInputValue(e.target.value)}
                placeholder="e.g., list users created last month in db.users"
                disabled={isLoading}
                className="flex-grow"
                autoComplete="off"
                autoCorrect="off"
              />
              <Button
                type="submit"
                size="icon"
                disabled={isLoading || !inputValue.trim()}
                aria-label="Send message"
              >
                <SendHorizonal className="h-4 w-4" />
              </Button>
            </form>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
};
