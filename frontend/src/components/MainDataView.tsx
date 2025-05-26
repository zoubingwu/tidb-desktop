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
import { filterFn } from "@/lib/filters";
import {
  ColumnDataTypeIcons,
  isSystemDatabase,
  mapDbColumnTypeToFilterType,
} from "@/lib/utils";
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
import { useLocalStorageState, useMemoizedFn } from "ahooks";
import { tool } from "ai";
import { Allotment as ReactSplitView } from "allotment";
import "allotment/dist/style.css"; // for 3 column split view
import { Loader, SettingsIcon, SparkleIcon, UnplugIcon } from "lucide-react";
import { memo, useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import { useImmer } from "use-immer";
import {
  ExecuteSQL,
  GetDatabaseMetadata,
  GetTableData,
  ListDatabases,
  ListTables,
} from "wailsjs/go/main/App";
import { services } from "wailsjs/go/models";
import { EventsEmit, EventsOn } from "wailsjs/runtime";
import { z } from "zod";
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

const LAYOUT_DB_TREE_WIDTH_KEY = "layout:dbTreeWidth";
const LAYOUT_AI_PANEL_WIDTH_KEY = "layout:aiPanelWidth";
const LAYOUT_AI_PANEL_VISIBLE_KEY = "layout:aiPanelVisible";

// @TODO: make it configurable
const SHOW_SYSTEM_DATABASES = false;

const MainDataView = ({
  onClose,
  connectionDetails,
}: {
  onClose: () => void;
  connectionDetails: services.ConnectionDetails | null;
}) => {
  const [activityLog, setActivityLog] = useState<string[]>([]);
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

  const appendActivityLog = useMemoizedFn((log: string) => {
    setActivityLog((prev) => [...prev, log]);
  });

  const [sqlFromAI, setSqlFromAI] = useState<string>("");

  const [dbTreeWidth, setDbTreeWidth] = useLocalStorageState<number>(
    LAYOUT_DB_TREE_WIDTH_KEY,
    {
      defaultValue: DEFAULT_DB_TREE_WIDTH,
    },
  );

  const [aiPanelWidth, setAiPanelWidth] = useLocalStorageState<number>(
    LAYOUT_AI_PANEL_WIDTH_KEY,
    {
      defaultValue: DEFAULT_AI_PANEL_WIDTH,
    },
  );

  const [showAIPanel, setShowAIPanel] = useLocalStorageState<boolean>(
    LAYOUT_AI_PANEL_VISIBLE_KEY,
    {
      defaultValue: false,
    },
  );

  const mergeDatabaseTree = (
    tree: { dbName: string; tables?: string[]; isLoadingTables?: boolean }[],
  ) => {
    setDatabaseTree((draft: DatabaseTreeData) => {
      tree.forEach((db) => {
        const existing = draft.find((item) => item.name === db.dbName);
        if (existing) {
          if (db.tables) {
            existing.tables = db.tables;
          }
          existing.isLoadingTables = db.isLoadingTables ?? false;
        } else {
          draft.push({
            name: db.dbName,
            tables: db.tables || [],
            isLoadingTables: db.isLoadingTables ?? false,
          });
        }

        // system databases first, then alphabetically
        draft.sort((a, b) => {
          const isASystemDb = isSystemDatabase(a.name);
          const isBSystemDb = isSystemDatabase(b.name);
          if (isASystemDb && !isBSystemDb) {
            return -1; // a comes first
          }
          if (!isASystemDb && isBSystemDb) {
            return 1; // b comes first
          }
          // If both are system or both are not system, sort alphabetically by name
          return a.name.localeCompare(b.name);
        });
      });
    });
  };

  // --- List Databases Query (keeps automatic fetching for initial page load) ---
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
      console.log("databases fetched", databases);

      const filteredDatabases = databases.filter((i) =>
        SHOW_SYSTEM_DATABASES ? true : !isSystemDatabase(i),
      );

      mergeDatabaseTree(filteredDatabases.map((dbName) => ({ dbName })));

      // Check if databases exist in metadata, trigger indexer if not
      const checkMetadataAndTriggerIndexer = async () => {
        try {
          const metadata = await GetDatabaseMetadata();
          const metadataDbs = Object.keys(metadata?.databases || {});

          const missingDbs = filteredDatabases.filter(
            (dbName) => !metadataDbs.includes(dbName),
          );

          if (missingDbs.length > 0) {
            console.log("Databases missing from metadata:", missingDbs);
            missingDbs.forEach((dbName) => triggerIndexer(true, dbName));
          }
        } catch (error) {
          console.log("Error checking metadata, triggering indexer:", error);
          triggerIndexer(true);
        }
      };

      checkMetadataAndTriggerIndexer();
    }
  }, [databases]);

  const indexStartedRef = useRef(false);
  const triggerIndexer = useMemoizedFn((force: boolean, dbName?: string) => {
    if (indexStartedRef.current && !force) {
      return;
    }
    indexStartedRef.current = true;

    appendActivityLog("Indexing database...");
    EventsEmit(
      "metadata:extraction:start",
      connectionDetails?.name!,
      force,
      dbName ?? "",
    );
  });

  useEffect(() => {
    const cleanup1 = EventsOn("metadata:extraction:failed", (error: string) => {
      appendActivityLog(`Indexing database failed: ${error}`);
    });

    const cleanup2 = EventsOn(
      "metadata:extraction:completed",
      (metadata: services.ConnectionMetadata) => {
        appendActivityLog("Indexing database completed.");
        mergeDatabaseTree(
          Object.keys(metadata.databases).map((dbName) => ({
            dbName,
            tables: metadata.databases[dbName].tables.map(
              (table) => table.name,
            ),
            isLoadingTables: false,
          })),
        );
      },
    );

    triggerIndexer(false);

    return () => {
      cleanup1();
      cleanup2();
    };
  }, []);

  const { mutate: fetchTables } = useMutation({
    mutationFn: (dbName: string) => ListTables(dbName),
    onMutate: (dbName: string) => {
      if (!databaseTree.find((db) => db.name === dbName)?.tables?.length) {
        mergeDatabaseTree([{ dbName, isLoadingTables: true }]);
      }
    },
    onSuccess: (tables, dbName) => {
      mergeDatabaseTree([{ dbName, tables, isLoadingTables: false }]);
    },
    onError: (error, dbName) => {
      mergeDatabaseTree([{ dbName, isLoadingTables: false }]);
      toast.error("Error fetching tables", {
        description: error.message,
      });
    },
  });

  // fetch from a specific table
  const { data: tableData, isFetching: isFetchingTableData } = useQuery({
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

      const titleTarget = tableName
        ? `${dbName}.${tableName}`
        : "SQL Query Result";

      try {
        appendActivityLog(`Fetching data from ${titleTarget}...`);
        const res = await GetTableData(
          dbName,
          tableName,
          currentPageSize,
          currentPageIndex * currentPageSize,
          filterObject,
        );
        console.log("tableData", res);
        appendActivityLog(`Fetched data from ${dbName}.${tableName}`);

        return res;
      } catch (error: any) {
        appendActivityLog(
          `Error fetching ${titleTarget}: ${error?.message || String(error)}`,
        );
        toast.error("Error fetching table data", {
          description: error,
        });
        throw error;
      }
    },
    placeholderData: keepPreviousData,
  });

  const {
    mutateAsync: executeSql,
    data: sqlFromAIResult,
    isPending: isExecutingSQLFromAI,
    reset: resetSqlFromAIResult,
  } = useMutation<services.SQLResult, Error, string>({
    mutationFn: async (sqlToExecute: string) => {
      try {
        appendActivityLog("Executing SQL...");
        const res = await ExecuteSQL(sqlToExecute);

        if (res.rowsAffected && res.rowsAffected > 0) {
          appendActivityLog(
            `SQL executed successfully, ${res.rowsAffected} row${res.rowsAffected === 1 ? "" : "s"} affected`,
          );
        } else {
          appendActivityLog("SQL executed successfully");
        }
        return res;
      } catch (error: any) {
        const errorMessage = error?.message || String(error);
        appendActivityLog(`SQL execution error: ${errorMessage}`);
        toast.error("Error executing SQL", { description: errorMessage });
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
    if (isLoadingDatabases) {
      return "init";
    }

    if (isFetchingTableData || isExecutingSQLFromAI) {
      return "loading";
    }

    if (
      (currentDb && currentTable && tableData?.columns?.length) ||
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
      resetSqlFromAIResult();
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
    onClose();
  });

  const handleApplyAIGeneratedQuery = async (query: string, dbName: string) => {
    const sqlQuery = query;
    const upperSqlQuery = sqlQuery.toUpperCase();
    const tableModifyingKeywords = [
      "CREATE TABLE",
      "ALTER TABLE",
      "DROP TABLE",
      "RENAME TABLE",
    ];
    const isModifyingQuery = tableModifyingKeywords.some((keyword) =>
      upperSqlQuery.includes(keyword),
    );

    resetTableDataPrameters();
    const result = await executeSql(sqlQuery);
    setSqlFromAI(sqlQuery);

    if (isModifyingQuery) {
      fetchTables(dbName);
      triggerIndexer(true, dbName);
    }

    return result;
  };

  const tools = {
    executeSql: tool({
      description:
        "Executes SQL queries. For read-only queries (SELECT), executes immediately. For write operations (INSERT, UPDATE, DELETE, etc.), requires user confirmation through the UI.",
      parameters: z.object({
        query: z
          .string()
          .describe(
            "The SQL query string to execute (e.g., `SELECT * FROM users LIMIT 3`, `INSERT INTO users (name) VALUES ('John')`).",
          ),
        dbName: z
          .string()
          .describe("The name of the database the query was executed on."),
        requiresConfirmation: z
          .boolean()
          .optional()
          .describe(
            "Set to true for non-read-only operations that require user confirmation. Should be true for INSERT, UPDATE, DELETE, ALTER, DROP, etc.",
          ),
      }),
      execute: ({
        query,
        requiresConfirmation = false,
        dbName,
      }): Promise<{
        success: boolean;
        result?: services.SQLResult;
        error?: string;
      }> => {
        console.log(
          `Tool Call: executeSql (Query: ${query}, dbName: ${dbName})`,
        );

        return new Promise((resolve) => {
          const trimmedQuery = query.trim().toUpperCase();
          const isReadOnly =
            trimmedQuery.startsWith("SELECT") ||
            trimmedQuery.startsWith("SHOW") ||
            trimmedQuery.startsWith("DESCRIBE") ||
            trimmedQuery.startsWith("EXPLAIN");

          // Auto-detect if confirmation is needed for non-read-only queries
          const needsConfirmation = requiresConfirmation || !isReadOnly;

          const run = () => {
            if (needsConfirmation) {
              toast.dismiss();
            }
            return handleApplyAIGeneratedQuery(query, dbName)
              .then((res) => {
                console.log("Tool Result: executeSql ->", res);
                return resolve({
                  success: true,
                  result: res,
                });
              })
              .catch((err) => {
                console.log("Tool Error: executeSql ->", err);
                return resolve({
                  success: false,
                  error: err,
                });
              });
          };

          const cancel = () => {
            if (needsConfirmation) {
              toast.dismiss();
            }
            return resolve({
              success: false,
              error: "User denied execution",
            });
          };

          if (needsConfirmation) {
            console.log(
              `Tool Call: executeSql (Query requires confirmation: ${query})`,
            );

            toast("Please confirm to run query", {
              action: (
                <Button
                  variant="default"
                  size="sm"
                  className="text-xs"
                  onClick={run}
                >
                  Confirm
                </Button>
              ),
              cancel: (
                <Button
                  variant="outline"
                  size="sm"
                  className="text-xs"
                  onClick={cancel}
                >
                  Cancel
                </Button>
              ),
              duration: Infinity,
            });
          } else {
            return run();
          }
        });
      },
    }),
  };

  const table: ReactTable<TableRowData> = useReactTable({
    data:
      (sqlFromAI && sqlFromAIResult ? sqlFromAIResult.rows : tableData?.rows) ||
      [],
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
    <div className="flex flex-col h-full">
      <ReactSplitView
        key="outer-split"
        defaultSizes={[dbTreeWidth!, window.innerWidth - dbTreeWidth!]}
        separator={false}
        onChange={(sizes: number[]) => {
          if (sizes.length > 0 && sizes[0] > 50) {
            // Ensure a minimum sensible width
            setDbTreeWidth(sizes[0]);
          }
        }}
      >
        <ReactSplitView.Pane
          minSize={DEFAULT_DB_TREE_WIDTH / 2}
          maxSize={DEFAULT_DB_TREE_WIDTH * 2}
        >
          <DatabaseTree
            databaseTree={databaseTree}
            isLoadingDatabases={isLoadingDatabases && databaseTree.length === 0}
            databasesError={databasesError}
            onSelectDatabase={handleSelectDatabase}
            onSelectTable={handleSelectTable}
            selectedTable={{ db: currentDb, table: currentTable }}
          />
        </ReactSplitView.Pane>

        <ReactSplitView.Pane className="flex flex-col overflow-hidden">
          <ReactSplitView
            key={`inner-split`}
            defaultSizes={[
              window.innerWidth - dbTreeWidth! - aiPanelWidth!,
              aiPanelWidth ?? DEFAULT_AI_PANEL_WIDTH,
            ]}
            separator={false}
            onChange={(sizes: number[]) => {
              // sizes[0] is table width, sizes[1] is AI panel width (if visible)
              if (showAIPanel && sizes.length === 2 && sizes[1] > 50) {
                // Ensure a minimum
                setAiPanelWidth(sizes[1]);
              }
            }}
          >
            <ReactSplitView.Pane minSize={200}>
              {tableViewState === "data" ? (
                <DataTable<TableRowData> table={table} height={TABLE_HEIGHT} />
              ) : (
                <TablePlaceholder animate={tableViewState === "loading"} />
              )}
            </ReactSplitView.Pane>

            <ReactSplitView.Pane
              visible={showAIPanel}
              minSize={DEFAULT_AI_PANEL_WIDTH / 2}
              preferredSize={aiPanelWidth ?? DEFAULT_AI_PANEL_WIDTH}
              maxSize={DEFAULT_AI_PANEL_WIDTH * 2}
            >
              <AIPanel
                onApplyQueryFromAI={handleApplyAIGeneratedQuery}
                opened={showAIPanel}
                isExecutingSQLFromAI={isExecutingSQLFromAI}
                tools={tools}
              />
            </ReactSplitView.Pane>
          </ReactSplitView>
        </ReactSplitView.Pane>
      </ReactSplitView>

      <TooltipProvider delayDuration={0}>
        <div className="flex items-center justify-between px-2 py-0 bg-[var(--card)] gap-2 border-t border-[var(--muted)]/10">
          <div className="flex text-xs gap-1 items-center flex-1 min-w-0">
            {tableViewState === "loading" && (
              <Loader className="size-3 animate-spin mr-1" />
            )}
            <Tooltip>
              <TooltipTrigger asChild>
                <span
                  className="relative top-[1px] truncate block"
                  style={{ maxWidth: "100%" }}
                >
                  {activityLog.length > 0
                    ? activityLog[activityLog.length - 1]
                    : "Ready"}
                </span>
              </TooltipTrigger>
              <TooltipContent>
                <div className="max-h-60 w-auto max-w-lg overflow-y-auto rounded-md text-xs force-select-text">
                  {activityLog.length > 0 ? (
                    activityLog.map((log, index) => (
                      <p key={index} className="whitespace-pre-wrap">
                        {log}
                      </p>
                    ))
                  ) : (
                    <p>No activity yet.</p>
                  )}
                </div>
              </TooltipContent>
            </Tooltip>
          </div>

          <div className="flex flex-nowrap items-center gap-2">
            {!sqlFromAI && (
              <DataTablePagination
                table={table}
                totalRowCount={totalRowCount}
                disabled={tableViewState !== "data"}
                loading={tableViewState === "loading"}
              />
            )}

            <div className="flex gap-2">
              {!sqlFromAI && (
                <Tooltip>
                  <DataTableFilter
                    table={table}
                    onChange={handleFilterChange}
                    disabled={tableViewState !== "data"}
                  />
                  <TooltipContent>
                    <p>Filter</p>
                  </TooltipContent>
                </Tooltip>
              )}

              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => setShowAIPanel(!showAIPanel)}
                  >
                    <SparkleIcon className="size-3.5" />
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
                    <SettingsIcon className="size-3.5" />
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
                    <UnplugIcon className="size-3.5" />
                    <span className="sr-only">Disconnect</span>
                  </Button>
                </TooltipTrigger>
                <TooltipContent>
                  <p>Disconnect</p>
                </TooltipContent>
              </Tooltip>
            </div>
          </div>
        </div>
      </TooltipProvider>
    </div>
  );
};

export default memo(MainDataView);
