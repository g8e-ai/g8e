// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { type ColumnDef } from '@tanstack/react-table';
import { DataTable } from '../src/components/DataTable';

type Row = { id: string; name: string };

const columns: ColumnDef<Row, unknown>[] = [
  { id: 'id', header: 'ID', accessorKey: 'id' },
  { id: 'name', header: 'Name', accessorKey: 'name' },
];

const data: Row[] = [
  { id: 'c', name: 'Charlie' },
  { id: 'a', name: 'Alice' },
  { id: 'b', name: 'Bob' },
];

describe('DataTable', () => {
  it('sorts rows when a sortable column header is clicked', async () => {
    const user = userEvent.setup();

    render(<DataTable data={data} columns={columns} caption="Sortable rows" enableColumnVisibility={false} />);

    const bodyCells = () =>
      Array.from(document.querySelectorAll('.data-table tbody tr')).map((row) =>
        row.querySelector('td')?.textContent,
      );

    expect(bodyCells()).toEqual(['c', 'a', 'b']);

    await user.click(screen.getByRole('button', { name: 'Sort by ID' }));

    expect(bodyCells()).toEqual(['a', 'b', 'c']);
  });
});
