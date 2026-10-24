// Run from any directory after `npm ci` in ui/. No network or AI generation.
import {readFile, writeFile, mkdir, copyFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import {createRequire} from 'node:module';
import path from 'node:path';

const mobile = fileURLToPath(new URL('../', import.meta.url));
const require = createRequire(path.join(mobile, '../ui/package.json'));
const sharp = require('sharp');
const source = path.join(mobile, '../ui/public/svgs/logo.svg');
const svg = await readFile(source, 'utf8');
const brand = svg.match(/<rect[^>]*fill="([^"]+)"/)[1];
async function png(relative, size, input = svg, opaque = false) {
  const target = path.join(mobile, relative);
  await mkdir(path.dirname(target), {recursive: true});
  let render = sharp(Buffer.from(input)).resize(size, size);
  if (opaque) render = render.flatten({background: brand}).removeAlpha();
  await render.png().toFile(target);
}
await mkdir(path.join(mobile, 'assets/brand'), {recursive: true});
await copyFile(source, path.join(mobile, 'assets/brand/logo.svg'));
await png('assets/brand/logo.png', 512);

for (const [density, scale] of Object.entries({mdpi: 1, hdpi: 1.5, xhdpi: 2, xxhdpi: 3, xxxhdpi: 4})) {
  await png(`android/app/src/main/res/mipmap-${density}/ic_launcher.png`, 48 * scale, svg, true);
  await png(`android/app/src/main/res/drawable-${density}/poker_launch.png`, 96 * scale);
}
// Adaptive icons reserve the outer 22/108 on every side for launcher masking.
const foreground = svg.replace('viewBox="0 0 64 64"', 'viewBox="-22 -22 108 108"').replace(/<rect[^>]*\/>/, '');
await png('android/app/src/main/res/drawable-xxxhdpi/poker_foreground.png', 432, foreground);
const paths = [...svg.matchAll(/<path[\s\S]*?\/>/g)].map(([p]) => {
  const data = p.match(/d="([^"]+)"/)[1];
  return `    <path android:pathData="${data}" android:fillColor="@android:color/transparent" android:strokeColor="#ffffff" android:strokeWidth="4" android:strokeLineCap="round" android:strokeLineJoin="round"/>`;
}).join('\n');
await writeFile(path.join(mobile, 'android/app/src/main/res/drawable/poker_monochrome.xml'),
  `<vector xmlns:android="http://schemas.android.com/apk/res/android" android:width="108dp" android:height="108dp" android:viewportWidth="108" android:viewportHeight="108">\n  <group android:translateX="22" android:translateY="22">\n${paths}\n  </group>\n</vector>\n`);
const iconDir = 'ios/Runner/Assets.xcassets/AppIcon.appiconset';
const manifest = JSON.parse(await readFile(path.join(mobile, iconDir, 'Contents.json'), 'utf8'));
for (const entry of manifest.images) {
  await png(`${iconDir}/${entry.filename}`, Math.round(parseFloat(entry.size) * parseFloat(entry.scale)), svg, true);
}
for (const scale of [1, 2, 3]) {
  await png(`ios/Runner/Assets.xcassets/LaunchImage.imageset/LaunchImage${scale === 1 ? '' : `@${scale}x`}.png`, 96 * scale);
}
console.log('Logo, launch images and launcher icons generated from ui/public/svgs/logo.svg.');
