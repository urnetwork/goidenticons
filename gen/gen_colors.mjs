// Generates ../colors.go from the Nanoidenticons color table.
// Usage: node gen_colors.mjs [path-to-Nanoidenticons-checkout]
import { readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const genDir = dirname(fileURLToPath(import.meta.url));
const nanoDir = process.argv[2] ?? join(genDir, '../../Nanoidenticons');

const src = readFileSync(join(nanoDir, 'src/nanoidenticons.mjs'), 'utf8');
const tableText = src.slice(src.indexOf('[', src.indexOf('colorCombinations')), src.indexOf('];') + 1);
const table = eval(tableText);
if (table.length !== 337) throw new Error(`unexpected table length ${table.length}`);

// shortest decimal form; the values are exact decimals from the source strings, so the
// emitted literals parse to the same float64s
const fmt = (v) => String(Number(v.endsWith?.('%') ? v.slice(0, -1) : v));

let out = `// Code generated from Nanoidenticons src/nanoidenticons.mjs (colorCombinations); do not edit by hand.

package goidenticons

// colorCombinations are the pre-defined color palettes, ported verbatim from the js library.
// Each combination is [background, foreground, spot]. Hue is in degrees, saturation and
// lightness are percentages.
var colorCombinations = [][3]hslColor{
`;
for (const combo of table) {
	const cs = combo.map((c) => `{h: ${fmt(c.h)}, s: ${fmt(c.s)}, l: ${fmt(c.l)}}`);
	out += `\t{${cs.join(', ')}},\n`;
}
out += `}\n`;
writeFileSync(join(genDir, '../colors.go'), out);
console.log('wrote colors.go,', table.length, 'combinations');
