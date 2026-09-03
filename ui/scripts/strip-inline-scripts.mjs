// Postbuild step for the static export: rewrites every inline <script> in out/**/*.html
// into a content-addressed file under /_next/static/chunks/inline/, so the deployed CSP
// can be `script-src 'self' …` with no 'unsafe-inline' and no hash list. See #46 / #120.
//
// Why not per-build sha256 hashes: the export emits one *distinct* inline script per page
// (the page's own `self.__next_f.push([1,…])` flight payload). 25 pages measured at 25
// distinct hashes ≈ 1.4 kB of `'sha256-…'` on top of a ~0.65 kB policy, and Cloudflare's
// `_headers` caps a single header line at 2,000 characters (asserted by
// ctech-cdk/.github/workflows/frontend-cloudflare.yml). A hash list would break the deploy
// on the next few routes added; externalizing is O(1) in the header.
//
// The rewrite is positional: each tag is replaced in place by a *synchronous* `<script src>`,
// which the HTML spec runs in document order relative to the surrounding scripts, so the
// `push([0])` → `push([1,…])` ordering the flight runtime relies on is preserved. The chunk
// <script> tags around them are `async` and already unordered.
import {createHash} from 'node:crypto';
import {mkdirSync, readFileSync, readdirSync, writeFileSync} from 'node:fs';
import {join, relative} from 'node:path';

export const INLINE_DIR = '_next/static/chunks/inline';

const INLINE_SCRIPT_SOURCE = '<script(?![^>]*\\bsrc=)([^>]*)>([\\s\\S]*?)</script>';
const inlineScripts = () => new RegExp(INLINE_SCRIPT_SOURCE, 'gi');

/**
 * @param {string} html
 * @param {string} label file path, for error messages only
 * @returns {{html: string, assets: Map<string, string>}} rewritten HTML and name -> body
 */
export function externalizeInlineScripts(html, label = '<html>') {
  const assets = new Map();
  const out = html.replace(inlineScripts(), (tag, attrs, body) => {
    const rest = attrs.trim();
    if (rest) {
      // A typed (`application/ld+json`) or attributed (`nonce`, `type="module"`) inline
      // script is not something this rewrite has been reasoned about; refuse instead of
      // silently changing its semantics.
      throw new Error(`${label}: inline <script ${rest}> is not a bare inline script`);
    }
    if (!body.trim()) return tag;
    const name = `${createHash('sha256').update(body).digest('hex').slice(0, 32)}.js`;
    assets.set(name, body);
    return `<script src="/${INLINE_DIR}/${name}"></script>`;
  });
  return {html: out, assets};
}

function htmlFiles(dir) {
  return readdirSync(dir, {withFileTypes: true}).flatMap((entry) => {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) return htmlFiles(full);
    return entry.name.endsWith('.html') ? [full] : [];
  });
}

function main() {
  const outDir = join(process.cwd(), 'out');
  let pages = 0;
  let rewritten = 0;
  const written = new Set();
  for (const file of htmlFiles(outDir)) {
    pages += 1;
    const {html, assets} = externalizeInlineScripts(readFileSync(file, 'utf8'), relative(outDir, file));
    if (!assets.size) continue;
    mkdirSync(join(outDir, INLINE_DIR), {recursive: true});
    for (const [name, body] of assets) {
      if (!written.has(name)) {
        writeFileSync(join(outDir, INLINE_DIR, name), body);
        written.add(name);
      }
      rewritten += 1;
    }
    writeFileSync(file, html);
    if (inlineScripts().test(html)) throw new Error(`${file}: inline scripts survived the rewrite`);
  }
  console.log(`strip-inline-scripts: ${rewritten} inline scripts in ${pages} pages -> ${written.size} files in ${INLINE_DIR}/`);
}

if (process.argv[1] && import.meta.url === `file://${process.argv[1]}`) main();
