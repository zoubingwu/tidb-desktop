import { useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Loader2, Trash2, Database } from "lucide-react";
import { services } from "wailsjs/go/models";

type ConnectionCardProps = {
  name: string;
  details: services.ConnectionDetails;
  onConnect: (name: string) => Promise<void>; // Pass connect logic from parent
  onDelete: (name: string) => Promise<void>; // Pass delete logic from parent
  // onEdit: (name: string, details: services.ConnectionDetails) => void; // Placeholder for edit
  isConnecting: boolean; // Flag from parent indicating *this* card is connecting
};

export const ConnectionCard = ({
  name,
  details,
  onConnect,
  onDelete,
  isConnecting,
}: ConnectionCardProps) => {
  const [isDeleting, setIsDeleting] = useState(false); // Local delete loading state

  const handleDeleteConfirm = async () => {
    setIsDeleting(true);
    await onDelete(name); // Call parent delete handler
    // Parent handler shows toast and triggers refetch
    setIsDeleting(false); // Reset local state (dialog closes automatically)
  };

  return (
    <Card className="flex flex-col justify-between h-full shadow-sm hover:shadow-md transition-shadow duration-200 gap-4">
      <CardHeader className="">
        <div className="space-y-1 min-w-0">
          <CardTitle className="text-lg break-words">{name}</CardTitle>
          {/* Optional: Add DB Type if available in details later */}
          <CardDescription className="text-sm">TiDB / MySQL</CardDescription>
        </div>
      </CardHeader>
      <CardContent className="flex-grow space-y-2 text-sm text-muted-foreground">
        {/* Host Info */}
        <div className="flex items-center gap-2">
          <Database className="h-4 w-4 shrink-0" />
          <span className="truncate">
            {details.host}:{details.port} ({details.user})
          </span>
        </div>
        {/* Last Connected Info (Placeholder) */}
        {/* <div className="flex items-center gap-2">
                     <Clock className="h-4 w-4" />
                     <span>Last connected: 2 days ago</span>
                 </div> */}
      </CardContent>
      <CardFooter className="flex items-center justify-between gap-2">
        {/* Connect Button */}
        <Button
          className="flex-grow"
          onClick={() => onConnect(name)}
          disabled={isConnecting || isDeleting}
        >
          {isConnecting && <Loader2 className="h-4 w-4 animate-spin" />}
          Connect
        </Button>

        {/* Delete Button with Confirmation */}
        <AlertDialog>
          <AlertDialogTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="text-destructive hover:text-destructive border-destructive/50 hover:border-destructive/80 h-9 w-9"
              disabled={isConnecting || isDeleting}
            >
              {isDeleting ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Trash2 className="h-4 w-4" />
              )}
              <span className="sr-only">Delete {name}</span>
            </Button>
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Are you sure?</AlertDialogTitle>
              <AlertDialogDescription>
                This action cannot be undone. Delete connection '{name}'?
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>Cancel</AlertDialogCancel>
              <AlertDialogAction
                onClick={handleDeleteConfirm}
                disabled={isDeleting}
                className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              >
                {isDeleting && (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                )}
                Delete
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </CardFooter>
    </Card>
  );
};
