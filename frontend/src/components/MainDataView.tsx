import { useState, useEffect, useMemo } from "react";
import { Loader2, Table2Icon, RefreshCw, Columns3 } from "lucide-react";
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
import { ScrollArea } from "@/components/ui/scroll-area";
import { DataTablePagination } from "@/components/DataTablePagination";
import {
  DataTableFilter,
  ServerSideFilter,
} from "@/components/ui/data-table-filter";
import { Button } from "@/components/ui/button";
import { filterFn } from "@/lib/filters";
import { mapDbColumnTypeToFilterType } from "@/lib/utils";
import { ListTables, GetTableData, ListDatabases } from "wailsjs/go/main/App";

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

// Define the structure for the unified selection state
type SelectionState =
  | { dbName: string; tableName: string } // Specific table selected
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
  // New state for server-side filters
  const [serverFilters, setServerFilters] = useState<ServerSideFilter[]>([]);

  // Convenience accessors (optional, but can make code clearer)
  const selectedDbName = selection?.dbName;
  const selectedTableName = selection?.tableName;

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
  });

  // --- TanStack Query for fetching tables ---
  const {
    data: tables = [],
    isLoading: isLoadingTables,
    isFetching: isFetchingTables,
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
    refetch: refetchTableData,
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
      // Include serverFilters in the query key
      serverFilters,
    ],
    queryFn: async () => {
      if (!selectedTableName || !selectedDbName) return null;

      // Pass filters to the backend as the fourth parameter
      const filterObject =
        serverFilters.length > 0 ? { filters: serverFilters } : null;
      return await GetTableData(
        selectedTableName,
        pageSize,
        pageIndex * pageSize,
        filterObject, // Pass filterObject as the 4th parameter
      );
    },
    enabled: !!selectedDbName && !!selectedTableName,
    placeholderData: (previousData) => previousData ?? undefined,
    staleTime: 1 * 60 * 1000, // Cache data for 1 minute
    refetchOnWindowFocus: false,
  });

  // Effect 1: Initialize/Update databaseTree when databases load & handle DB selection validity
  useEffect(() => {
    if (databases?.length) {
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
    }
  }, [databases]);

  // Effect 2: Update the tree with fetched tables for the *selected* database
  useEffect(() => {
    if (selectedDbName && !isLoadingTables && tables.length > 0) {
      setDatabaseTree((currentTree) => {
        return currentTree.map((item) => {
          if (item.name === selectedDbName) {
            return { ...item, tables: tables ?? [], isLoadingTables: false };
          }
          return item;
        });
      });
    } else if (selectedDbName && isLoadingTables) {
      setDatabaseTree((currentTree) => {
        let changed = false;
        const newTree = currentTree.map((item) => {
          if (item.name === selectedDbName && !item.isLoadingTables) {
            changed = true;
            return { ...item, isLoadingTables: true };
          }
          return item;
        });
        return changed ? newTree : currentTree;
      });
    }
  }, [selectedDbName, tables, isLoadingTables]);

  // --- Handle server-side filter changes ---
  const handleFilterChange = (filters: ServerSideFilter[]) => {
    // Update the server filters state
    setServerFilters(filters);
    // Reset pagination when filters change
    setPagination({ pageIndex: 0, pageSize });
  };

  // --- Derived State & Calculations ---
  const isRefreshingIndicator =
    (!!selectedDbName && isLoadingTables) ||
    (!!selectedTableName && isLoadingData);
  const isInitialLoading =
    isLoadingDatabases ||
    (!!selectedDbName && isLoadingTables) ||
    (!!selectedTableName && isLoadingData);

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
          filterFn: filterFn(mapDbColumnTypeToFilterType(col.type)),
          meta: {
            displayName: col.name,
            type: mapDbColumnTypeToFilterType(col.type),
            icon: Columns3,
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
    // Add manualFiltering to indicate we're handling filtering on the server
    manualFiltering: true,
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

  // Function to handle database selection from tree
  const handleSelectDatabase = (dbName: string) => {
    if (dbName !== selectedDbName) {
      setSelection({ dbName: dbName, tableName: "" });
    }
  };

  // Function to handle table selection from tree
  const handleSelectTable = (dbName: string, tableName: string) => {
    if (selection?.dbName !== dbName || selection?.tableName !== tableName) {
      setSelection({ dbName: dbName, tableName: tableName });
      setPagination({ pageIndex: 0, pageSize }); // Reset pagination
      // Reset filters when selecting a new table
      setServerFilters([]);
      setColumnFilters([]);
    }
  };

  const handleRefresh = () => {
    if (selectedTableName && selectedDbName) {
      refetchTableData();
    }
  };

  // --- Render Logic ---
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
                        selection?.tableName === tbl &&
                        selection?.dbName === dbItem.name
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
        {selection?.tableName && columns.length > 1 && (
          <div className="p-2 flex items-center gap-2 sticky top-0 bg-background z-20">
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8"
              onClick={handleRefresh}
              disabled={
                isRefreshingIndicator || (!selectedDbName && !selectedTableName)
              }
            >
              {isRefreshingIndicator ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <RefreshCw className="h-4 w-4" />
              )}
              <span className="sr-only">Refresh</span>
            </Button>

            <DataTableFilter table={table} onChange={handleFilterChange} />
          </div>
        )}

        <div className="rounded-none overflow-hidden flex-grow flex flex-col">
          {isInitialLoading ? (
            <div className="flex-grow flex items-center justify-center text-muted-foreground">
              <Loader2 className="h-8 w-8 animate-spin mr-3" /> Loading...
            </div>
          ) : error ? (
            <div className="flex-grow flex items-center justify-center text-destructive p-4">
              Error:{" "}
              {(error as any) instanceof Error
                ? (error as Error).message
                : String(error ?? "An unknown error occurred")}
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
              {error ? (
                <div className="flex-grow flex items-center justify-center text-destructive p-4">
                  Error:{" "}
                  {(error as any) instanceof Error
                    ? (error as Error).message
                    : String(error ?? "An unknown error occurred")}
                </div>
              ) : !selection ? (
                <div className="flex-grow flex items-center justify-center text-muted-foreground p-4">
                  Please select a database or table from the sidebar.
                </div>
              ) : selection.tableName === "" && !isFetchingTables ? (
                <div className="flex-grow flex items-center justify-center text-muted-foreground p-4">
                  Please select a table from the sidebar.
                </div>
              ) : (
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
              )}
            </div>
          )}
        </div>

        {selection?.tableName && !isLoadingData && !error && (
          <DataTablePagination table={table} totalRowCount={totalRowCount} />
        )}
      </div>
    </div>
  );
};

export default MainDataView;
