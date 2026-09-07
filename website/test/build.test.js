import assert from 'node:assert/strict';
import test from 'node:test';
import { repositoryLink, renderWebsite, slugify } from '../src/build.js';

test('slugify creates stable heading anchors', () => {
  assert.equal(slugify('Proof of Human Presence'), 'proof-of-human-presence');
  assert.equal(slugify('L1–L5: Verify & Execute'), 'l1l5-verify-execute');
});

test('repositoryLink sends relative documents to the source repository', () => {
  assert.equal(repositoryLink('docs/guides/getting_started.md'), 'https://github.com/g8e-ai/g8e/blob/main/docs/guides/getting_started.md');
  assert.equal(repositoryLink('dashboard/'), 'https://github.com/g8e-ai/g8e/tree/main/dashboard');
  assert.equal(repositoryLink('#quick-start'), '#quick-start');
});

test('renderWebsite renders navigation, local images, and Mermaid diagrams', () => {
  const readme = '# Suite\n\n## Architecture\n\n<img src="docs/media/jit-mcp-with-receipts.png">\n\n```mermaid\ngraph LR\n  A --> B\n```\n\n[Guide](docs/guides/getting_started.md)';
  const html = renderWebsite(readme, '<nav>{{TOC}}</nav><main>{{CONTENT}}</main>');
  assert.match(html, /href="#architecture"/);
  assert.match(html, /src="\/assets\/jit-mcp-with-receipts\.png"/);
  assert.match(html, /<pre class="mermaid">/);
  assert.match(html, /github\.com\/g8e-ai\/g8e\/blob\/main\/docs\/guides\/getting_started\.md/);
});
