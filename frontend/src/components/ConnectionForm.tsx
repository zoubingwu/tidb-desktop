import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { inferConnectionDetails } from "@/lib/ai";
import { Loader2 } from "lucide-react";
import React, { useState, useEffect, FormEvent } from "react";
import { toast } from "sonner";
import {
  DeleteSavedConnection,
  SaveConnection,
  TestConnection,
} from "wailsjs/go/main/App";
import { services } from "wailsjs/go/models";
import { ClipboardGetText } from "wailsjs/runtime/runtime";

// Type definition for the connection details state
type ConnectionFormState = Pick<
  services.ConnectionDetails,
  "host" | "port" | "user" | "password" | "dbName" | "useTLS"
>;

const initialFormState: ConnectionFormState = {
  host: "",
  port: "4000", // Default TiDB port
  user: "",
  password: "",
  dbName: "",
  useTLS: true,
};

// Add props to control open state and notify on save
type ConnectionFormDialogProps = {
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  onConnectionSaved: (
    name: string,
    connection: services.ConnectionDetails,
  ) => void;
  defaultValues: {
    name: string;
    connection: ConnectionFormState;
  } | null;
  isEditing?: boolean;
  savedConnections: Record<string, services.ConnectionDetails>;
};

export function ConnectionFormDialog({
  isOpen,
  onOpenChange,
  onConnectionSaved,
  defaultValues,
  isEditing,
  savedConnections,
}: ConnectionFormDialogProps) {
  const [formState, setFormState] = useState<ConnectionFormState>(
    defaultValues?.connection || initialFormState,
  );
  const [connectionName, setConnectionName] = useState<string>(
    defaultValues?.name || "",
  );
  const [isTesting, setIsTesting] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [isInferring, setIsInferring] = useState(false);

  useEffect(() => {
    if (isOpen) {
      setFormState(defaultValues?.connection || initialFormState);
      setConnectionName(defaultValues?.name || "");
      setIsTesting(false);
      setIsSaving(false);
      setIsInferring(false);
    }
  }, [isOpen, defaultValues]);

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

  const handleNameChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setConnectionName(e.target.value);
  };

  const handleTestConnection = async () => {
    setIsTesting(true);
    try {
      const success = await TestConnection(formState);
      if (success) {
        toast.success("Connection Successful", {
          description: "Successfully connected to the database.",
        });
      } else {
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

  const handleSave = async (event?: FormEvent<HTMLFormElement>) => {
    // Prevent default form submission if called from onSubmit
    if (event) {
      event.preventDefault();
    }
    const name = connectionName.trim();
    if (!name) {
      toast.error("Missing Connection Name", {
        description: "Please provide a name to save this connection.",
      });
      return;
    }

    setIsSaving(true);
    try {
      const isNameTaken =
        (!isEditing && name in savedConnections) ||
        (isEditing &&
          defaultValues?.name &&
          defaultValues?.name !== name &&
          name in savedConnections);

      if (isNameTaken) {
        toast.error("Connection Name Already Exists", {
          description: "Please choose a different name.",
        });
        return;
      }

      if (isEditing && defaultValues?.name) {
        await DeleteSavedConnection(defaultValues.name);
      }
      await SaveConnection(name, formState);

      toast.success("Connection Saved", {
        description: `Connection '${name}' saved successfully.`,
      });
      onConnectionSaved(name, formState);
      onOpenChange(false);
    } catch (error: any) {
      toast.error("Save Error", {
        description:
          typeof error === "string"
            ? error
            : error?.message || "Could not save connection.",
      });
    } finally {
      setIsSaving(false);
    }
  };

  const handleReadFromClipboard = async () => {
    setIsInferring(true);
    try {
      const textFromClipboard = await ClipboardGetText();
      const inferredDetails = await inferConnectionDetails(textFromClipboard);
      if (inferredDetails) {
        setFormState((prev) => {
          return { ...prev, ...inferredDetails };
        });
        if (inferredDetails.host && !connectionName) {
          const suggestedName = `${inferredDetails.user || "user"}@${inferredDetails.host.split(".")[0]}`;
          setConnectionName(suggestedName);
        }
        toast.success("Details Inferred", {
          description:
            "Form updated from clipboard. Please verify and name the connection.",
        });
      } else {
        toast.error("Inference Failed", {
          description: "Could not infer details from clipboard content.",
        });
      }
    } catch (error: any) {
      toast.error("Clipboard Inference Error", {
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
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[525px]">
        <DialogHeader>
          <DialogTitle>
            {isEditing ? "Edit Database Connection" : "Add Database Connection"}
          </DialogTitle>
          <DialogDescription>
            Enter details to connect. Provide a name to save the connection for
            later use.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSave}>
          <div className="grid gap-4 py-4">
            {/* Connection Name Input */}
            <div className="grid grid-cols-4 items-center gap-4">
              <Label htmlFor="connectionName" className="text-right">
                Name <span className="text-destructive">*</span>
              </Label>
              <Input
                id="connectionName"
                name="connectionName"
                value={connectionName}
                onChange={handleNameChange}
                className="col-span-3"
                placeholder="e.g., My TiDB Cloud Dev, Local Test"
                autoComplete="off"
                autoCorrect="off"
              />
            </div>
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
                placeholder="Optional, e.g., test"
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
                  Force TLS, required for TiDB Cloud
                </label>
              </div>
            </div>
          </div>

          <DialogFooter className="gap-2 sm:justify-between">
            <Button
              type="button"
              variant="outline"
              onClick={handleReadFromClipboard}
              disabled={isTesting || isSaving || isInferring}
            >
              {isInferring && <Loader2 className="h-4 w-4 animate-spin" />}
              {isInferring ? "Inferring..." : "Read from Clipboard"}
            </Button>
            <div className="flex gap-2">
              <DialogClose asChild>
                <Button type="button" variant="ghost">
                  Cancel
                </Button>
              </DialogClose>
              <Button
                type="button"
                variant="secondary"
                onClick={handleTestConnection}
                disabled={isTesting || isSaving || isInferring}
              >
                {isTesting && <Loader2 className="h-4 w-4 animate-spin" />}
                {isTesting ? "Testing..." : "Test"}
              </Button>
              <Button
                type="submit"
                disabled={
                  isTesting || isSaving || isInferring || !connectionName.trim()
                }
              >
                {isSaving && <Loader2 className="h-4 w-4 animate-spin" />}
                {isSaving ? "Saving..." : "Save"}
              </Button>
            </div>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
