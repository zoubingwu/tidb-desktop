import React, { useState, useEffect } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import { services } from "wailsjs/go/models";
import { TestConnection } from "wailsjs/go/main/App";

// Type definition for the connection details state
type ConnectionFormState = Omit<services.ConnectionDetails, "toJSON">; // Exclude toJSON if present

const initialFormState: ConnectionFormState = {
  host: "",
  port: "4000", // Default TiDB port
  user: "",
  password: "",
  dbName: "",
  useTLS: true,
};

export function ConnectionFormDialog() {
  const [isOpen, setIsOpen] = useState(false);
  const [formState, setFormState] =
    useState<ConnectionFormState>(initialFormState);
  const [isTesting, setIsTesting] = useState(false);
  const [isConnecting, setIsConnecting] = useState(false);
  const [isInferring, setIsInferring] = useState(false);

  // Reset form when dialog opens
  useEffect(() => {
    if (isOpen) {
      setFormState(initialFormState);
      setIsTesting(false);
      setIsConnecting(false);
      setIsInferring(false);
    }
  }, [isOpen]);

  const handleChange = (
    e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>,
  ) => {
    const { name, value, type } = e.target;
    // Handle checkbox separately
    if (type === "checkbox" && e.target instanceof HTMLInputElement) {
      // Cast target to HTMLInputElement after the type guard
      const inputElement = e.target as HTMLInputElement;
      setFormState((prev) => ({ ...prev, [name]: inputElement.checked }));
    } else {
      setFormState((prev) => ({ ...prev, [name]: value }));
    }
  };

  const handleTestConnection = async () => {
    setIsTesting(true);
    try {
      // Ensure TestConnection exists and is bound in your Go app.go
      const success = await TestConnection(formState);
      if (success) {
        toast.success("Connection Successful", {
          description: "Successfully connected to the database.",
        });
      } else {
        // This case might mean Ping failed but no Go error occurred
        toast.error("Connection Test Failed", {
          description: "Could not ping the database.",
        });
      }
    } catch (error: any) {
      console.error("Test Connection Error:", error);
      toast.error("Connection Test Error", {
        description:
          typeof error === "string"
            ? error
            : error?.message || "An unknown error occurred.",
      });
    } finally {
      setIsTesting(false);
    }
  };

  const handleConnect = async () => {
    setIsConnecting(true);
    try {
      const success = await TestConnection(formState);
      if (success) {
        // Connection successful, Go backend handles window transition/event.
        // The event listener in App.tsx should handle UI change.
        setIsOpen(false); // Close the dialog on success
        toast.info("Connecting...", {
          description: "Connection established, loading main view.",
        });
      } else {
        toast.error("Connection Failed", {
          description: "Failed to establish connection (unknown reason).",
        });
      }
    } catch (error: any) {
      console.error("Connect Error:", error);
      toast.error("Connection Error", {
        description:
          typeof error === "string"
            ? error
            : error?.message || "An unknown error occurred.",
      });
    } finally {
      setIsConnecting(false);
    }
  };

  const handleReadFromClipboard = async () => {
    setIsInferring(true);
    toast.info("Reading Clipboard...", {
      description: "Attempting to infer connection details.",
    });
    try {
      // TODO: Implement ReadClipboard and InferConnectionDetails in Go
      // const clipboardText = await ReadClipboard();
      // if (!clipboardText) {
      //     toast({ title: "Clipboard Empty", variant: "destructive" });
      //     return;
      // }
      // const inferredDetails = await InferConnectionDetails(clipboardText);
      // setFormState(prev => ({ ...prev, ...inferredDetails })); // Merge inferred details
      // toast({ title: "Details Inferred", description: "Form updated from clipboard." });

      // --- Mock Implementation ---
      await new Promise((resolve) => setTimeout(resolve, 1000)); // Simulate delay
      const mockDetails: Partial<ConnectionFormState> = {
        host: "clip.example.com",
        user: "clip_user",
        dbName: "clip_db",
      };
      setFormState((prev) => ({ ...prev, ...mockDetails }));
      toast.success("Mock Details Inferred", {
        description: "Form updated with mock clipboard data.",
      });
      // --- End Mock ---
    } catch (error: any) {
      console.error("Clipboard/Infer Error:", error);
      toast.error("Inference Error", {
        description:
          typeof error === "string"
            ? error
            : error?.message || "Could not read or infer from clipboard.",
      });
    } finally {
      setIsInferring(false);
    }
  };

  return (
    <Dialog open={isOpen} onOpenChange={setIsOpen}>
      <DialogTrigger asChild>
        <Button size="lg">Add New Connection</Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-[525px]">
        {" "}
        {/* Wider dialog */}
        <DialogHeader>
          <DialogTitle>Database Connection Details</DialogTitle>
          <DialogDescription>
            Enter the information needed to connect to your TiDB database.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-4">
          {/* Host */}
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="host" className="text-right">
              Host
            </Label>
            <Input
              id="host"
              name="host"
              value={formState.host}
              onChange={handleChange}
              className="col-span-3"
              placeholder="e.g., gateway01.us-east-1.prod.aws.tidbcloud.com"
            />
          </div>
          {/* Port */}
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="port" className="text-right">
              Port
            </Label>
            <Input
              id="port"
              name="port"
              type="number"
              value={formState.port}
              onChange={handleChange}
              className="col-span-3"
              placeholder="e.g., 4000"
            />
          </div>
          {/* User */}
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="user" className="text-right">
              User
            </Label>
            <Input
              id="user"
              name="user"
              value={formState.user}
              onChange={handleChange}
              className="col-span-3"
              placeholder="e.g., root or your_db_user"
            />
          </div>
          {/* Password */}
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="password" className="text-right">
              Password
            </Label>
            <Input
              id="password"
              name="password"
              type="password"
              value={formState.password}
              onChange={handleChange}
              className="col-span-3"
            />
          </div>
          {/* Database Name */}
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="dbName" className="text-right">
              Database
            </Label>
            <Input
              id="dbName"
              name="dbName"
              value={formState.dbName}
              onChange={handleChange}
              className="col-span-3"
              placeholder="(Optional) e.g., test"
            />
          </div>
          {/* Use TLS Checkbox - Note: Go backend auto-detects for .tidbcloud.com */}
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="useTLS" className="text-right">
              Use TLS
            </Label>
            <div className="col-span-3 flex items-center space-x-2">
              <Checkbox
                id="useTLS"
                name="useTLS"
                checked={formState.useTLS}
                onCheckedChange={(checked) =>
                  setFormState((prev) => ({ ...prev, useTLS: !!checked }))
                }
              />
              <label
                htmlFor="useTLS"
                className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70 text-muted-foreground"
              >
                Force TLS (Required for TiDB Cloud if not auto-detected)
              </label>
            </div>
          </div>
        </div>
        <DialogFooter className="gap-2 sm:justify-between">
          {" "}
          {/* Adjust footer layout */}
          <Button
            variant="outline"
            onClick={handleReadFromClipboard}
            disabled={isTesting || isConnecting || isInferring}
          >
            {isInferring ? "Inferring..." : "Read from Clipboard"}
          </Button>
          <div className="flex gap-2">
            <Button
              variant="secondary"
              onClick={handleTestConnection}
              disabled={isTesting || isConnecting || isInferring}
            >
              {isTesting ? "Testing..." : "Test Connection"}
            </Button>
            <Button
              type="submit" // Technically not submitting a form, but standard practice
              onClick={handleConnect}
              disabled={isTesting || isConnecting || isInferring}
            >
              {isConnecting ? "Connecting..." : "Connect"}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
