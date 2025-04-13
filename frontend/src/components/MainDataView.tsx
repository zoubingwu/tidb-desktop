import React, { useState, useEffect, useMemo } from "react";
import { toast } from "sonner";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"; // From your table.tsx
import { DataTableFilter } from "@/components/ui/data-table-filter"; // From your data-table-filter.tsx
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Loader2, Database } from "lucide-react";
import { ListTables, GetTableData } from "wailsjs/go/main/App"; // Wails bindings

// Type for the Go backend response
type TableColumn = { name: string; type: string };
// Use `any` for row data initially, can be refined if needed
type TableRowData = Record<string, any>;

// Define expected props (e.g., disconnect handler)
interface MainDataViewProps {
  onDisconnect: () => void;
}

const MainDataView: React.FC<MainDataViewProps> = ({ onDisconnect }) => {
  const [tables, setTables] = useState<string[]>([]);
  const [selectedTable, setSelectedTable] = useState<string>("");
  const [columns, setColumns] = useState<ColumnDef<TableRowData>[]>([]);
  const [data, setData] = useState<TableRowData[]>([]);
  const [isLoadingTables, setIsLoadingTables] = useState(true);
  const [isLoadingData, setIsLoadingData] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // --- Pagination State ---
  const [{ pageIndex, pageSize }, setPagination] = useState<PaginationState>({
    pageIndex: 0, // Initial page index
    pageSize: 50, // Default page size
  });
  const pagination = useMemo(
    () => ({ pageIndex, pageSize }),
    [pageIndex, pageSize],
  );
  // We don't know total rows yet, so manual pagination
  const [pageCount, setPageCount] = useState(-1); // -1 means unknown

  // --- Filtering State ---
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([]);
  // --- Row Selection State ---
  const [rowSelection, setRowSelection] = useState({});

  // --- Fetch Table List ---
  useEffect(() => {
    const loadTables = async () => {
      setIsLoadingTables(true);
      setError(null);
      try {
        const tableList = await ListTables();
        setTables(tableList || []);
        if (tableList && tableList.length > 0) {
          setSelectedTable(tableList[0]); // Select first table by default
        } else {
          setData([]); // No tables, clear data
          setColumns([]);
        }
      } catch (err: any) {
        console.error("Error listing tables:", err);
        setError(err?.message || "Failed to load tables.");
        toast.error("Error listing tables", { description: err?.message });
      } finally {
        setIsLoadingTables(false);
      }
    };
    loadTables();
  }, []); // Run once on mount

  // --- Fetch Table Data ---
  useEffect(() => {
    if (!selectedTable) return; // Don't fetch if no table is selected

    const loadData = async () => {
      setIsLoadingData(true);
      setError(null);
      setRowSelection({}); // Clear selection on data change
      try {
        const offset = pageIndex * pageSize;
        const response = await GetTableData(selectedTable, pageSize, offset);

        if (!response) {
          throw new Error("No data received from backend.");
        }

        // Dynamically create columns for TanStack Table
        const dynamicColumns: ColumnDef<TableRowData>[] = [
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
          ...(response.columns?.map(
            (col: TableColumn): ColumnDef<TableRowData> => ({
              accessorKey: col.name,
              header: col.name, // Simple header for now
              cell: (info) => String(info.getValue() ?? ""), // Basic cell rendering
              // --- Add meta for filtering ---
              meta: {
                displayName: col.name,
                type: col.type.toLowerCase().includes("int")
                  ? "number"
                  : col.type.toLowerCase().includes("date") ||
                      col.type.toLowerCase().includes("time")
                    ? "date"
                    : "text", // Basic type inference
                icon: Database, // Assign the default icon here
              },
            }),
          ) || []),
        ];
        setColumns(dynamicColumns);
        setData(response.rows || []);

        // TODO: Use response.totalRows if available to set pageCount accurately
        // For now, assume there's a next page if we got a full page of results
        setPageCount(
          response.rows?.length === pageSize ? pageIndex + 1 : pageIndex,
        );
      } catch (err: any) {
        console.error(`Error fetching data for ${selectedTable}:`, err);
        setError(err?.message || `Failed to load data for ${selectedTable}.`);
        toast.error(`Error loading ${selectedTable}`, {
          description: err?.message,
        });
        setData([]);
        setColumns([]);
      } finally {
        setIsLoadingData(false);
      }
    };

    loadData();
  }, [selectedTable, pageIndex, pageSize]); // Refetch when table or pagination changes

  // --- TanStack Table Instance ---
  const table = useReactTable({
    data,
    columns,
    state: {
      columnFilters,
      pagination,
      rowSelection,
    },
    manualPagination: true, // Control pagination manually
    pageCount, // Set page count (-1 means unknown)
    onPaginationChange: setPagination, // Update state on change
    onColumnFiltersChange: setColumnFilters,
    onRowSelectionChange: setRowSelection,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    // debugTable: true, // Uncomment for debugging
    // debugHeaders: true,
    // debugColumns: true,
  });

  return (
    <div className="p-4 md:p-6 space-y-4 h-full flex flex-col">
      {/* Header could include table selector, query editor, etc. */}
      <header className="flex items-center justify-between">
        {/* TODO: Add a Select component to change selectedTable */}
        <h2 className="text-xl font-semibold">
          Table: {selectedTable || "None"}
        </h2>
        <Button onClick={onDisconnect} variant="outline">
          Disconnect
        </Button>
      </header>

      {/* Data Table Filter Component */}
      {columns.length > 0 && <DataTableFilter table={table} />}

      {/* Loading / Error / Table Display */}
      <div className="border rounded-md overflow-hidden flex-grow flex flex-col">
        {" "}
        {/* Ensure table area grows */}
        {isLoadingData && !data.length ? ( // Show loader only if data isn't already displayed
          <div className="flex-grow flex items-center justify-center text-muted-foreground">
            <Loader2 className="h-8 w-8 animate-spin mr-3" /> Loading data...
          </div>
        ) : error ? (
          <div className="flex-grow flex items-center justify-center text-destructive p-4">
            Error: {error}
          </div>
        ) : (
          <div className="flex-grow overflow-auto">
            {" "}
            {/* Scrollable table content */}
            <Table>
              <TableHeader className="sticky top-0 bg-background z-10">
                {" "}
                {/* Sticky header */}
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
                      No results.
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </div>
        )}
      </div>

      {/* Pagination and Row Selection Info */}
      <div className="flex items-center justify-between pt-2">
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
  );
};

export default MainDataView;
