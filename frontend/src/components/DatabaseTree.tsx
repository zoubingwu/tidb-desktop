import { File, Folder, Tree } from "@/components/ui/file-tree";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Table2Icon } from "lucide-react";
import { memo } from "react";

export type DatabaseTreeItem = {
  name: string;
  tables: string[];
  isLoadingTables?: boolean;
};

export type DatabaseTree = DatabaseTreeItem[];

type DatabaseTreeProps = {
  databaseTree: DatabaseTree;
  isLoadingDatabases: boolean;
  databasesError: Error | null;
  onSelectDatabase: (dbName: string) => void;
  onSelectTable: (dbName: string, tableName: string) => void;
  selectedTable: { db: string; table: string } | null;
};

export const DatabaseTree = memo(
  ({
    databaseTree,
    isLoadingDatabases,
    databasesError,
    onSelectDatabase,
    onSelectTable,
    selectedTable,
  }: DatabaseTreeProps) => {
    return (
      <ScrollArea className="w-[240px] flex-shrink-0 h-full bg-muted/50">
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
                onExpand={onSelectDatabase}
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
                        selectedTable?.db === dbItem.name &&
                        selectedTable?.table === tbl
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
