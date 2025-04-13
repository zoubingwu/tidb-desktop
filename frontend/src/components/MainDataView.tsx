import { useState, useEffect, useMemo } from "react";
import { Loader2, Database, Table2Icon } from "lucide-react";
import {
  ColumnDef,
  flexRender,
  getCoreRowModel,
  useReactTable,
  getFilteredRowModel,
  getPaginationRowModel,
  ColumnFiltersState,
  PaginationState,
} from "@tanstack/react-table";
import { useQuery } from "@tanstack/react-query";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Checkbox } from "@/components/ui/checkbox";
import { Tree, Folder, File } from "@/components/ui/file-tree";

import { ListTables, GetTableData, ListDatabases } from "wailsjs/go/main/App";
import { ScrollArea } from "@/components/ui/scroll-area";
import { DataTablePagination } from "./DataTablePagination";

// Type for the Go backend response from GetTableData
// Assuming TableDataResponse structure defined in Go
type TableDataResponse = {
  columns: { name: string; type: string }[];
  rows: Record<string, any>[];
  totalRows?: number;
};

// Use `any` for row data initially, can be refined if needed
type TableRowData = Record<string, any>;

type DatabaseTreeItem = {
  name: string;
  tables: string[];
  isLoadingTables?: boolean; // Track loading state per DB
};
type DatabaseTree = DatabaseTreeItem[];

const SystemDatabases = [
  "PERFORMANCE_SCHEMA",
  "INFORMATION_SCHEMA",
  "mysql",
  "sys",
]; // Add others if needed

// Define the structure for the unified selection state
type SelectionState =
  | { type: "database"; dbName: string; tableName?: null } // Database selected, no table yet
  | { type: "table"; dbName: string; tableName: string } // Specific table selected
  | null; // Nothing selected

const MainDataView = () => {
  const [databaseTree, setDatabaseTree] = useState<DatabaseTree>([]);
  const [selection, setSelection] = useState<SelectionState>(null);
  const [{ pageIndex, pageSize }, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: 50,
  });
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([]);
  const [rowSelection, setRowSelection] = useState({});

  // Convenience accessors (optional, but can make code clearer)
  const selectedDbName = selection?.dbName;
  const selectedTableName =
    selection?.type === "table" ? selection.tableName : null;

  // --- TanStack Query for fetching databases ---
  const {
    data: databases = [],
    isLoading: isLoadingDatabases,
    error: databasesError,
  } = useQuery<string[], Error>({
    queryKey: ["databases"],
    queryFn: ListDatabases,
    staleTime: 15 * 60 * 1000, // Cache for 15 minutes
    refetchOnWindowFocus: false,
    // Filter out system databases immediately
    select: (data) =>
      data.filter((db) => !SystemDatabases.includes(db.toUpperCase())),
  });

  // --- TanStack Query for fetching tables ---
  const {
    data: tables,
    isLoading: isLoadingTables,
    error: tablesError,
  } = useQuery<string[], Error>({
    queryKey: ["tables", selectedDbName],
    queryFn: () => ListTables(selectedDbName!),
    enabled: !!selectedDbName,
    staleTime: 5 * 60 * 1000, // Cache for 5 minutes
    refetchOnWindowFocus: false,
  });

  // --- TanStack Query for fetching table data ---
  const {
    data: tableDataResponse,
    isLoading: isLoadingData,
    error: dataError,
  } = useQuery<
    TableDataResponse | null, // Can be null if GetTableData returns null
    Error,
    TableDataResponse | null
  >({
    queryKey: [
      "tableData",
      selectedDbName,
      selectedTableName,
      pageIndex,
      pageSize,
    ],
    queryFn: async () => {
      if (!selectedTableName || !selectedDbName) return null;
      return await GetTableData(
        selectedTableName,
        pageSize,
        pageIndex * pageSize,
      );
    },
    enabled: !!selectedDbName && !!selectedTableName,
    placeholderData: (previousData) => previousData ?? undefined,
    staleTime: 1 * 60 * 1000, // Cache data for 1 minute
    refetchOnWindowFocus: false,
  });

  // --- Effects ---

  // Initialize/Update databaseTree when databases load
  useEffect(() => {
    setDatabaseTree((currentTree) => {
      const newTree: DatabaseTree = databases.map((dbName) => {
        const existingItem = currentTree.find((item) => item.name === dbName);
        return {
          name: dbName,
          tables: existingItem?.tables ?? [],
          isLoadingTables: existingItem?.isLoadingTables ?? false,
        };
      });
      return newTree;
    });

    // Handle case where selected DB is no longer in the list
    if (
      selectedDbName &&
      databases.length > 0 &&
      !databases.includes(selectedDbName)
    ) {
      setSelection(null); // Reset selection entirely
    }
    // Handle case where there are no databases at all
    else if (databases.length === 0 && !isLoadingDatabases) {
      setSelection(null);
    }
  }, [databases, isLoadingDatabases, selectedDbName]); // Depend on selectedDbName

  // Update the tree with fetched tables for the *selected* database
  useEffect(() => {
    // We update the tree based on the DB derived from the query key, which is selectedDbName
    if (selectedDbName && !isLoadingTables) {
      setDatabaseTree((currentTree) => {
        return currentTree.map((item) => {
          if (item.name === selectedDbName) {
            // Ensure tables is always an array here
            return { ...item, tables: tables ?? [], isLoadingTables: false };
          }
          return item;
        });
      });

      // Check if the current TABLE selection is still valid within the *updated* tables list
      if (selection?.type === "table") {
        const currentTables = tables ?? []; // Use the fetched tables
        const tableStillExists = currentTables.includes(selection.tableName);

        if (!tableStillExists || currentTables.length === 0) {
          // Table is no longer valid, revert selection to only the database
          // This state change is okay, it reflects reality and won't immediately loop
          // because the condition !tableStillExists depends on `tables` data changing.
          setSelection({ type: "database", dbName: selectedDbName });
        }
      }
      // If selection is 'database', no need to check table validity here
    } else if (selectedDbName && isLoadingTables) {
      // Mark the selected database as loading tables (only update if needed)
      setDatabaseTree((currentTree) => {
        let changed = false;
        const newTree = currentTree.map((item) => {
          if (item.name === selectedDbName && !item.isLoadingTables) {
            changed = true;
            return { ...item, isLoadingTables: true };
          }
          return item;
        });
        return changed ? newTree : currentTree; // Avoid state update if already loading
      });
    }

    // Dependencies: selectedDbName triggers fetch, tables/isLoadingTables process results,
    // selection is needed to check validity of selected table *after* tables load.
  }, [selectedDbName, tables, isLoadingTables, selection]);

  // --- Derive columns and data from query result ---
  const columns = useMemo<ColumnDef<TableRowData>[]>(() => {
    if (!tableDataResponse?.columns) return [];

    return [
      // Select Checkbox Column
      {
        id: "select",
        header: ({ table }) => (
          <Checkbox
            checked={
              table.getIsAllPageRowsSelected() ||
              (table.getIsSomePageRowsSelected() && "indeterminate")
            }
            onCheckedChange={(value) =>
              table.toggleAllPageRowsSelected(!!value)
            }
            aria-label="Select all"
            className="translate-y-[2px]"
          />
        ),
        cell: ({ row }) => (
          <Checkbox
            checked={row.getIsSelected()}
            onCheckedChange={(value) => row.toggleSelected(!!value)}
            aria-label="Select row"
            className="translate-y-[2px]"
          />
        ),
        enableSorting: false,
        enableHiding: false,
      },
      // Data Columns
      ...(tableDataResponse.columns.map(
        (col): ColumnDef<TableRowData> => ({
          accessorKey: col.name,
          header: col.name,
          cell: (info) => {
            const value = info.getValue();
            if (value === null || value === undefined) {
              // Style NULL values
              return <span className="text-muted-foreground italic">NULL</span>;
            }
            if (value === "") {
              // Style empty strings differently
              return (
                <span className="text-muted-foreground italic">(empty)</span>
              );
            }
            // Render other values as strings
            return String(value);
          },
          meta: {
            displayName: col.name,
            type: col.type.toLowerCase().includes("int")
              ? "number"
              : col.type.toLowerCase().includes("date") ||
                  col.type.toLowerCase().includes("time")
                ? "date"
                : "text",
            icon: Database,
          },
        }),
      ) || []),
    ];
  }, [tableDataResponse?.columns]);

  const data = useMemo(
    () => tableDataResponse?.rows ?? [],
    [tableDataResponse?.rows],
  );

  // --- Calculate pagination values ---
  const pagination = useMemo(
    () => ({ pageIndex, pageSize }),
    [pageIndex, pageSize],
  );
  const totalRowCount = tableDataResponse?.totalRows;
  const pageCount = useMemo(() => {
    if (totalRowCount != null && totalRowCount >= 0) {
      return Math.ceil(totalRowCount / pageSize);
    }
    // Fallback estimation if totalRowCount is not available
    // -1 tells the table instance pagination controls might be inaccurate
    return -1;
  }, [totalRowCount, pageSize]);

  // --- TanStack Table Instance ---
  const table = useReactTable({
    data,
    columns,
    state: {
      columnFilters,
      pagination,
      rowSelection,
    },
    manualPagination: true,
    pageCount, // Use accurate or estimated page count
    onPaginationChange: setPagination,
    onColumnFiltersChange: setColumnFilters,
    onRowSelectionChange: setRowSelection,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    // debugTable: true,
  });

  // Handle and display errors
  const error = databasesError || tablesError || dataError;

  // Combined loading state
  const isLoading =
    isLoadingDatabases ||
    (selectedDbName && isLoadingTables) ||
    (selectedTableName && isLoadingData && !table.getRowModel().rows.length);

  // Function to handle database selection from tree
  const handleSelectDatabase = (dbName: string) => {
    if (dbName !== selectedDbName) {
      setSelection({ type: "database", dbName: dbName }); // Select the database
      setPagination({ pageIndex: 0, pageSize }); // Reset pagination
      // Table fetching will be triggered by the tablesQuery enabling
    }
  };

  // Function to handle table selection from tree
  const handleSelectTable = (dbName: string, tableName: string) => {
    if (
      selection?.type !== "table" ||
      selection.dbName !== dbName ||
      selection.tableName !== tableName
    ) {
      setSelection({ type: "table", dbName: dbName, tableName: tableName });
      setPagination({ pageIndex: 0, pageSize }); // Reset pagination
    }
  };

  return (
    <div className="h-full flex">
      <ScrollArea className="w-[240px] h-full bg-muted/40">
        {isLoadingDatabases ? (
          <div className="p-4 text-center text-muted-foreground">
            Loading Databases...
          </div>
        ) : databasesError ? (
          <div className="p-4 text-center text-destructive">
            Error loading DBs
          </div>
        ) : (
          <Tree className="p-2">
            {databaseTree.map((dbItem) => (
              <Folder
                key={dbItem.name}
                element={dbItem.name}
                value={dbItem.name}
                isSelectable={true}
                isSelect={selection?.dbName === dbItem.name}
                onClick={() => handleSelectDatabase(dbItem.name)}
              >
                {dbItem.isLoadingTables ? (
                  <File
                    isSelectable={false}
                    value={".loading"}
                    className="text-muted-foreground italic"
                  >
                    <Loader2 className="size-4 mr-2 animate-spin" /> Loading...
                  </File>
                ) : dbItem.tables.length > 0 ? (
                  dbItem.tables.map((tbl) => (
                    <File
                      key={tbl}
                      value={tbl}
                      isSelect={
                        selection?.type === "table" &&
                        selection.tableName === tbl &&
                        selection.dbName === dbItem.name
                      }
                      onClick={(e) => {
                        e.stopPropagation();
                        handleSelectTable(dbItem.name, tbl);
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

      <div className="flex-grow flex flex-col overflow-hidden">
        <div className="rounded-none overflow-hidden flex-grow flex flex-col">
          {isLoading && !tableDataResponse ? (
            <div className="flex-grow flex items-center justify-center text-muted-foreground">
              <Loader2 className="h-8 w-8 animate-spin mr-3" /> Loading...
            </div>
          ) : error ? (
            <div className="flex-grow flex items-center justify-center text-destructive p-4">
              Error:{" "}
              {error instanceof Error
                ? error.message
                : "An unknown error occurred"}
            </div>
          ) : !selectedDbName ? (
            <div className="flex-grow flex items-center justify-center text-muted-foreground p-4">
              Please select a database.
            </div>
          ) : !selectedTableName && tables?.length ? (
            <div className="flex-grow flex items-center justify-center text-muted-foreground p-4">
              Please select a table.
            </div>
          ) : (
            <div className="flex-grow overflow-auto relative">
              <Table>
                <TableHeader className="sticky top-0 bg-background z-10 shadow-sm">
                  {table.getHeaderGroups().map((headerGroup) => (
                    <TableRow key={headerGroup.id}>
                      {headerGroup.headers.map((header) => (
                        <TableHead
                          key={header.id}
                          style={{ width: header.getSize() }}
                          className="px-4"
                        >
                          {header.isPlaceholder
                            ? null
                            : flexRender(
                                header.column.columnDef.header,
                                header.getContext(),
                              )}
                        </TableHead>
                      ))}
                    </TableRow>
                  ))}
                </TableHeader>
                <TableBody>
                  {table.getRowModel().rows?.length ? (
                    table.getRowModel().rows.map((row) => (
                      <TableRow
                        key={row.id}
                        data-state={row.getIsSelected() && "selected"}
                      >
                        {row.getVisibleCells().map((cell) => (
                          <TableCell
                            key={cell.id}
                            style={{ width: cell.column.getSize() }}
                            className="max-w-[250px] truncate px-4"
                          >
                            {flexRender(
                              cell.column.columnDef.cell,
                              cell.getContext(),
                            )}
                          </TableCell>
                        ))}
                      </TableRow>
                    ))
                  ) : (
                    <TableRow>
                      <TableCell
                        colSpan={columns.length}
                        className="h-24 text-center px-4"
                      >
                        {isLoadingData
                          ? "Loading data..."
                          : selectedTableName
                            ? "No results found."
                            : "Select a table."}
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </div>
          )}
        </div>

        {selection?.type === "table" && !isLoading && !error && (
          <DataTablePagination table={table} totalRowCount={totalRowCount} />
        )}
      </div>
    </div>
  );
};

export default MainDataView;
