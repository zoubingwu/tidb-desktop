import { useState, useEffect, useCallback, useMemo, memo } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Loader2Icon, PlusCircleIcon, SettingsIcon } from "lucide-react";
import { ConnectionCard } from "./ConnectionCard";
import { ConnectionFormDialog } from "./ConnectionForm";
import {
  ListSavedConnections,
  ConnectUsingSaved,
  DeleteSavedConnection,
} from "wailsjs/go/main/App";
import { services } from "wailsjs/go/models";
import { formatDistanceToNow } from "date-fns";
import SettingsModal from "./SettingModal";

type SavedConnectionsMap = Record<string, services.ConnectionDetails>;

const WelcomeScreen = () => {
  const [savedConnections, setSavedConnections] = useState<SavedConnectionsMap>(
    {},
  );
  const hasConnections = Object.keys(savedConnections).length > 0;
  const [isLoadingConnections, setIsLoadingConnections] = useState(true);
  const [connectingName, setConnectingName] = useState<string | null>(null);
  const [isFormOpen, setIsFormOpen] = useState(false);

  const fetchConnections = useCallback(async () => {
    try {
      const connections = await ListSavedConnections();
      setSavedConnections(connections || {});
    } catch (error: any) {
      console.error("Error fetching saved connections:", error);
      toast.error("Failed to load saved connections", {
        description: error?.message,
      });
      setSavedConnections({});
    } finally {
      if (isLoadingConnections) setIsLoadingConnections(false);
    }
  }, [isLoadingConnections]);

  useEffect(() => {
    fetchConnections();
  }, [fetchConnections]);

  const handleConnect = async (name: string) => {
    setConnectingName(name);
    try {
      await ConnectUsingSaved(name);
    } catch (error: any) {
      console.error(`Connect using ${name} error:`, error);
      toast.error("Connection Failed", { description: error?.message });
    } finally {
      setConnectingName(null);
    }
  };

  const handleDelete = async (name: string) => {
    try {
      await DeleteSavedConnection(name);
      toast.success("Connection Deleted", {
        description: `Connection '${name}' was deleted.`,
      });
      fetchConnections();
    } catch (error: any) {
      console.error(`Delete connection ${name} error:`, error);
      toast.error("Delete Failed", { description: error?.message });
    }
  };

  const handleConnectionSaved = () => {
    fetchConnections();
  };

  // Memoize sorted connections
  const sortedConnections = useMemo(() => {
    return Object.entries(savedConnections).sort(([, a], [, b]) => {
      const timeA = a.lastUsed ? new Date(a.lastUsed).getTime() : 0;
      const timeB = b.lastUsed ? new Date(b.lastUsed).getTime() : 0;
      return timeB - timeA; // Sort descending (most recent first)
    });
  }, [savedConnections]);

  return (
    <div className="w-full min-h-full bg-muted/50 p-6 md:p-10">
      <header className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-2xl md:text-3xl font-bold">
            Welcome to TiDB Desktop
          </h1>
          <p className="text-muted-foreground">
            Connect and manage your TiDB database connections
          </p>
        </div>

        <div className="flex items-center gap-2">
          <Button onClick={() => setIsFormOpen(true)}>
            <PlusCircleIcon className="mr-2 h-4 w-4" /> Add New Connection
          </Button>

          <SettingsModal>
            <Button variant="outline" className="size-9">
              <SettingsIcon className="h-4 w-4" />
              <span className="sr-only">Preferences</span>
            </Button>
          </SettingsModal>
        </div>
      </header>

      <section>
        {isLoadingConnections ? (
          <div className="flex items-center justify-center p-10 text-muted-foreground">
            <Loader2Icon className="h-8 w-8 animate-spin mr-3" />
            <span>Loading connections...</span>
          </div>
        ) : hasConnections ? (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-6">
            {sortedConnections.map(([name, details]) => (
              <ConnectionCard
                key={name}
                name={name}
                details={details}
                onConnect={handleConnect}
                onDelete={handleDelete}
                isConnecting={connectingName === name}
                lastUsed={
                  details.lastUsed
                    ? formatDistanceToNow(new Date(details.lastUsed), {
                        addSuffix: true,
                      })
                    : "Never"
                }
              />
            ))}
          </div>
        ) : (
          <div className="text-center py-10 border border-dashed rounded-lg">
            <h3 className="text-lg font-semibold">No Saved Connections</h3>
            <p className="text-muted-foreground mt-1 mb-4">
              Ready to explore? Add your first connection now.
            </p>
            <Button onClick={() => setIsFormOpen(true)} variant="outline">
              <PlusCircleIcon className="mr-2 h-4 w-4" />
              Add New Connection
            </Button>
          </div>
        )}
      </section>

      <ConnectionFormDialog
        isOpen={isFormOpen}
        onOpenChange={setIsFormOpen}
        onConnectionSaved={handleConnectionSaved}
      />
    </div>
  );
};

export default memo(WelcomeScreen);
