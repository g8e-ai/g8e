import { cp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { dirname, extname, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import MarkdownIt from 'markdown-it';

const SOURCE_DIRECTORY = dirname(fileURLToPath(import.meta.url));
const WEBSITE_DIRECTORY = resolve(SOURCE_DIRECTORY, '..');
const REPOSITORY_DIRECTORY = resolve(WEBSITE_DIRECTORY, '..');
const OUTPUT_DIRECTORY = resolve(WEBSITE_DIRECTORY, 'dist');
const STATIC_DIRECTORY = resolve(WEBSITE_DIRECTORY, 'static');
const README_PATH = resolve(REPOSITORY_DIRECTORY, 'README.md');
const TEMPLATE_PATH = resolve(SOURCE_DIRECTORY, 'template.html');
const STYLES_PATH = resolve(SOURCE_DIRECTORY, 'styles.css');
const SCRIPT_PATH = resolve(SOURCE_DIRECTORY, 'site.js');
const REPOSITORY_URL = 'https://github.com/g8e-ai/g8e';
const RAW_REPOSITORY_URL = 'https://raw.githubusercontent.com/g8e-ai/g8e/main';
const LOCAL_IMAGES = new Map([
  ['docs/media/jit-mcp-with-receipts.png', 'assets/jit-mcp-with-receipts.png'],
  ['docs/diagrams/g8e-diagram.png', 'assets/g8e-diagram.png']
]);

export function slugify(value) {
  return value.toLowerCase().trim().replace(/<[^>]*>/g, '').replace(/[^\p{L}\p{N}\s-]/gu, '').replace(/\s+/g, '-').replace(/-+/g, '-');
}

export function repositoryLink(href) {
  if (!href || href.startsWith('#') || /^(?:[a-z]+:|\/)/i.test(href)) return href;
  const [path, fragment] = href.split('#', 2);
  const location = path.endsWith('/') ? `${REPOSITORY_URL}/tree/main/${path.replace(/\/$/, '')}` : `${REPOSITORY_URL}/blob/main/${path}`;
  return fragment ? `${location}#${fragment}` : location;
}

function configureMarkdown(headings) {
  const markdown = new MarkdownIt({ html: true, linkify: true, typographer: true });
  const slugs = new Map();
  markdown.core.ruler.push('g8e_heading_ids', state => {
    for (let index = 0; index < state.tokens.length; index += 1) {
      const token = state.tokens[index];
      if (token.type !== 'heading_open') continue;
      const text = state.tokens[index + 1].content;
      const base = slugify(text) || 'section';
      const count = slugs.get(base) ?? 0;
      slugs.set(base, count + 1);
      const id = count === 0 ? base : `${base}-${count + 1}`;
      token.attrSet('id', id);
      token.meta = { id };
      headings.push({ id, level: Number(token.tag.slice(1)), text });
    }
  });
  const defaultHeadingOpen = markdown.renderer.rules.heading_open ?? ((tokens, index, options, environment, renderer) => renderer.renderToken(tokens, index, options));
  markdown.renderer.rules.heading_open = (tokens, index, options, environment, renderer) => {
    const token = tokens[index];
    return `${defaultHeadingOpen(tokens, index, options, environment, renderer)}<a class="header-anchor" href="#${token.meta.id}" aria-label="Link to this section">#</a>`;
  };
  const defaultLinkOpen = markdown.renderer.rules.link_open ?? ((tokens, index, options, environment, renderer) => renderer.renderToken(tokens, index, options));
  markdown.renderer.rules.link_open = (tokens, index, options, environment, renderer) => {
    const href = tokenAttribute(tokens[index], 'href');
    if (href) tokens[index].attrSet('href', repositoryLink(href));
    const target = tokens[index].attrGet('href');
    if (target?.startsWith('http')) tokens[index].attrSet('rel', 'noopener noreferrer');
    return defaultLinkOpen(tokens, index, options, environment, renderer);
  };
  const defaultImage = markdown.renderer.rules.image;
  markdown.renderer.rules.image = (tokens, index, options, environment, renderer) => {
    const source = tokenAttribute(tokens[index], 'src');
    if (source && !/^(?:[a-z]+:|\/)/i.test(source)) tokens[index].attrSet('src', imageURL(source));
    return defaultImage(tokens, index, options, environment, renderer);
  };
  const defaultFence = markdown.renderer.rules.fence;
  markdown.renderer.rules.fence = (tokens, index, options, environment, renderer) => {
    const token = tokens[index];
    if (token.info.trim() === 'mermaid') return `<pre class="mermaid">${markdown.utils.escapeHtml(token.content)}</pre>`;
    return defaultFence(tokens, index, options, environment, renderer);
  };
  return markdown;
}

function tokenAttribute(token, name) {
  return token.attrGet(name);
}

function imageURL(source) {
  return LOCAL_IMAGES.has(source) ? `/${LOCAL_IMAGES.get(source)}` : `${RAW_REPOSITORY_URL}/${source}`;
}

function prepareMarkdown(markdown) {
  return markdown.replace(/^> \[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]$/gm, (_, label) => `> **${label[0]}${label.slice(1).toLowerCase()}**`).replace(/(<img\s[^>]*src=["'])([^"']+)(["'])/gi, (match, prefix, source, suffix) => {
    if (/^(?:[a-z]+:|\/)/i.test(source)) return match;
    return `${prefix}${imageURL(source)}${suffix}`;
  });
}

function renderTableOfContents(headings) {
  const links = headings.filter(heading => heading.level === 2).map(heading => `<li><a href="#${heading.id}">${escapeHTML(heading.text)}</a></li>`).join('');
  return `<ul>${links}</ul>`;
}

function escapeHTML(value) {
  return value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;').replaceAll("'", '&#39;');
}

export function renderWebsite(readme, template) {
  const headings = [];
  const markdown = configureMarkdown(headings);
  const content = markdown.render(prepareMarkdown(readme));
  return template.replace('{{TOC}}', renderTableOfContents(headings)).replace('{{CONTENT}}', content);
}

async function build() {
  const [readme, template, styles, script] = await Promise.all([
    readFile(README_PATH, 'utf8'),
    readFile(TEMPLATE_PATH, 'utf8'),
    readFile(STYLES_PATH, 'utf8'),
    readFile(SCRIPT_PATH, 'utf8')
  ]);
  const html = renderWebsite(readme, template);
  if (process.argv.includes('--check')) {
    if (!html.includes('id="proof-not-promises"') || !html.includes('id="four-components-one-governance-boundary"') || !html.includes('class="mermaid"') || html.includes('href="docs/')) throw new Error('generated website validation failed');
    return;
  }
  await rm(OUTPUT_DIRECTORY, { recursive: true, force: true });
  await mkdir(resolve(OUTPUT_DIRECTORY, 'assets'), { recursive: true });
  await cp(STATIC_DIRECTORY, OUTPUT_DIRECTORY, { recursive: true });
  await Promise.all([
    writeFile(resolve(OUTPUT_DIRECTORY, 'index.html'), html),
    writeFile(resolve(OUTPUT_DIRECTORY, 'styles.css'), styles),
    writeFile(resolve(OUTPUT_DIRECTORY, 'site.js'), script),
    writeFile(resolve(OUTPUT_DIRECTORY, 'llms-full.txt'), readme),
    writeFile(resolve(OUTPUT_DIRECTORY, 'llms.txt'), '# g8e AI Data and Execution Governance\n\nThe canonical project overview is available at https://g8e.ai/llms-full.txt and the source repository is https://github.com/g8e-ai/g8e.\n')
  ]);
  await Promise.all([...LOCAL_IMAGES].map(async ([source, destination]) => {
    if (!extname(source)) throw new Error(`invalid image path: ${source}`);
    await cp(resolve(REPOSITORY_DIRECTORY, source), resolve(OUTPUT_DIRECTORY, destination));
  }));
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
  build().catch(error => {
    process.stderr.write(`${error.message}\n`);
    process.exitCode = 1;
  });
}
