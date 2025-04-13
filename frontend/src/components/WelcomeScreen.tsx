import { useState, useEffect } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Loader2 } from "lucide-react";
import { ConnectionFormDialog } from "./ConnectionForm";
import { ListSavedConnections, ConnectUsingSaved } from "wailsjs/go/main/App";
import { services } from "wailsjs/go/models";

type SavedConnectionsMap = Record<string, services.ConnectionDetails>;

const WelcomeScreen = () => {
  const [savedConnections, setSavedConnections] =
    useState<SavedConnectionsMap | null>(null);
  const [selectedConnectionName, setSelectedConnectionName] =
    useState<string>("");
  const [isLoadingConnections, setIsLoadingConnections] = useState(true);
  const [isConnectingSaved, setIsConnectingSaved] = useState(false);

  // Fetch saved connections on mount
  useEffect(() => {
    const fetchConnections = async () => {
      setIsLoadingConnections(true);
      try {
        const connections = await ListSavedConnections();
        setSavedConnections(connections || {}); // Ensure it's an object even if null/empty
      } catch (error: any) {
        console.error("Error fetching saved connections:", error);
        toast.error("Failed to load saved connections", {
          description:
            typeof error === "string"
              ? error
              : error?.message || "An unknown error occurred.",
        });
        setSavedConnections({}); // Set to empty object on error
      } finally {
        setIsLoadingConnections(false);
      }
    };
    fetchConnections();
  }, []); // Run only once on mount

  const handleConnectSaved = async () => {
    if (!selectedConnectionName) {
      toast.warning("No connection selected", {
        description: "Please choose a connection from the list.",
      });
      return;
    }
    setIsConnectingSaved(true);
    try {
      const connectedDetails = await ConnectUsingSaved(selectedConnectionName);
      if (connectedDetails) {
        // App.tsx listener handles UI transition via "connection:established" event
        toast.info("Connecting Session...", {
          description: `Connecting to '${selectedConnectionName}'...`,
        });
        // No need to close dialog as we are not in one
      } else {
        // Should ideally not happen if Go func throws error
        toast.error("Connection Failed", {
          description: `Failed to connect using '${selectedConnectionName}'.`,
        });
      }
    } catch (error: any) {
      console.error("Connect using saved error:", error);
      toast.error("Connection Failed", {
        description:
          typeof error === "string"
            ? error
            : error?.message ||
              `Failed to connect using '${selectedConnectionName}'.`,
      });
    } finally {
      setIsConnectingSaved(false);
    }
  };

  const hasSavedConnections =
    savedConnections && Object.keys(savedConnections).length > 0;

  return (
    <div className="flex items-center justify-center h-full bg-gradient-to-br from-background to-secondary/30 p-8">
      <Card className="w-full max-w-md shadow-lg">
        <CardHeader className="text-center">
          <CardTitle className="text-2xl">Welcome to TiDB Desktop</CardTitle>
          <CardDescription>Manage your TiDB connections</CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          {isLoadingConnections ? (
            <div className="flex items-center justify-center p-4">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
              <span className="ml-2 text-muted-foreground">
                Loading connections...
              </span>
            </div>
          ) : hasSavedConnections ? (
            <div className="space-y-4">
              <Label htmlFor="saved-connections">
                Connect using a saved connection:
              </Label>
              <div className="flex gap-2">
                <Select
                  value={selectedConnectionName}
                  onValueChange={setSelectedConnectionName}
                >
                  <SelectTrigger id="saved-connections" className="flex-grow">
                    {selectedConnectionName ? (
                      <span className="truncate text-left">
                        {selectedConnectionName}
                      </span>
                    ) : (
                      <SelectValue placeholder="Select a connection..." />
                    )}
                  </SelectTrigger>
                  <SelectContent>
                    {Object.entries(savedConnections).map(([name, details]) => (
                      <SelectItem key={name} value={name}>
                        <div>
                          <div className="font-medium">{name}</div>
                          <div className="text-xs text-muted-foreground">
                            {details.user}@{details.host}:{details.port}
                          </div>
                        </div>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Button
                  onClick={handleConnectSaved}
                  disabled={!selectedConnectionName || isConnectingSaved}
                  className="shrink-0"
                >
                  {isConnectingSaved && (
                    <Loader2 className="h-4 w-4 animate-spin mr-2" />
                  )}
                  Connect
                </Button>
              </div>
            </div>
          ) : (
            <p className="text-center text-muted-foreground italic">
              No saved connections found.
            </p>
          )}

          <div className="relative">
            <Separator />
            <span className="absolute left-1/2 -translate-x-1/2 -top-3 bg-background px-2 text-sm text-muted-foreground">
              OR
            </span>
          </div>

          <div className="text-center">
            {/* Render the Dialog component trigger here */}
            <ConnectionFormDialog />
          </div>
        </CardContent>
        {/* Optional Footer */}
        {/* <CardFooter className="text-center text-xs text-muted-foreground">
                    Footer text if needed
                </CardFooter> */}
      </Card>
    </div>
  );
};

export default WelcomeScreen;
