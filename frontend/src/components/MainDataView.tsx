import "allotment/dist/style.css"; // for 3 column split view
import { AIPanel } from "@/components/AIPanel";
import { DataTablePagination } from "@/components/DataTablePagination";
import { DatabaseTree, DatabaseTreeItem } from "@/components/DatabaseTree";
import { Button } from "@/components/ui/button";
import {
  DataTableFilter,
  ServerSideFilter,
} from "@/components/ui/data-table-filter";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { SqlAgentResponse } from "@/lib/ai";
import { filterFn } from "@/lib/filters";
import { ColumnDataTypeIcons, mapDbColumnTypeToFilterType } from "@/lib/utils";
import { keepPreviousData, useMutation, useQuery } from "@tanstack/react-query";
import {
  CellContext,
  ColumnDef,
  PaginationState,
  Table as ReactTable,
  Updater,
  getCoreRowModel,
  getPaginationRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { useMemoizedFn } from "ahooks";
import { Allotment as ReactSplitView } from "allotment";
import { SettingsIcon, SparkleIcon, UnplugIcon } from "lucide-react";
import { memo, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { useImmer } from "use-immer";
import {
  ExecuteSQL,
  GetTableData,
  ListDatabases,
  ListTables,
} from "wailsjs/go/main/App";
import DataTable from "./DataTable";
import SettingsModal from "./SettingModal";
import TablePlaceholder from "./TablePlaceHolder";

// Use `any` for row data initially, can be refined if needed
type TableRowData = Record<string, any>;

// Rename to avoid conflict with the imported component
type DatabaseTreeData = DatabaseTreeItem[];

const defaultPageSize = 50;
const defaultTableDataParameters = {
  dbName: "",
  tableName: "",
  pageSize: defaultPageSize,
  pageIndex: 0,
  serverFilters: [],
};
const TITLE_BAR_HEIGHT = 28;
const FOOTER_HEIGHT = 40;
const DEFAULT_DB_TREE_WIDTH = 240;
const DEFAULT_AI_PANEL_WIDTH = 300;
const TABLE_HEIGHT = window.innerHeight - TITLE_BAR_HEIGHT - FOOTER_HEIGHT;

const MainDataView = ({
  onClose,
  onUpdateTitle,
}: {
  onClose: () => void;
  onUpdateTitle: (title: string, loading?: boolean) => void;
}) => {
  const [databaseTree, setDatabaseTree] = useImmer<DatabaseTreeData>([]);
  const [tableDataPrameters, setTableDataPrameters] = useImmer<{
    dbName: string;
    tableName: string;
    pageSize: number;
    pageIndex: number;
    serverFilters: ServerSideFilter[];
  }>(defaultTableDataParameters);
  const currentDb = tableDataPrameters.dbName;
  const currentTable = tableDataPrameters.tableName;
  const currentPageSize = tableDataPrameters.pageSize;
  const currentPageIndex = tableDataPrameters.pageIndex;
  const currentServerFilters = tableDataPrameters.serverFilters;

  const resetTableDataPrameters = useMemoizedFn(() => {
    setTableDataPrameters(defaultTableDataParameters);
  });

  const [sqlFromAI, setSqlFromAI] = useState<string>("");
  const [showAIPanel, setShowAIPanel] = useState(false);

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
        description: error.message,
      });
    },
  });

  const { data: tableData, isFetching: isFetchingTableData } = useQuery({
    enabled: !!currentDb && !!currentTable,
    queryKey: [
      "tableData",
      currentDb,
      currentTable,
      currentPageSize,
      currentPageIndex,
    ],
    queryFn: async () => {
      const dbName = currentDb;
      const tableName = currentTable;
      const filterObject =
        currentServerFilters.length > 0
          ? { filters: currentServerFilters }
          : null;

      const titleTarget = tableName
        ? `${dbName}.${tableName}`
        : "SQL Query Result";

      try {
        onUpdateTitle(`Fetching data from ${titleTarget}...`, true);
        const res = await GetTableData(
          dbName,
          tableName,
          currentPageSize,
          currentPageIndex * currentPageSize,
          filterObject,
        );
        onUpdateTitle(`${dbName}.${tableName}`);

        return res;
      } catch (error: any) {
        onUpdateTitle(`Error fetching ${titleTarget}`);
        toast.error("Error fetching table data", {
          description: error.message,
        });
        throw error;
      }
    },
    placeholderData: keepPreviousData,
  });

  const { data: sqlFromAIResult, isFetching: isExecutingSQLFromAI } = useQuery({
    enabled: !!sqlFromAI,
    queryKey: ["sqlFromAI", sqlFromAI],
    queryFn: async () => {
      try {
        onUpdateTitle("Executing SQL from AI...", true);
        const res = await ExecuteSQL(sqlFromAI);
        onUpdateTitle("SQL from AI executed");
        return res;
      } catch (error: any) {
        toast.error("Error fetching SQL from AI", {
          description: error.message,
        });
        throw error;
      }
    },
  });

  const handleFilterChange = useMemoizedFn((filters: ServerSideFilter[]) => {
    setTableDataPrameters((draft) => {
      draft.serverFilters = filters;
      draft.pageIndex = 0;
    });
  });

  const columns = useMemo<ColumnDef<TableRowData>[]>(() => {
    const renderCell = (info: CellContext<TableRowData, unknown>) => {
      const value = info.getValue();
      if (value === null || value === undefined) {
        // Style NULL values
        return <span className="text-muted-foreground italic">NULL</span>;
      }
      if (value === "") {
        // Style empty strings differently
        return <span className="text-muted-foreground italic">EMPTY</span>;
      }
      // Render other values as strings
      return String(value);
    };

    if (sqlFromAI && sqlFromAIResult?.columns?.length) {
      return sqlFromAIResult.columns.map((colName) => {
        return {
          accessorKey: colName,
          header: colName,
          cell: renderCell,
        };
      });
    }

    if (tableData?.columns) {
      return [
        ...(tableData.columns.map((col): ColumnDef<TableRowData> => {
          const type = mapDbColumnTypeToFilterType(col.type);

          return {
            accessorKey: col.name,
            header: col.name,
            cell: renderCell,
            filterFn: filterFn(type),
            meta: {
              displayName: col.name,
              type: type,
              icon: ColumnDataTypeIcons[type],
            },
          };
        }) || []),
      ];
    }

    return [];
  }, [tableData?.columns, sqlFromAIResult, sqlFromAI]);

  console.log("tableData", tableData);
  console.log("sqlFromAIResult", sqlFromAIResult);
  console.log("columns", columns);

  const totalRowCount = sqlFromAIResult ? null : tableData?.totalRows;

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
    if (isFetchingTableData || isLoadingDatabases || isExecutingSQLFromAI) {
      return "loading";
    }

    if (
      (currentDb &&
        currentTable &&
        tableData?.rows?.length &&
        tableData?.columns?.length) ||
      sqlFromAIResult
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
      setSqlFromAI("");
      setTableDataPrameters((draft) => {
        draft.dbName = dbName;
        draft.tableName = tableName;
        draft.serverFilters = [];
        draft.pageIndex = 0;
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

  const handleApplyAIGeneratedQuery = (result: SqlAgentResponse) => {
    if (result.success) {
      resetTableDataPrameters();
      setSqlFromAI(result.query);
    }
  };

  const table: ReactTable<TableRowData> = useReactTable({
    data: (sqlFromAIResult ? sqlFromAIResult.rows : tableData?.rows) || [],
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
    <ReactSplitView
      defaultSizes={[
        DEFAULT_DB_TREE_WIDTH,
        window.innerWidth - DEFAULT_DB_TREE_WIDTH,
      ]}
      separator={false}
    >
      <ReactSplitView.Pane>
        <DatabaseTree
          databaseTree={databaseTree}
          isLoadingDatabases={isLoadingDatabases}
          databasesError={databasesError}
          onSelectDatabase={handleSelectDatabase}
          onSelectTable={handleSelectTable}
          selectedTable={{ db: currentDb, table: currentTable }}
        />
      </ReactSplitView.Pane>

      <ReactSplitView.Pane className="flex flex-col overflow-hidden">
        <ReactSplitView
          defaultSizes={[
            window.innerWidth - DEFAULT_DB_TREE_WIDTH - DEFAULT_AI_PANEL_WIDTH,
            DEFAULT_AI_PANEL_WIDTH,
          ]}
          separator={false}
        >
          <ReactSplitView.Pane>
            {tableViewState === "data" ? (
              <DataTable<TableRowData> table={table} height={TABLE_HEIGHT} />
            ) : (
              <TablePlaceholder animate={tableViewState === "loading"} />
            )}
          </ReactSplitView.Pane>

          <ReactSplitView.Pane visible={showAIPanel}>
            <AIPanel
              currentDb={currentDb}
              currentTable={currentTable}
              onApplyQueryFromAI={handleApplyAIGeneratedQuery}
              opened={showAIPanel}
            />
          </ReactSplitView.Pane>
        </ReactSplitView>

        <div className="flex items-center justify-between px-2 py-2 bg-background gap-2">
          <div className="flex gap-2"></div>

          <TooltipProvider delayDuration={0}>
            <div className="flex flex-nowrap items-center gap-2">
              <DataTablePagination
                table={table}
                totalRowCount={totalRowCount}
                disabled={tableViewState !== "data" || !!sqlFromAI}
              />

              <div className="flex gap-2">
                <Tooltip>
                  <DataTableFilter
                    table={table}
                    onChange={handleFilterChange}
                    disabled={tableViewState !== "data" || !!sqlFromAIResult}
                  />
                  <TooltipContent>
                    <p>Filter</p>
                  </TooltipContent>
                </Tooltip>

                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => setShowAIPanel(!showAIPanel)}
                    >
                      <SparkleIcon className="h-4 w-4" />
                      <span className="sr-only">Ask AI</span>
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>
                    <p>Ask AI</p>
                  </TooltipContent>
                </Tooltip>

                <Tooltip>
                  <SettingsModal>
                    <Button title="Preferences" variant="ghost" size="icon">
                      <SettingsIcon className="h-4 w-4" />
                      <span className="sr-only">Preferences</span>
                    </Button>
                  </SettingsModal>
                  <TooltipContent>
                    <p>Preferences</p>
                  </TooltipContent>
                </Tooltip>

                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={handleClose}
                      title="Disconnect"
                    >
                      <UnplugIcon className="h-4 w-4" />
                      <span className="sr-only">Disconnect</span>
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>
                    <p>Disconnect</p>
                  </TooltipContent>
                </Tooltip>
              </div>
            </div>
          </TooltipProvider>
        </div>
      </ReactSplitView.Pane>
    </ReactSplitView>
  );
};

export default memo(MainDataView);
