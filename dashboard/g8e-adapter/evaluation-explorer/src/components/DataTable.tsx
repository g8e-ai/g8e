// Accessible sortable table built on TanStack Table. Renders semantic
// <table> markup with proper <th scope>, sort announcements, and a
// screen-reader table alternative is provided by the semantic markup itself.
// Includes column-visibility controls so users can hide columns on wide
// tables (e.g. the 1,125-row assignment table) and keep the view scannable.

import { type ReactNode, useMemo, useState } from 'react';
import {
  type ColumnDef,
  type SortingState,
  type VisibilityState,
  flexRender,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table';

export interface DataTableProps<T> {
  data: T[];
  columns: ColumnDef<T, unknown>[];
  empty?: ReactNode;
  pageSize?: number;
  caption?: string;
  /** When true, renders the column-visibility toggle panel. Defaults to true. */
  enableColumnVisibility?: boolean;
}

export function DataTable<T>({
  data,
  columns,
  empty,
  pageSize = 50,
  caption,
  enableColumnVisibility = true,
}: DataTableProps<T>) {
  const [sorting, setSorting] = useState<SortingState>([]);
  const [page, setPage] = useState(0);
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({});
  const [showColumns, setShowColumns] = useState(false);

  const table = useReactTable({
    data,
    columns,
    state: { sorting, columnVisibility },
    onSortingChange: setSorting,
    onColumnVisibilityChange: setColumnVisibility,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  });

  const pageCount = Math.ceil(data.length / pageSize);
  const safePage = Math.min(page, Math.max(0, pageCount - 1));
  const pageRows = useMemo(() => {
    const rows = table.getRowModel().rows;
    const start = safePage * pageSize;
    return rows.slice(start, start + pageSize);
  }, [table, safePage, pageSize, data.length]);

  if (data.length === 0) {
    return <>{empty ?? <p className="table-empty">No rows.</p>}</>;
  }

  const toggleableColumns = table.getAllLeafColumns().filter((c) => c.getCanHide());

  return (
    <div className="data-table-wrap" data-testid="data-table">
      {enableColumnVisibility && toggleableColumns.length > 1 ? (
        <div className="column-visibility">
          <button
            type="button"
            className="column-visibility-toggle"
            aria-expanded={showColumns}
            aria-controls="column-visibility-panel"
            onClick={() => setShowColumns((v) => !v)}
          >
            Columns {showColumns ? '▲' : '▼'}
          </button>
          {showColumns ? (
            <ul id="column-visibility-panel" className="column-visibility-panel" role="group" aria-label="Toggle columns">
              {toggleableColumns.map((column) => {
                const label = String(column.columnDef.header ?? column.id);
                return (
                  <li key={column.id}>
                    <label className="column-visibility-item">
                      <input
                        type="checkbox"
                        checked={column.getIsVisible()}
                        onChange={column.getToggleVisibilityHandler()}
                      />
                      <span>{label}</span>
                    </label>
                  </li>
                );
              })}
            </ul>
          ) : null}
        </div>
      ) : null}
      <table className="data-table">
        {caption ? <caption className="sr-only">{caption}</caption> : null}
        <thead>
          {table.getHeaderGroups().map((group) => (
            <tr key={group.id}>
              {group.headers.map((header) => {
                const sortDir = header.column.getIsSorted();
                const isSorted = sortDir !== false;
                return (
                  <th
                    key={header.id}
                    scope="col"
                    aria-sort={isSorted ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'}
                  >
                    {header.column.getCanSort() ? (
                      <button
                        type="button"
                        className="th-sort"
                        onClick={header.column.getToggleSortingHandler()}
                        aria-label={`Sort by ${String(header.column.columnDef.header ?? header.id)}`}
                      >
                        {flexRender(header.column.columnDef.header, header.getContext())}
                        <span className="sort-indicator" aria-hidden="true">
                          {isSorted ? (sortDir === 'asc' ? '▲' : '▼') : '↕'}
                        </span>
                      </button>
                    ) : (
                      flexRender(header.column.columnDef.header, header.getContext())
                    )}
                  </th>
                );
              })}
            </tr>
          ))}
        </thead>
        <tbody>
          {pageRows.map((row) => (
            <tr key={row.id}>
              {row.getVisibleCells().map((cell) => (
                <td key={cell.id}>{flexRender(cell.column.columnDef.cell, cell.getContext())}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
      {pageCount > 1 ? (
        <div className="table-pagination" data-testid="table-pagination">
          <button
            type="button"
            onClick={() => setPage((p) => Math.max(0, p - 1))}
            disabled={safePage === 0}
            aria-label="Previous page"
          >
            Previous
          </button>
          <span className="page-info">
            Page {safePage + 1} of {pageCount} ({data.length} rows)
          </span>
          <button
            type="button"
            onClick={() => setPage((p) => Math.min(pageCount - 1, p + 1))}
            disabled={safePage >= pageCount - 1}
            aria-label="Next page"
          >
            Next
          </button>
        </div>
      ) : null}
    </div>
  );
}
