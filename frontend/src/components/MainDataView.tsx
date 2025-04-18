import { useState, useMemo, useCallback, useEffect, memo } from "react";
import { useImmer } from "use-immer";
import { Columns3Icon, SettingsIcon, UnplugIcon } from "lucide-react";
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
import { filterFn } from "@/lib/filters";
import { mapDbColumnTypeToFilterType } from "@/lib/utils";
import { ListTables, GetTableData, ListDatabases } from "wailsjs/go/main/App";
import { DatabaseTree, DatabaseTreeItem } from "@/components/DatabaseTree";
import { toast } from "sonner";
import TablePlaceholder from "./TablePlaceHolder";
import { SettingsModal } from "./SettingModal";
import { Button } from "@/components/ui/button";

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

  const { mutate: fetchTables } = useMutation({
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
    onMutate: (variables) => {
      onUpdateTitle(
        `Fetching data from ${variables.dbName}.${variables.tableName}...`,
      );
    },
    onSuccess: (_data, variables) => {
      onUpdateTitle(`${variables.dbName}.${variables.tableName}`);
    },
    onError: (error, variables) => {
      onUpdateTitle(
        `Error fetching ${variables.dbName}.${variables.tableName}`,
      );
      toast.error("Error fetching table data", {
        description: `Error fetching ${variables.dbName}.${variables.tableName}: ${error}`,
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

  // --- Derive columns and data from table data ---
  const columns = useMemo<ColumnDef<TableRowData>[]>(() => {
    if (!tableData?.columns) return [];

    return [
      ...(tableData.columns.map(
        (col): ColumnDef<TableRowData> => ({
          accessorKey: col.name,
          header: col.name,
          cell: (info) => {
            const value = info.getValue();
            if (value === null || value === undefined) {
              // Style NULL values
              return <span className="text-muted-foreground">NULL</span>;
            }
            if (value === "") {
              // Style empty strings differently
              return <span className="text-muted-foreground ">""</span>;
            }
            // Render other values as strings
            return String(value);
          },
          filterFn: filterFn(mapDbColumnTypeToFilterType(col.type)),
          meta: {
            displayName: col.name,
            type: mapDbColumnTypeToFilterType(col.type),
            icon: Columns3Icon,
          },
        }),
      ) || []),
    ];
  }, [tableData?.columns]);

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

  const tableViewState = useMemo(() => {
    if (isFetchingTableData || isLoadingDatabases) return "loading";

    if (
      currentTable?.table &&
      tableData?.rows?.length &&
      tableData?.columns.length
    ) {
      return "data";
    }

    return "empty";
  }, [currentTable, tableData, isFetchingTableData, isLoadingDatabases]);

  // Safely update database tree for selected DB
  const handleSelectDatabase = useCallback(
    (dbName: string) => {
      fetchTables(dbName);
    },
    [fetchTables],
  );

  // --- Function to handle table selection from tree ---
  const handleSelectTable = useCallback(
    (dbName: string, tableName: string) => {
      if (currentTable?.db !== dbName || currentTable?.table !== tableName) {
        // First set the selection
        setCurrentTable({ db: dbName, table: tableName });

        // Reset filters and pagination
        const newFilters: ServerSideFilter[] = [];
        setServerFilters(newFilters);
        setColumnFilters([]);

        const newPageIndex = 0;
        setPagination((prev) => ({ ...prev, pageIndex: newPageIndex }));

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
    },
    [currentTable, fetchTableData],
  );

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
    onUpdateTitle("");
    onClose();
  };

  // --- TanStack Table Instance ---
  const table = useReactTable({
    data: tableData?.rows || [],
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
    defaultColumn: {
      minSize: 0,
      size: Number.MAX_SAFE_INTEGER,
      maxSize: Number.MAX_SAFE_INTEGER,
    },
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
        <div className="flex-grow overflow-auto relative">
          <TablePlaceholder animate={tableViewState === "loading"} />

          {tableViewState !== "loading" && (
            <Table className="z-10 bg-white">
              <TableHeader className="sticky top-0 bg-background z-10 shadow-sm">
                {table.getHeaderGroups().map((headerGroup) => (
                  <TableRow key={headerGroup.id}>
                    {headerGroup.headers.map((header) => (
                      <TableHead
                        key={header.id}
                        style={{
                          width:
                            header.getSize() === Number.MAX_SAFE_INTEGER
                              ? "auto"
                              : header.getSize(),
                        }}
                        className="px-4"
                      >
                        {flexRender(
                          header.column.columnDef.header,
                          header.getContext(),
                        )}
                      </TableHead>
                    ))}
                  </TableRow>
                ))}
              </TableHeader>
              <TableBody>
                {tableData?.rows?.length &&
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
                  ))}
              </TableBody>
            </Table>
          )}
        </div>

        <div className="flex items-center justify-between p-2 bg-background gap-2">
          <DataTableFilter table={table} onChange={handleFilterChange} />

          <DataTablePagination
            table={table}
            totalRowCount={totalRowCount}
            disabled={isFetchingTableData || !tableData}
          />

          <div className="flex gap-2">
            <SettingsModal>
              <Button title="Preferences" variant="ghost" size="icon">
                <SettingsIcon className="h-4 w-4" />
                <span className="sr-only">Preferences</span>
              </Button>
            </SettingsModal>

            <Button
              variant="ghost"
              size="icon"
              onClick={handleClose}
              title="Disconnect"
            >
              <UnplugIcon className="h-4 w-4" />
              <span className="sr-only">Close</span>
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
};

export default memo(MainDataView);
