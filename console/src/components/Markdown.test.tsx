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
});
