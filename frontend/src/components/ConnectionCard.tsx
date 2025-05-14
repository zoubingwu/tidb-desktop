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
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Clock, Database, Loader, MoreHorizontal } from "lucide-react";
import { useState } from "react";
import { services } from "wailsjs/go/models";

type ConnectionCardProps = {
  name: string;
  details: services.ConnectionDetails;
  onConnect: (name: string) => Promise<void>;
  onDelete: (name: string) => Promise<void>;
  onEdit: (name: string, details: services.ConnectionDetails) => void;
  isConnecting: boolean;
  lastUsed: string;
};

export const ConnectionCard = ({
  name,
  details,
  onConnect,
  onDelete,
  isConnecting,
  lastUsed,
  onEdit,
}: ConnectionCardProps) => {
  const [isDeleting, setIsDeleting] = useState(false);

  const handleDeleteConfirm = async () => {
    setIsDeleting(true);
    await onDelete(name);
    setIsDeleting(false);
  };

  const handleOpenEditForm = () => {
    onEdit(name, details);
  };

  return (
    <Card className="flex flex-col justify-between h-full shadow-sm hover:shadow-md transition-shadow duration-200 gap-4">
      <CardHeader className="">
        <div className="space-y-1 min-w-0">
          <CardTitle className="text-lg break-words">{name}</CardTitle>
          {/* Optional: Add DB Type if available in details later */}
          <CardDescription className="text-sm">TiDB/MySQL</CardDescription>
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
        {/* Last Connected Info */}
        <div className="flex items-center gap-2">
          <Clock className="h-4 w-4 shrink-0" />
          <span>Last used: {lastUsed}</span>
        </div>
      </CardContent>
      <CardFooter className="flex items-center justify-between gap-2">
        {/* Connect Button */}
        <Button
          onClick={() => onConnect(name)}
          disabled={isConnecting || isDeleting}
          className="flex-grow"
        >
          {isConnecting ? (
            <>
              <Loader className="h-4 w-4 animate-spin" />
              Connecting...
            </>
          ) : (
            <>Connect</>
          )}
        </Button>

        {/* Replace Delete Button with Dropdown Menu */}
        <AlertDialog>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="size-8"
                disabled={isConnecting || isDeleting}
              >
                <MoreHorizontal className="h-4 w-4" />
                <span className="sr-only">More options for {name}</span>
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              <DropdownMenuItem
                onClick={handleOpenEditForm}
                disabled={isConnecting || isDeleting}
              >
                Edit
              </DropdownMenuItem>

              <AlertDialogTrigger asChild>
                <DropdownMenuItem
                  className="text-destructive focus:text-destructive focus:bg-destructive/10"
                  disabled={isConnecting || isDeleting}
                >
                  Delete
                </DropdownMenuItem>
              </AlertDialogTrigger>
            </DropdownMenuContent>
          </DropdownMenu>

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
                {isDeleting && <Loader className="h-4 w-4 animate-spin" />}
                Delete
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </CardFooter>
    </Card>
  );
};
