import { useState, useEffect, useMemo } from "react";
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
import { DataTableFilter } from "@/components/ui/data-table-filter";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Loader2, Database } from "lucide-react";
import { ListTables, GetTableData, ListDatabases } from "wailsjs/go/main/App";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

// Type for the Go backend response from GetTableData
// Assuming TableDataResponse structure defined in Go
type TableDataResponse = {
  columns: { name: string; type: string }[];
  rows: Record<string, any>[];
  totalRows?: number;
};

// Use `any` for row data initially, can be refined if needed
type TableRowData = Record<string, any>;

// Define expected props (e.g., disconnect handler)
interface MainDataViewProps {
  onDisconnect: () => void;
}

const MainDataView = ({ onDisconnect }: MainDataViewProps) => {
  // State managed by user interaction / table instance
  const [selectedDatabase, setSelectedDatabase] = useState<string>("");
  const [selectedTable, setSelectedTable] = useState<string>("");
  const [{ pageIndex, pageSize }, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: 50,
  });
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([]);
  const [rowSelection, setRowSelection] = useState({});

  // --- TanStack Query for fetching tables ---
  const {
    data: tables = [],
    isLoading: isLoadingTables,
    error: tablesError,
  } = useQuery<string[], Error>({
    queryKey: ["tables", selectedDatabase],
    queryFn: () => ListTables(selectedDatabase),
    enabled: !!selectedDatabase,
    staleTime: 5 * 60 * 1000, // Cache for 5 minutes
    refetchOnWindowFocus: false,
  });

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

  // --- Select first table when tables load ---
  useEffect(() => {
    if (!selectedTable && tables.length > 0) {
      setSelectedTable(tables[0]);
    }
  }, [tables, selectedTable]);

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
      selectedDatabase,
      selectedTable,
      pageIndex,
      pageSize,
    ],
    queryFn: async () => {
      if (!selectedTable || !selectedDatabase) return null;
      return await GetTableData(selectedTable, pageSize, pageIndex * pageSize);
    },
    enabled: !!selectedDatabase && !!selectedTable,
    placeholderData: (previousData) => previousData ?? undefined,
    staleTime: 1 * 60 * 1000, // Cache data for 1 minute
    refetchOnWindowFocus: false,
  });

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
          cell: (info) => String(info.getValue() ?? ""),
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
  const pageCount = useMemo(() => {
    if (tableDataResponse?.totalRows != null) {
      return Math.ceil(tableDataResponse.totalRows / pageSize);
    }
    // If totalRows isn't provided, estimate based on current data
    // This allows the "Next" button to be enabled if a full page was fetched.
    return data.length < pageSize ? pageIndex + 1 : pageIndex + 2; // Indicate potentially more pages
  }, [tableDataResponse?.totalRows, pageSize, data.length, pageIndex]);

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
    pageCount, // Use calculated page count
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
    (selectedDatabase && isLoadingTables) ||
    (selectedTable && isLoadingData && !table.getRowModel().rows.length);

  // --- Effects to handle selection changes ---

  // Auto-select first database when list loads
  useEffect(() => {
    if (!selectedDatabase && databases.length > 0) {
      setSelectedDatabase(databases[0]);
    }
    // If the current selected DB disappears from the list, reset
    if (
      selectedDatabase &&
      databases.length > 0 &&
      !databases.includes(selectedDatabase)
    ) {
      setSelectedDatabase(databases[0] || "");
      setSelectedTable(""); // Reset table too
    }
  }, [databases, selectedDatabase]);

  // Auto-select first table when list loads or DB changes
  useEffect(() => {
    if (selectedDatabase && tables.length > 0) {
      // If current table is not in the new list, select the first one
      if (!selectedTable || !tables.includes(selectedTable)) {
        setSelectedTable(tables[0]);
      }
    } else if (selectedDatabase && !isLoadingTables && tables.length === 0) {
      setSelectedTable(""); // Reset table if DB has no tables
    }
    // Don't reset if selectedDatabase is cleared, wait for DB selection effect
  }, [tables, selectedDatabase, selectedTable, isLoadingTables]);

  return (
    <div className="h-full flex flex-col">
      <header className="p-4 flex items-center justify-between border-b sticky top-0 bg-background z-20">
        <div className="flex items-center gap-4">
          <Select
            value={selectedDatabase}
            onValueChange={(value) => {
              setSelectedDatabase(value);
              setSelectedTable("");
              setPagination({ pageIndex: 0, pageSize });
            }}
            disabled={isLoadingDatabases}
          >
            <SelectTrigger className="w-[180px]">
              <SelectValue
                placeholder={
                  isLoadingDatabases ? "Loading DBs..." : "Select Database"
                }
              />
            </SelectTrigger>
            <SelectContent>
              {databases.map((db) => (
                <SelectItem key={db} value={db}>
                  {db}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Select
            value={selectedTable}
            onValueChange={setSelectedTable}
            disabled={
              !selectedDatabase || isLoadingTables || tables.length === 0
            }
          >
            <SelectTrigger className="w-[180px]">
              <SelectValue
                placeholder={
                  isLoadingTables ? "Loading Tables..." : "Select Table"
                }
              />
            </SelectTrigger>
            <SelectContent>
              {tables.map((tbl) => (
                <SelectItem key={tbl} value={tbl}>
                  {tbl}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          {columns.length > 1 && <DataTableFilter table={table} />}
        </div>

        <Button onClick={onDisconnect} variant="outline">
          Disconnect
        </Button>
      </header>

      <div className="flex-grow flex flex-col overflow-hidden p-4">
        <div className="border rounded-md overflow-hidden flex-grow flex flex-col">
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
          ) : !selectedDatabase ? (
            <div className="flex-grow flex items-center justify-center text-muted-foreground p-4">
              Please select a database.
            </div>
          ) : !selectedTable && tables.length > 0 ? (
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
                        className="h-24 text-center"
                      >
                        {isLoadingData
                          ? "Loading data..."
                          : selectedTable
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

        <div className="flex items-center justify-between pt-4">
          <div className="flex-1 text-sm text-muted-foreground">
            {table.getFilteredSelectedRowModel().rows.length} of{" "}
            {table.getFilteredRowModel().rows.length} row(s) selected.
          </div>
          <div className="flex items-center space-x-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => table.previousPage()}
              disabled={!table.getCanPreviousPage()}
            >
              Previous
            </Button>
            <span className="text-sm">
              Page {table.getState().pagination.pageIndex + 1}
              {table.getPageCount() > 0 ? ` of ${table.getPageCount()}` : ""}
            </span>
            <Button
              variant="outline"
              size="sm"
              onClick={() => table.nextPage()}
              disabled={!table.getCanNextPage()}
            >
              Next
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
};

export default MainDataView;
