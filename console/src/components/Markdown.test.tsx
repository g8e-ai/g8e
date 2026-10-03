// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Markdown, parseBlocks } from './Markdown';

describe('parseBlocks', () => {
  it('splits code fences, headings, lists, and paragraphs', () => {
    const blocks = parseBlocks('# Title\n\nSome text\nmore\n\n- a\n- b\n\n```sh\nls -la\n```');
    expect(blocks.map((b) => b.t)).toEqual(['h', 'p', 'ul', 'code']);
  });

  it('parses GFM tables with alignment and ragged rows', () => {
    const blocks = parseBlocks('intro\n| A | B | C |\n|:--|:-:|--:|\n| 1 | 2 |\n| x \\| y | 4 | 5 |\n\nafter');
    expect(blocks.map((b) => b.t)).toEqual(['p', 'table', 'p']);
    const t = blocks[1]!;
    if (t.t !== 'table') throw new Error('expected table');
    expect(t.head).toEqual(['A', 'B', 'C']);
    expect(t.align).toEqual(['left', 'center', 'right']);
    expect(t.rows).toEqual([['1', '2', ''], ['x | y', '4', '5']]);
  });

  it('does not treat a lone pipe line as a table', () => {
    expect(parseBlocks('a | b\nc | d').map((b) => b.t)).toEqual(['p']);
  });

  it('recovers a table collapsed onto one line', () => {
    const collapsed = '| Phase | Goal | |---|---| | 1 | Define | | 2 | Build | |';
    const blocks = parseBlocks(collapsed);
    expect(blocks.map((b) => b.t)).toEqual(['table']);
    const t = blocks[0]!;
    if (t.t !== 'table') throw new Error('expected table');
    expect(t.head).toEqual(['Phase', 'Goal']);
    expect(t.rows).toEqual([['1', 'Define'], ['2', 'Build']]);
  });

  it('recovers the collapsed multi-phase table models emit', () => {
    const line =
      '| Phase | Goal | Key deliverables | |-------|------|-----------------| | 1️⃣ | Define the puzzle concept | • Puzzle description <br>• List of shapes/lines | | 2️⃣ | Set up Three.js | • `index.html` with <script> <br>• Boilerplate | | 3️⃣ | Load map data | • GeoJSON | |';
    const t = parseBlocks(line)[0]!;
    if (t.t !== 'table') throw new Error('expected table');
    expect(t.head).toEqual(['Phase', 'Goal', 'Key deliverables']);
    expect(t.rows).toHaveLength(3);
    expect(t.rows[1]).toEqual(['2️⃣', 'Set up Three.js', '• `index.html` with <script> <br>• Boilerplate']);
  });

  it('leaves pipes inside code fences alone', () => {
    expect(parseBlocks('```\n| a | b |\n|---|---|\n```').map((b) => b.t)).toEqual(['code']);
  });
});

describe('Markdown', () => {
  it('renders model text without interpreting HTML', () => {
    const { container } = render(<Markdown text={'<img src=x onerror=alert(1)> **bold** [ok](https://example.com) [bad](javascript:alert(1))'} />);
    expect(container.querySelector('img')).toBeNull();
    expect(container.querySelector('strong')?.textContent).toBe('bold');
    const links = container.querySelectorAll('a');
    expect(links).toHaveLength(1);
    expect(links[0]!.getAttribute('href')).toBe('https://example.com');
    expect(links[0]!.getAttribute('rel')).toBe('noopener noreferrer');
  });

  it('renders tables with inline markup and <br> as elements only', () => {
    const { container } = render(
      <Markdown text={'| Name | Notes |\n|---|---|\n| `ls` | **fast**<br>• safe<br><script>x</script> |'} />,
    );
    expect(container.querySelectorAll('th')).toHaveLength(2);
    const td = container.querySelectorAll('td');
    expect(td[0]!.querySelector('code')?.textContent).toBe('ls');
    expect(td[1]!.querySelector('strong')?.textContent).toBe('fast');
    expect(td[1]!.querySelectorAll('br')).toHaveLength(2);
    expect(container.querySelector('script')).toBeNull();
  });
});
