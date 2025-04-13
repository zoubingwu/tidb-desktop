import { useState, useMemo, useCallback } from "react";
import { Loader2, RefreshCw, Columns3 } from "lucide-react";
import {
  ColumnDef,
  flexRender,
  getCoreRowModel,
  useReactTable,
  getFilteredRowModel,
  getPaginationRowModel,
  ColumnFiltersState,
  PaginationState,
  Updater,
} from "@tanstack/react-table";
import { useQuery, useMutation } from "@tanstack/react-query";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Checkbox } from "@/components/ui/checkbox";
import { DataTablePagination } from "@/components/DataTablePagination";
import {
  DataTableFilter,
  ServerSideFilter,
} from "@/components/ui/data-table-filter";
import { Button } from "@/components/ui/button";
import { filterFn } from "@/lib/filters";
import { mapDbColumnTypeToFilterType } from "@/lib/utils";
import { ListTables, GetTableData, ListDatabases } from "wailsjs/go/main/App";
import {
  DatabaseTree,
  DatabaseTreeItem,
  SelectionState as TreeSelectionState,
} from "@/components/DatabaseTree";

// Type for the Go backend response from GetTableData
// Assuming TableDataResponse structure defined in Go
type TableDataResponse = {
  columns: { name: string; type: string }[];
  rows: Record<string, any>[];
  totalRows?: number;
};

// Use `any` for row data initially, can be refined if needed
type TableRowData = Record<string, any>;

// Rename to avoid conflict with the imported component
type DatabaseTreeData = DatabaseTreeItem[];
// Reuse the SelectionState from the DatabaseTree component
type SelectionState = TreeSelectionState;

const MainDataView = () => {
  const [databaseTree, setDatabaseTree] = useState<DatabaseTreeData>([]);
  const [selection, setSelection] = useState<SelectionState>(null);
  const [{ pageIndex, pageSize }, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: 50,
  });
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([]);
  const [rowSelection, setRowSelection] = useState({});
  const [serverFilters, setServerFilters] = useState<ServerSideFilter[]>([]);

  // Store table data in state
  const [tableData, setTableData] = useState<TableDataResponse | null>(null);
  const [loadingTableData, setLoadingTableData] = useState<boolean>(false);
  const [tableDataError, setTableDataError] = useState<Error | null>(null);

  console.log("tableData", tableData);

  // Convenience accessors
  const selectedDbName = selection?.dbName;
  const selectedTableName = selection?.tableName;

  // --- Databases Query (keeps automatic fetching for initial page load) ---
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

  // --- Convert Tables Query to a Mutation ---
  const {
    mutate: fetchTables,
    isPending: isLoadingTables,
    error: tablesError,
  } = useMutation({
    mutationFn: (dbName: string) => ListTables(dbName),
    onSuccess: (tables, dbName) => {
      // Update database tree with fetched tables
      setDatabaseTree((currentTree) =>
        currentTree.map((item) => {
          if (item.name === dbName) {
            return { ...item, tables: tables || [], isLoadingTables: false };
          }
          return item;
        }),
      );
    },
    onError: (error) => {
      console.error("Error fetching tables:", error);
    },
  });

  // --- Convert Table Data Query to a Mutation ---
  const { mutate: fetchTableData, isPending: isFetchingTableData } =
    useMutation({
      mutationFn: ({
        tableName,
        dbName,
        pageSize,
        pageIndex,
        filters,
      }: {
        tableName: string;
        dbName: string;
        pageSize: number;
        pageIndex: number;
        filters: ServerSideFilter[];
      }) => {
        const filterObject = filters.length > 0 ? { filters } : null;
        return GetTableData(
          dbName,
          tableName,
          pageSize,
          pageIndex * pageSize,
          filterObject,
        );
      },
      onMutate: () => {
        setLoadingTableData(true);
        setTableDataError(null);
      },
      onSuccess: (data) => {
        console.log("onSuccess data", data);
        setTableData(data);
        setLoadingTableData(false);
      },
      onError: (error) => {
        setTableDataError(error as Error);
        setLoadingTableData(false);
        console.error("Error fetching table data:", error);
      },
    });

  // --- Handle server-side filter changes ---
  const handleFilterChange = useCallback(
    (filters: ServerSideFilter[]) => {
      setServerFilters(filters);
      setPagination({ pageIndex: 0, pageSize });

      // Refetch data with new filters if we have a selection
      if (selectedTableName && selectedDbName) {
        fetchTableData({
          tableName: selectedTableName,
          dbName: selectedDbName,
          pageSize,
          pageIndex: 0, // Reset to first page
          filters,
        });
      }
    },
    [selectedTableName, selectedDbName, pageSize],
  );

  // --- Derived State & Calculations ---
  const isRefreshingIndicator = isLoadingTables || isFetchingTableData;
  const isInitialLoading =
    isLoadingDatabases || isLoadingTables || loadingTableData;
  const error = databasesError || tablesError || tableDataError;

  // --- Derive columns and data from table data ---
  const columns = useMemo<ColumnDef<TableRowData>[]>(() => {
    if (!tableData?.columns) return [];

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
      ...(tableData.columns.map(
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
  }, [tableData?.columns]);

  const data = useMemo(() => tableData?.rows ?? [], [tableData?.rows]);
  const totalRowCount = tableData?.totalRows;

  // --- Calculate pagination values ---
  const pagination = useMemo(
    () => ({ pageIndex, pageSize }),
    [pageIndex, pageSize],
  );

  const pageCount = useMemo(() => {
    if (totalRowCount != null && totalRowCount >= 0) {
      return Math.ceil(totalRowCount / pageSize);
    }
    return -1;
  }, [totalRowCount, pageSize]);

  // --- Initialize database tree when databases load ---
  useMemo(() => {
    if (databases?.length) {
      // Only set database tree if it's empty or if databases have changed
      const currentDbNames = databaseTree.map((item) => item.name);
      const newDbNames = databases;

      // Check if the database list has changed
      const hasChanges =
        currentDbNames.length !== newDbNames.length ||
        newDbNames.some((dbName) => !currentDbNames.includes(dbName));

      if (hasChanges) {
        setDatabaseTree((prevTree) => {
          // Preserve existing tree data when possible
          return databases.map((dbName) => {
            const existingItem = prevTree.find((item) => item.name === dbName);
            // Keep existing table data if we had it
            if (existingItem) {
              return existingItem;
            }
            // Create new item if this database is new
            return {
              name: dbName,
              tables: [],
              isLoadingTables: false,
            };
          });
        });
      }
    }
    // Don't include databaseTree in dependencies to avoid re-render loops
  }, [databases]);

  // Safely update database tree for selected DB
  const handleSelectDatabase = (dbName: string) => {
    if (dbName !== selectedDbName) {
      // First set the selection
      setSelection({ dbName, tableName: "" });

      // Update the loading state for the specific DB without triggering a loop
      setDatabaseTree((prevTree) =>
        prevTree.map((item) => {
          if (item.name === dbName && !item.isLoadingTables) {
            return { ...item, isLoadingTables: true };
          }
          return item;
        }),
      );

      // Fetch tables for the selected database
      fetchTables(dbName);
    }
  };

  // --- Function to handle table selection from tree ---
  const handleSelectTable = (dbName: string, tableName: string) => {
    if (selection?.dbName !== dbName || selection?.tableName !== tableName) {
      // First set the selection
      setSelection({ dbName, tableName });

      // Reset filters and pagination
      const newFilters: ServerSideFilter[] = [];
      setServerFilters(newFilters);
      setColumnFilters([]);

      const newPageIndex = 0;
      setPagination({ pageIndex: newPageIndex, pageSize });

      // Only fetch table data if tableName is provided
      if (tableName) {
        fetchTableData({
          tableName,
          dbName,
          pageSize,
          pageIndex: newPageIndex,
          filters: newFilters,
        });
      }
    }
  };

  const handleRefresh = () => {
    if (selectedTableName && selectedDbName) {
      fetchTableData({
        tableName: selectedTableName,
        dbName: selectedDbName,
        pageSize,
        pageIndex,
        filters: serverFilters,
      });
    }
  };

  const handlePaginationChange = (updaterOrValue: Updater<PaginationState>) => {
    // Handle both function updater and direct value
    const newPagination =
      typeof updaterOrValue === "function"
        ? updaterOrValue(pagination)
        : updaterOrValue;

    setPagination(newPagination);

    if (selectedTableName && selectedDbName) {
      fetchTableData({
        tableName: selectedTableName,
        dbName: selectedDbName,
        pageSize: newPagination.pageSize,
        pageIndex: newPagination.pageIndex,
        filters: serverFilters,
      });
    }
  };

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
    manualFiltering: true,
    pageCount,
    onPaginationChange: handlePaginationChange,
    onColumnFiltersChange: setColumnFilters,
    onRowSelectionChange: setRowSelection,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
  });

  // Also fix the render condition for tables
  const getTablesForCurrentDb = () => {
    const currentDb = databaseTree.find((db) => db.name === selectedDbName);
    return currentDb?.tables || [];
  };

  return (
    <div className="h-full flex">
      <DatabaseTree
        databaseTree={databaseTree}
        selection={selection}
        isLoadingDatabases={isLoadingDatabases}
        databasesError={databasesError}
        onSelectDatabase={handleSelectDatabase}
        onSelectTable={handleSelectTable}
      />

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
          ) : !selectedTableName && getTablesForCurrentDb().length ? (
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
              ) : selection.tableName === "" ? (
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
                          {loadingTableData
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

        {selection?.tableName && !loadingTableData && !error && (
          <DataTablePagination table={table} totalRowCount={totalRowCount} />
        )}
      </div>
    </div>
  );
};

export default MainDataView;
