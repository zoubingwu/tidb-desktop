import { useState, useEffect, useCallback } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Loader2, PlusCircle } from "lucide-react";
import { ConnectionCard } from "./ConnectionCard";
import { ConnectionFormDialog } from "./ConnectionForm";
import {
  ListSavedConnections,
  ConnectUsingSaved,
  DeleteSavedConnection,
} from "wailsjs/go/main/App";
import { services } from "wailsjs/go/models";

type SavedConnectionsMap = Record<string, services.ConnectionDetails>;

const WelcomeScreen = () => {
  const [savedConnections, setSavedConnections] = useState<SavedConnectionsMap>(
    {},
  );
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

  return (
    <div className="w-full min-h-screen bg-muted/30 p-6 md:p-10">
      <header className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-2xl md:text-3xl font-bold">
            Welcome to TiDB Desktop
          </h1>
          <p className="text-muted-foreground">
            Connect and manage your TiDB database connections
          </p>
        </div>
        <Button onClick={() => setIsFormOpen(true)}>
          <PlusCircle className="mr-2 h-4 w-4" /> Add New Connection
        </Button>
      </header>

      <section>
        {isLoadingConnections ? (
          <div className="flex items-center justify-center p-10 text-muted-foreground">
            <Loader2 className="h-8 w-8 animate-spin mr-3" />
            <span>Loading connections...</span>
          </div>
        ) : Object.keys(savedConnections).length > 0 ? (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-6">
            {Object.entries(savedConnections).map(([name, details]) => (
              <ConnectionCard
                key={name}
                name={name}
                details={details}
                onConnect={handleConnect}
                onDelete={handleDelete}
                isConnecting={connectingName === name}
              />
            ))}
          </div>
        ) : (
          <div className="text-center py-10 border border-dashed rounded-lg">
            <h3 className="text-lg font-semibold">No Saved Connections</h3>
            <p className="text-muted-foreground mt-1 mb-4">
              Click "Add New Connection" to get started.
            </p>
            <Button onClick={() => setIsFormOpen(true)} variant="outline">
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

export default WelcomeScreen;
