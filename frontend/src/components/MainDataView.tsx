import { useMemoizedFn } from "ahooks";
import { useMemo, useEffect, memo } from "react";
import { useImmer } from "use-immer";
import { SettingsIcon, UnplugIcon } from "lucide-react";
import { toast } from "sonner";
import {
  ColumnDef,
  flexRender,
  getCoreRowModel,
  useReactTable,
  getPaginationRowModel,
  PaginationState,
  Updater,
} from "@tanstack/react-table";
import { useQuery, useMutation, keepPreviousData } from "@tanstack/react-query";
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
import { ColumnDataTypeIcons, mapDbColumnTypeToFilterType } from "@/lib/utils";
import { DatabaseTree, DatabaseTreeItem } from "@/components/DatabaseTree";
import { Button } from "@/components/ui/button";
import { ListTables, GetTableData, ListDatabases } from "wailsjs/go/main/App";
import TablePlaceholder from "./TablePlaceHolder";
import { SettingsModal } from "./SettingModal";

// Use `any` for row data initially, can be refined if needed
type TableRowData = Record<string, any>;

// Rename to avoid conflict with the imported component
type DatabaseTreeData = DatabaseTreeItem[];

const defaultPageSize = 50;

const MainDataView = ({
  onClose,
  onUpdateTitle,
}: {
  onClose: () => void;
  onUpdateTitle: (title: string) => void;
}) => {
  const [databaseTree, setDatabaseTree] = useImmer<DatabaseTreeData>([]);
  const [tableDataPrameters, setTableDataPrameters] = useImmer<{
    dbName: string;
    tableName: string;
    pageSize: number;
    pageIndex: number;
    serverFilters: ServerSideFilter[];
  }>({
    dbName: "",
    tableName: "",
    pageSize: defaultPageSize,
    pageIndex: 0,
    serverFilters: [],
  });
  const currentDb = tableDataPrameters.dbName;
  const currentTable = tableDataPrameters.tableName;
  const currentPageSize = tableDataPrameters.pageSize;
  const currentPageIndex = tableDataPrameters.pageIndex;
  const currentServerFilters = tableDataPrameters.serverFilters;

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
      toast.error("Error fetching tables", {
        description: `Error fetching tables for ${dbName}: ${error}`,
      });
    },
  });

  const { isPending: isFetchingTableData, data: tableData } = useQuery({
    enabled: !!currentDb && !!currentTable,
    queryKey: [
      "tableData",
      currentDb,
      currentTable,
      currentPageSize,
      currentPageIndex,
      currentServerFilters,
    ],
    queryFn: async () => {
      const dbName = currentDb;
      const tableName = currentTable;
      const filterObject =
        currentServerFilters.length > 0
          ? { filters: currentServerFilters }
          : null;

      try {
        onUpdateTitle(`Fetching data from ${dbName}.${tableName}...`);
        const res = await GetTableData(
          dbName,
          tableName,
          currentPageSize,
          currentPageIndex * currentPageSize,
          filterObject,
        );
        onUpdateTitle(`${dbName}.${tableName}`);

        return res;
      } catch (error) {
        onUpdateTitle(`Error fetching ${dbName}.${tableName}`);
        toast.error("Error fetching table data", {
          description: `Error fetching ${dbName}.${tableName}: ${error}`,
        });
        throw error;
      }
    },
    placeholderData: keepPreviousData,
  });

  // --- Handle server-side filter changes ---
  const handleFilterChange = useMemoizedFn((filters: ServerSideFilter[]) => {
    setTableDataPrameters((draft) => {
      draft.serverFilters = filters;
      draft.pageIndex = 0;
    });
  });

  // --- Derive columns and data from table data ---
  const columns = useMemo<ColumnDef<TableRowData>[]>(() => {
    if (!tableData?.columns) return [];

    return [
      ...(tableData.columns.map((col): ColumnDef<TableRowData> => {
        const type = mapDbColumnTypeToFilterType(col.type);

        return {
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
          filterFn: filterFn(type),
          meta: {
            displayName: col.name,
            type: type,
            icon: ColumnDataTypeIcons[type],
          },
        };
      }) || []),
    ];
  }, [tableData?.columns]);

  const totalRowCount = tableData?.totalRows;

  // --- Calculate pagination values ---
  const pagination = useMemo(
    () => ({
      pageIndex: currentPageIndex,
      pageSize: currentPageSize,
    }),
    [currentPageIndex, currentPageSize],
  );

  const pageCount = useMemo(() => {
    if (totalRowCount != null && totalRowCount >= 0) {
      return Math.ceil(totalRowCount / currentPageSize);
    }
    return -1;
  }, [totalRowCount, currentPageSize]);

  const tableViewState = (() => {
    if (isFetchingTableData || isLoadingDatabases) return "loading";

    if (
      currentDb &&
      currentTable &&
      tableData?.rows?.length &&
      tableData?.columns.length
    ) {
      return "data";
    }

    return "empty";
  })();

  const handleSelectDatabase = useMemoizedFn((dbName: string) => {
    fetchTables(dbName);
  });

  const handleSelectTable = useMemoizedFn(
    (dbName: string, tableName: string) => {
      setTableDataPrameters((draft) => {
        if (draft.dbName !== dbName || draft.tableName !== tableName) {
          draft.dbName = dbName;
          draft.tableName = tableName;
          draft.serverFilters = [];
          draft.pageIndex = 0;
        }
      });
    },
  );

  const handlePaginationChange = useMemoizedFn(
    (updaterOrValue: Updater<PaginationState>) => {
      const newPagination =
        typeof updaterOrValue === "function"
          ? updaterOrValue(pagination)
          : updaterOrValue;

      setTableDataPrameters((draft) => {
        draft.pageIndex = newPagination.pageIndex;
        draft.pageSize = newPagination.pageSize;
      });
    },
  );

  const handleClose = useMemoizedFn(() => {
    onUpdateTitle("");
    onClose();
  });

  // --- TanStack Table Instance ---
  const table = useReactTable({
    data: tableData?.rows || [],
    columns,
    state: {
      pagination,
    },
    manualPagination: true,
    manualFiltering: true,
    pageCount,
    onPaginationChange: handlePaginationChange,
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
        selectedTable={{ db: currentDb, table: currentTable }}
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

        <div className="flex items-center justify-between px-2 py-2 bg-background gap-2">
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
