// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Minimal markdown for model output: fenced code, headings, lists, GFM tables,
// paragraphs, inline code, bold, italic, and http(s) links. Output is React
// nodes only — never innerHTML — so model text cannot inject markup.

import type { ReactNode } from 'react';

const INLINE = /(`[^`]+`|\*\*[^*]+\*\*|\*[^*\s][^*]*\*|\[[^\]]+\]\(https?:\/\/[^)\s]+\))/g;

function inline(text: string, keyBase: string): ReactNode[] {
  const out: ReactNode[] = [];
  let last = 0;
  let i = 0;
  for (const m of text.matchAll(INLINE)) {
    const tok = m[0];
    const at = m.index ?? 0;
    if (at > last) out.push(text.slice(last, at));
    const key = `${keyBase}-${i++}`;
    if (tok.startsWith('`')) out.push(<code key={key}>{tok.slice(1, -1)}</code>);
    else if (tok.startsWith('**')) out.push(<strong key={key}>{tok.slice(2, -2)}</strong>);
    else if (tok.startsWith('[')) {
      const close = tok.indexOf('](');
      out.push(
        <a key={key} href={tok.slice(close + 2, -1)} target="_blank" rel="noopener noreferrer">
          {tok.slice(1, close)}
        </a>,
      );
    } else out.push(<em key={key}>{tok.slice(1, -1)}</em>);
    last = at + tok.length;
  }
  if (last < text.length) out.push(text.slice(last));
  return out;
}

type Align = 'left' | 'center' | 'right' | undefined;

type Block =
  | { t: 'code'; lang: string; body: string }
  | { t: 'h'; level: number; text: string }
  | { t: 'ul' | 'ol'; items: string[] }
  | { t: 'table'; head: string[]; align: Align[]; rows: string[][] }
  | { t: 'p'; text: string };

const DELIM_CELL = /^\s*:?-+:?\s*$/;
// the collapsed-table repair guesses more, so it wants unambiguous delimiters
const STRICT_DELIM_CELL = /^\s*:?-{3,}:?\s*$/;

function splitRow(line: string): string[] {
  let s = line.trim();
  if (s.startsWith('|')) s = s.slice(1);
  if (s.endsWith('|') && !s.endsWith('\\|')) s = s.slice(0, -1);
  return s.split(/(?<!\\)\|/).map((c) => c.replace(/\\\|/g, '|').trim());
}

function delimiterAlign(line: string): Align[] | null {
  if (!line.includes('-') || !line.includes('|')) return null;
  const cells = splitRow(line);
  if (!cells.every((c) => DELIM_CELL.test(c))) return null;
  return cells.map((c) => {
    const l = c.startsWith(':');
    const r = c.endsWith(':');
    return l && r ? 'center' : r ? 'right' : l ? 'left' : undefined;
  });
}

// Some models emit a whole table on one line, joining rows with "| |". Recover
// the rows from the delimiter row's column count; null when the line is not
// that shape.
function explodeCollapsedTable(line: string): string[] | null {
  const trimmed = line.trim();
  if (!trimmed.startsWith('|') || !/\|\s*:?-{3,}:?\s*\|/.test(trimmed)) return null;
  const tokens = trimmed.slice(1).split('|');
  const first = tokens.findIndex((t) => STRICT_DELIM_CELL.test(t));
  if (first < 1) return null;
  let n = 0;
  while (first + n < tokens.length && STRICT_DELIM_CELL.test(tokens[first + n]!)) n++;
  // header cells, then one whitespace-only token where the header row ended
  if (first !== n + 1 || tokens[first - 1]!.trim() !== '') return null;
  const rows = [tokens.slice(0, n), tokens.slice(first, first + n)];
  const rest = tokens.slice(first + n);
  for (let i = 0; i < rest.length; ) {
    if (rest.slice(i).every((t) => !t.trim())) break;
    if (rest[i]!.trim() === '') i++; // junction between rows
    rows.push(rest.slice(i, i + n));
    i += n;
  }
  if (rows.length < 3) return null;
  return rows.map((cells) => `| ${cells.map((c) => c.trim()).join(' | ')} |`);
}

export function parseBlocks(src: string): Block[] {
  const lines = src.replace(/\r\n/g, '\n').split('\n');
  const blocks: Block[] = [];
  let para: string[] = [];
  const flush = () => {
    if (para.length) blocks.push({ t: 'p', text: para.join(' ') });
    para = [];
  };
  for (let i = 0; i < lines.length; i++) {
    let line = lines[i]!;
    const fence = line.match(/^```(\S*)/);
    if (fence) {
      flush();
      const body: string[] = [];
      i++;
      while (i < lines.length && !lines[i]!.startsWith('```')) body.push(lines[i++]!);
      blocks.push({ t: 'code', lang: fence[1] ?? '', body: body.join('\n') });
      continue;
    }
    const exploded = explodeCollapsedTable(line);
    if (exploded) {
      lines.splice(i, 1, ...exploded);
      line = lines[i]!;
    }
    const align = line.includes('|') && i + 1 < lines.length ? delimiterAlign(lines[i + 1]!) : null;
    if (align) {
      const head = splitRow(line);
      if (head.length === align.length) {
        flush();
        const rows: string[][] = [];
        i += 2;
        while (i < lines.length && lines[i]!.trim() && lines[i]!.includes('|')) {
          const cells = splitRow(lines[i++]!);
          rows.push(Array.from({ length: head.length }, (_, c) => cells[c] ?? ''));
        }
        i--;
        blocks.push({ t: 'table', head, align, rows });
        continue;
      }
    }
    const h = line.match(/^(#{1,4})\s+(.*)$/);
    if (h) {
      flush();
      blocks.push({ t: 'h', level: h[1]!.length, text: h[2]! });
      continue;
    }
    const li = line.match(/^\s*([-*]|\d+[.)])\s+(.*)$/);
    if (li) {
      flush();
      const t = /\d/.test(li[1]!) ? 'ol' : 'ul';
      const prev = blocks[blocks.length - 1];
      if (prev && prev.t === t) prev.items.push(li[2]!);
      else blocks.push({ t, items: [li[2]!] });
      continue;
    }
    if (!line.trim()) {
      flush();
      continue;
    }
    para.push(line.trim());
  }
  flush();
  return blocks;
}

// Cells may carry <br> as a line break (models use it inside tables); it is
// matched as text and rendered as an element, never as markup.
function cell(text: string, key: string): ReactNode[] {
  return text.split(/<br\s*\/?>/i).flatMap((part, i) => [
    ...(i > 0 ? [<br key={`${key}-br${i}`} />] : []),
    ...inline(part.trim(), `${key}-${i}`),
  ]);
}

export function Markdown({ text }: { text: string }) {
  return (
    <div className="md">
      {parseBlocks(text).map((b, i) => {
        const k = `b${i}`;
        switch (b.t) {
          case 'code':
            return (
              <pre key={k} data-lang={b.lang || undefined}>
                <code>{b.body}</code>
              </pre>
            );
          case 'h': {
            const level = Math.min(b.level + 2, 6);
            const Tag = `h${level}` as 'h3';
            return <Tag key={k}>{inline(b.text, k)}</Tag>;
          }
          case 'ul':
          case 'ol': {
            const List = b.t;
            return (
              <List key={k}>
                {b.items.map((item, j) => (
                  <li key={j}>{inline(item, `${k}-${j}`)}</li>
                ))}
              </List>
            );
          }
          case 'table':
            return (
              <div key={k} className="md-table">
                <table>
                  <thead>
                    <tr>
                      {b.head.map((c, j) => (
                        <th key={j} style={{ textAlign: b.align[j] }}>
                          {cell(c, `${k}-h${j}`)}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {b.rows.map((row, r) => (
                      <tr key={r}>
                        {row.map((c, j) => (
                          <td key={j} style={{ textAlign: b.align[j] }}>
                            {cell(c, `${k}-${r}-${j}`)}
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            );
          default:
            return <p key={k}>{inline(b.text, k)}</p>;
        }
      })}
    </div>
  );
}
