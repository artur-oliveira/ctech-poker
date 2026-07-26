// Generates every deck color variant from the shape templates in
// scripts/card-templates/ (one SVG per suit+rank, colored in the four-color
// palette). Each variant is a literal hex swap of that suit's four-color
// value for the variant's value, written to public/svgs/variants/{variantId}/.
// Run with: npm run cards:variants
import {readFileSync, writeFileSync, mkdirSync, readdirSync} from 'node:fs';
import {join, dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
import {DECK_VARIANTS, DEFAULT_DECK_VARIANT, type Suit} from '../src/lib/cardVariants.ts';

const root = dirname(fileURLToPath(import.meta.url));
const templatesDir = join(root, 'card-templates');
const outputDir = join(root, '..', 'public', 'svgs', 'variants');
const sourceColors = DECK_VARIANTS[DEFAULT_DECK_VARIANT].colors;

const templateFiles = readdirSync(templatesDir);

for (const [variantId, variant] of Object.entries(DECK_VARIANTS)) {
  const outDir = join(outputDir, variantId);
  mkdirSync(outDir, {recursive: true});
  for (const file of templateFiles) {
    const suit = file.split('-')[0] as Suit;
    const svg = readFileSync(join(templatesDir, file), 'utf8');
    writeFileSync(join(outDir, file), svg.split(sourceColors[suit]).join(variant.colors[suit]));
  }
  console.log(`generated ${variantId}`);
}
