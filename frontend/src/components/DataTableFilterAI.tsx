import React, { useState, useRef, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { ScrollArea } from "@/components/ui/scroll-area";
import { generateSqlAgent, SqlAgentResponse } from "@/lib/ai";
import {
  Bot,
  SendHorizonal,
  User,
  CircleAlert,
  SparkleIcon,
} from "lucide-react";

interface Message {
  id: string;
  role: "user" | "assistant" | "system" | "error";
  content: string;
}

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
  const [messages, setMessages] = useState<Message[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const scrollAreaRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    // Scroll to bottom when messages change
    if (scrollAreaRef.current) {
      const scrollViewport = scrollAreaRef.current.querySelector(
        "div[data-radix-scroll-area-viewport]",
      );
      if (scrollViewport) {
        scrollViewport.scrollTop = scrollViewport.scrollHeight;
      }
    }
  }, [messages]);

  const handleSubmit = async (e?: React.FormEvent<HTMLFormElement>) => {
    if (e) e.preventDefault();
    if (!inputValue.trim() || isLoading) return;

    const userMessage: Message = {
      id: Date.now().toString(),
      role: "user",
      content: inputValue,
    };
    setMessages((prev) => [...prev, userMessage]);
    setInputValue("");
    setIsLoading(true);

    try {
      const result = await generateSqlAgent(
        userMessage.content,
        currentDb,
        currentTable,
      );

      console.log("AI Generated SQL:", result);

      setMessages((prev) => [
        ...prev,
        {
          id: Date.now().toString() + "-assist",
          role: "assistant",
          content: `Applying filter based on your description.`,
        },
      ]);
      onApplyQueryFromAI(result); // Apply the generated filter
      setIsOpen(false); // Close popover on success
    } catch (error: any) {
      console.error("Error calling filterTableData:", error);
      setMessages((prev) => [
        ...prev,
        {
          id: Date.now().toString() + "-error",
          role: "error",
          content: `An error occurred: ${error.message || "Unknown error"}`,
        },
      ]);
    } finally {
      setIsLoading(false);
    }
  };

  const renderMessageContent = (message: Message) => {
    switch (message.role) {
      case "user":
        return (
          <div className="flex items-start gap-2 justify-end">
            <span className="bg-primary text-primary-foreground p-2 rounded-md max-w-[80%] break-words">
              {message.content}
            </span>
            <User className="w-5 h-5 text-muted-foreground flex-shrink-0 mt-1" />
          </div>
        );
      case "assistant":
        return (
          <div className="flex items-start gap-2">
            <Bot className="w-5 h-5 text-blue-500 flex-shrink-0 mt-1" />
            <span className="bg-muted p-2 rounded-md max-w-[80%] break-words">
              {message.content}
            </span>
          </div>
        );
      case "system":
        return (
          <div className="text-xs text-muted-foreground italic text-center py-2">
            {message.content}
          </div>
        );
      case "error":
        return (
          <div className="flex items-start gap-2">
            <CircleAlert className="w-5 h-5 text-destructive flex-shrink-0 mt-1" />
            <span className="bg-destructive/10 text-destructive p-2 rounded-md max-w-[80%] break-words">
              {message.content}
            </span>
          </div>
        );
      default:
        return null;
    }
  };

  return (
    <Popover open={isOpen} onOpenChange={setIsOpen}>
      <PopoverTrigger asChild>
        <Button variant="ghost" size="icon">
          <SparkleIcon className="size-4" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-96 p-0">
        <div className="flex flex-col h-[50vh] max-h-[600px]">
          <ScrollArea className="flex-grow p-3" ref={scrollAreaRef}>
            <div className="space-y-3">
              {messages.map((msg) => (
                <React.Fragment key={msg.id}>
                  {renderMessageContent(msg)}
                </React.Fragment>
              ))}
              {isLoading && (
                <div className="flex items-start gap-2">
                  <Bot className="w-5 h-5 text-blue-500 flex-shrink-0 mt-1 animate-pulse" />
                  <span className="bg-muted p-2 rounded-md max-w-[80%] italic text-muted-foreground">
                    Thinking...
                  </span>
                </div>
              )}
            </div>
          </ScrollArea>
          <div className="p-3 border-t">
            <form onSubmit={handleSubmit} className="flex items-center gap-2">
              <Input
                value={inputValue}
                onChange={(e) => setInputValue(e.target.value)}
                placeholder="e.g., users created last month"
                disabled={isLoading}
                className="flex-grow"
                autoComplete="off"
                autoCorrect="off"
              />
              <Button
                type="submit"
                size="icon"
                disabled={isLoading || !inputValue.trim()}
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
