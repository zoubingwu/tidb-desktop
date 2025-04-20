import { Table } from "@tanstack/react-table";
import { Button } from "@/components/ui/button";
import {
  ChevronsLeft,
  ChevronLeft,
  ChevronRight,
  ChevronsRight,
} from "lucide-react";

// Define props, primarily the TanStack Table instance
interface DataTablePaginationProps<TData> {
  table: Table<TData>;
  totalRowCount?: number | null; // Pass total rows if available
  disabled?: boolean;
}

export function DataTablePagination<TData>({
  table,
  totalRowCount,
  disabled,
}: DataTablePaginationProps<TData>) {
  const { pageIndex, pageSize } = table.getState().pagination;

  // Calculate display range
  const firstRowIndex = pageIndex * pageSize + 1;
  const lastRowIndex = Math.min(
    firstRowIndex + pageSize - 1,
    totalRowCount ?? Number.MAX_SAFE_INTEGER, // Use totalRowCount if provided
  );

  // Determine total pages accurately if totalRowCount is known
  const calculatedPageCount = totalRowCount
    ? Math.ceil(totalRowCount / pageSize)
    : table.getPageCount(); // Fallback to table's potentially estimated page count

  return (
    <div className="flex items-center space-x-2">
      {/* Row range display */}
      <div className="flex items-center justify-center text-xs font-medium whitespace-nowrap">
        {totalRowCount != null && totalRowCount > 0 // Check if totalRowCount is known and > 0
          ? `${firstRowIndex} - ${lastRowIndex} of ${totalRowCount}`
          : `${table.getRowCount()} rows`}
      </div>

      {/* Navigation buttons */}
      <div className="flex items-center space-x-2">
        <Button
          variant="ghost"
          className="hidden lg:flex"
          onClick={() => table.setPageIndex(0)}
          disabled={!table.getCanPreviousPage() || disabled}
          title="Go to first page"
          size="icon"
        >
          <span className="sr-only">Go to first page</span>
          <ChevronsLeft className="h-4 w-4" />
        </Button>
        <Button
          variant="ghost"
          onClick={() => table.previousPage()}
          disabled={!table.getCanPreviousPage() || disabled}
          title="Go to previous page"
          size="icon"
        >
          <span className="sr-only">Go to previous page</span>
          <ChevronLeft className="h-4 w-4" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          onClick={() => table.nextPage()}
          disabled={!table.getCanNextPage() || disabled}
          title="Go to next page"
        >
          <span className="sr-only">Go to next page</span>
          <ChevronRight className="h-4 w-4" />
        </Button>
        <Button
          variant="ghost"
          className="hidden lg:flex"
          onClick={() =>
            table.setPageIndex(
              calculatedPageCount > 0 ? calculatedPageCount - 1 : 0,
            )
          }
          disabled={!table.getCanNextPage() || disabled}
          title="Go to last page"
          size="icon"
        >
          <span className="sr-only">Go to last page</span>
          <ChevronsRight className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}
