import { readdir, readFile, writeFile } from 'node:fs/promises';
import { extname, relative } from 'node:path';

const projectRoot = new URL('../', import.meta.url);
const distRoot = new URL('dist/', projectRoot);
const expectedRuntime = '{\n  "schema_version": "1.0.0",\n  "mirror_origin": "https://opendevops.ai"\n}\n';
const allowedRootFiles = new Set(['_headers', 'index.html', 'runtime.json']);
const allowedAssetExtensions = new Set(['.css', '.js']);
const prohibited = [
  /http:\/\/(?:localhost|127\.0\.0\.1|\[::1\]):(?:8080|8081|8082|8443)\b/i,
  /spiffe:\/\//i,
  /\/home\/bob\//i,
  /(?:CLOUDFLARE_API_TOKEN|CLOUDFLARE_ACCOUNT_ID|G8E_PUBLIC_INGEST_TOKEN)/,
  /(?:Using local fixture data|run-live-demo-20260914)/,
];

async function filesUnder(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const url = new URL(entry.name, directory);
    if (entry.isDirectory()) files.push(...await filesUnder(new URL(`${entry.name}/`, directory)));
    else if (entry.isFile()) files.push(url);
    else throw new Error(`dist contains unsupported entry: ${entry.name}`);
  }
  return files;
}

const productionRuntime = await readFile(new URL('runtime.production.json', projectRoot), 'utf8');
if (productionRuntime !== expectedRuntime) throw new Error('runtime.production.json does not match the production contract');
await writeFile(new URL('runtime.json', distRoot), productionRuntime, 'utf8');

const files = await filesUnder(distRoot);
for (const file of files) {
  const path = relative(distRoot.pathname, file.pathname);
  const topLevel = !path.includes('/');
  if (topLevel && !allowedRootFiles.has(path)) throw new Error(`unexpected dist root file: ${path}`);
  if (!topLevel && (!path.startsWith('assets/') || !allowedAssetExtensions.has(extname(path)))) {
    throw new Error(`unexpected dist asset: ${path}`);
  }
  if (extname(path) === '.map') throw new Error(`source map is prohibited: ${path}`);
  const content = await readFile(file, 'utf8');
  for (const pattern of prohibited) {
    if (pattern.test(content)) throw new Error(`prohibited production artifact content in ${path}: ${pattern}`);
  }
}

console.log(`Prepared and verified ${files.length} production assets.`);
