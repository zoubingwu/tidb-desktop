import { Tree, Folder, File } from "@/components/ui/file-tree";
import { Table2Icon } from "lucide-react";
import { ScrollArea } from "@/components/ui/scroll-area";
import { memo } from "react";

export type DatabaseTreeItem = {
  name: string;
  tables: string[];
  isLoadingTables?: boolean;
};

export type DatabaseTree = DatabaseTreeItem[];

export type SelectionState = { dbName: string; tableName: string } | null;

type DatabaseTreeProps = {
  databaseTree: DatabaseTree;
  selection: SelectionState;
  isLoadingDatabases: boolean;
  databasesError: Error | null;
  onSelectDatabase: (dbName: string) => void;
  onSelectTable: (dbName: string, tableName: string) => void;
};

export const DatabaseTree = memo(
  ({
    databaseTree,
    selection,
    isLoadingDatabases,
    databasesError,
    onSelectDatabase,
    onSelectTable,
  }: DatabaseTreeProps) => {
    const selectedDbName = selection?.dbName;

    return (
      <ScrollArea className="w-[240px] h-full bg-muted/40">
        {isLoadingDatabases ? (
          <div className="p-4 text-center text-muted-foreground">
            Loading Databases...
          </div>
        ) : databasesError ? (
          <div className="p-4 text-center text-destructive">
            Error loading Databases
          </div>
        ) : (
          <Tree className="p-2">
            {databaseTree.map((dbItem) => (
              <Folder
                key={dbItem.name}
                element={dbItem.name}
                value={dbItem.name}
                onClick={() => onSelectDatabase(dbItem.name)}
              >
                {dbItem.isLoadingTables ? (
                  <File
                    isSelectable={false}
                    value={".loading"}
                    className="text-muted-foreground italic"
                  >
                    Loading...
                  </File>
                ) : dbItem.tables.length > 0 ? (
                  dbItem.tables.map((tbl) => (
                    <File
                      key={tbl}
                      value={tbl}
                      isSelect={
                        selection?.tableName === tbl &&
                        selection?.dbName === dbItem.name
                      }
                      onClick={(e) => {
                        e.stopPropagation();
                        onSelectTable(dbItem.name, tbl);
                      }}
                      fileIcon={<Table2Icon className="size-4" />}
                    >
                      {tbl}
                    </File>
                  ))
                ) : (
                  dbItem.name === selectedDbName &&
                  !dbItem.isLoadingTables && (
                    <File
                      isSelectable={false}
                      value=".no-tables"
                      className="text-muted-foreground italic"
                    >
                      No tables
                    </File>
                  )
                )}
              </Folder>
            ))}
          </Tree>
        )}
      </ScrollArea>
    );
  },
);
