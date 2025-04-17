import { useState, useMemo, useCallback, useEffect, memo } from "react";
import { useImmer } from "use-immer";
import { Loader2, RefreshCw, Columns3, Settings, XIcon } from "lucide-react";
import {
  ColumnDef,
  flexRender,
  getCoreRowModel,
  useReactTable,
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
import { DataTablePagination } from "@/components/DataTablePagination";
import {
  DataTableFilter,
  ServerSideFilter,
} from "@/components/ui/data-table-filter";
import { Button } from "@/components/ui/button";
import { filterFn } from "@/lib/filters";
import { mapDbColumnTypeToFilterType } from "@/lib/utils";
import { ListTables, GetTableData, ListDatabases } from "wailsjs/go/main/App";
import { DatabaseTree, DatabaseTreeItem } from "@/components/DatabaseTree";
import { SettingsModal } from "@/components/SettingModal";
import { toast } from "sonner";

// Use `any` for row data initially, can be refined if needed
type TableRowData = Record<string, any>;

// Rename to avoid conflict with the imported component
type DatabaseTreeData = DatabaseTreeItem[];

const MainDataView = ({
  onClose,
  onUpdateTitle,
}: {
  onClose: () => void;
  onUpdateTitle: (title: string) => void;
}) => {
  const [databaseTree, setDatabaseTree] = useImmer<DatabaseTreeData>([]);
  const [currentTable, setCurrentTable] = useState<{
    db: string;
    table: string;
  } | null>(null);
  const [{ pageIndex, pageSize }, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: 50,
  });
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([]);
  const [rowSelection, setRowSelection] = useState({});
  const [serverFilters, setServerFilters] = useState<ServerSideFilter[]>([]);

  const updateDatabaseTree = (
    dbName: string,
    newItem: Partial<Omit<DatabaseTreeItem, "name">>,
  ) => {
    setDatabaseTree((prevTree: DatabaseTreeData) => {
      const item = prevTree.find((item) => item.name === dbName);
      if (item) {
        item.isLoadingTables = newItem.isLoadingTables;
        item.tables = newItem.tables || [];
      }
    });
  };

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

  useEffect(() => {
    if (databases?.length) {
      setDatabaseTree((draft) => {
        databases.forEach((dbName) => {
          const existingItem = draft.find((item) => item.name === dbName);
          if (existingItem) {
            existingItem.isLoadingTables = false;
          } else {
            draft.push({
              name: dbName,
              tables: [],
              isLoadingTables: false,
            });
          }
        });
        return draft;
      });
    }
  }, [databases]);

  const {
    mutate: fetchTables,
    isPending: isLoadingTables,
    error: tablesError,
  } = useMutation({
    mutationFn: (dbName: string) => ListTables(dbName),
    onMutate: (dbName: string) => {
      if (!databaseTree.find((db) => db.name === dbName)?.tables?.length) {
        updateDatabaseTree(dbName, { isLoadingTables: true });
      }
    },
    onSuccess: (tables, dbName) => {
      updateDatabaseTree(dbName, {
        tables: tables || [],
        isLoadingTables: false,
      });
    },
    onError: (error, dbName) => {
      updateDatabaseTree(dbName, { isLoadingTables: false });
      console.error("Error fetching tables:", error);
    },
  });

  const {
    mutate: fetchTableData,
    isPending: isFetchingTableData,
    data: tableData,
    error: tableDataError,
  } = useMutation({
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
      onUpdateTitle(`Fetching ${currentTable?.db}.${currentTable?.table}...`);
    },
    onSuccess: (_data, variables) => {
      onUpdateTitle(`${variables.dbName}.${variables.tableName}`);
    },
    onError: (error) => {
      onUpdateTitle(
        `Error fetching ${currentTable?.db}.${currentTable?.table}`,
      );
      toast.error("Error fetching table data", {
        description: `Error fetching ${currentTable?.db}.${currentTable?.table}: ${error.message}`,
      });
    },
  });

  // --- Handle server-side filter changes ---
  const handleFilterChange = useCallback(
    (filters: ServerSideFilter[]) => {
      setServerFilters(filters);
      setPagination({ pageIndex: 0, pageSize });

      // Refetch data with new filters if we have a selection
      if (currentTable) {
        fetchTableData({
          tableName: currentTable.table,
          dbName: currentTable.db,
          pageSize,
          pageIndex: 0, // Reset to first page
          filters,
        });
      }
    },
    [currentTable, pageSize],
  );

  // --- Derived State & Calculations ---
  const isRefreshingIndicator = isLoadingTables || isFetchingTableData;
  const isInitialLoading = isLoadingDatabases || isLoadingTables;
  const error = databasesError || tablesError || tableDataError;

  // --- Derive columns and data from table data ---
  const columns = useMemo<ColumnDef<TableRowData>[]>(() => {
    if (!tableData?.columns) return [];

    return [
      // Select Checkbox Column
      // {
      //   id: "select",
      //   header: ({ table }) => (
      //     <Checkbox
      //       checked={
      //         table.getIsAllPageRowsSelected() ||
      //         (table.getIsSomePageRowsSelected() && "indeterminate")
      //       }
      //       onCheckedChange={(value) =>
      //         table.toggleAllPageRowsSelected(!!value)
      //       }
      //       aria-label="Select all"
      //       className="translate-y-[2px]"
      //     />
      //   ),
      //   cell: ({ row }) => (
      //     <Checkbox
      //       checked={row.getIsSelected()}
      //       onCheckedChange={(value) => row.toggleSelected(!!value)}
      //       aria-label="Select row"
      //       className="translate-y-[2px]"
      //     />
      //   ),
      //   enableSorting: false,
      //   enableHiding: false,
      // },
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

  const status = useMemo(() => {
    if (isFetchingTableData) {
      return "Fetching data...";
    }
    if (error) {
      return `Error loading data: ${error.message}`;
    }
    if (currentTable) {
      return `current table: ${currentTable?.db}.${currentTable?.table}`;
    }
    return "No table selected";
  }, [isFetchingTableData, error, currentTable]);

  // Safely update database tree for selected DB
  const handleSelectDatabase = (dbName: string) => {
    fetchTables(dbName);
  };

  // --- Function to handle table selection from tree ---
  const handleSelectTable = (dbName: string, tableName: string) => {
    if (currentTable?.db !== dbName || currentTable?.table !== tableName) {
      // First set the selection
      setCurrentTable({ db: dbName, table: tableName });

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
    if (currentTable) {
      fetchTableData({
        tableName: currentTable.table,
        dbName: currentTable.db,
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

    if (currentTable) {
      fetchTableData({
        tableName: currentTable.table,
        dbName: currentTable.db,
        pageSize: newPagination.pageSize,
        pageIndex: newPagination.pageIndex,
        filters: serverFilters,
      });
    }
  };

  const handleClose = () => {
    setCurrentTable(null);
    onClose();
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
    getPaginationRowModel: getPaginationRowModel(),
  });

  return (
    <div className="h-full flex">
      <DatabaseTree
        databaseTree={databaseTree}
        isLoadingDatabases={isLoadingDatabases}
        databasesError={databasesError}
        onSelectDatabase={handleSelectDatabase}
        onSelectTable={handleSelectTable}
        selectedTable={currentTable}
      />

      <div className="flex-grow flex flex-col overflow-hidden">
        <div className="p-2 flex items-center gap-2 sticky top-0 bg-background z-20">
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8"
            onClick={handleRefresh}
            disabled={isRefreshingIndicator || !currentTable}
          >
            {isRefreshingIndicator ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <RefreshCw className="h-4 w-4" />
            )}
            <span className="sr-only">Refresh</span>
          </Button>

          <DataTableFilter table={table} onChange={handleFilterChange} />

          <SettingsModal>
            <Button variant="ghost" size="icon" className="h-8 w-8">
              <Settings className="h-4 w-4" />
              <span className="sr-only">Settings</span>
            </Button>
          </SettingsModal>

          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8"
            onClick={handleClose}
          >
            <XIcon className="h-4 w-4" />
            <span className="sr-only">Close</span>
          </Button>
        </div>

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
          ) : !currentTable?.table ? (
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
              ) : !currentTable?.table ? (
                <div className="flex-grow flex items-center justify-center text-muted-foreground p-4">
                  Please select a database or table from the sidebar.
                </div>
              ) : currentTable.table === "" ? (
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
                          className="odd:bg-muted/50"
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
                          {currentTable.table
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

        <DataTablePagination
          table={table}
          totalRowCount={totalRowCount}
          status={status}
          disabled={isFetchingTableData}
        />
      </div>
    </div>
  );
};

export default memo(MainDataView);
