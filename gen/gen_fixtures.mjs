// Generates ../testdata/fixtures.json by running the real Nanoidenticons renderIcon
// against a fake recording canvas, at scale 4 (the natural 32px render), with the seed
// derived the same way the go package derives it: hex sha256 of the input.
// Usage: node gen_fixtures.mjs [path-to-Nanoidenticons-checkout]
import { createHash } from 'node:crypto';
import { writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const genDir = dirname(fileURLToPath(import.meta.url));
const nanoDir = process.argv[2] ?? join(genDir, '../../Nanoidenticons');
const { renderIcon } = await import(pathToFileURL(join(nanoDir, 'src/nanoidenticons.mjs')));

class FakeContext {
	constructor() {
		this._fillStyle = null;
		this.fills = [];
		this.bg = null;
		this._path = null;
	}
	set fillStyle(v) { this._fillStyle = v; }
	get fillStyle() { return this._fillStyle; }
	fillRect(x, y, w, h) { this.bg = { style: this._fillStyle, x, y, w, h }; }
	beginPath() { this._path = []; }
	arc(x, y, r) { this._path.push({ op: 'arc', x, y, r }); }
	moveTo(x, y) { this._path.push({ op: 'moveTo', x, y }); }
	lineTo(x, y) { this._path.push({ op: 'lineTo', x, y }); }
	bezierCurveTo(c1x, c1y, c2x, c2y, x, y) { this._path.push({ op: 'cubeTo', c1x, c1y, c2x, c2y, x, y }); }
	fill() { this.fills.push({ style: this._fillStyle, path: this._path }); }
}

// mirror of the go pipeline, used to also record the expected cells and color index
function predict(seed) {
	const randseed = [0n, 0n, 0n, 0n];
	const toInt32 = (v) => BigInt.asIntN(32, v);
	for (let i = 0; i < seed.length; i++) {
		const k = i % 4;
		const raw = randseed[k];
		randseed[k] = BigInt.asIntN(32, toInt32(raw) << 5n) - raw + BigInt(seed.charCodeAt(i));
	}
	let s = randseed.map((v) => Number(toInt32(v)) | 0);
	const rand = () => {
		const t = s[0] ^ (s[0] << 11);
		s[0] = s[1]; s[1] = s[2]; s[2] = s[3];
		s[3] = (s[3] ^ (s[3] >> 19) ^ t ^ (t >> 8)) | 0;
		return (s[3] >>> 0) / ((1 << 31) >>> 0);
	};
	for (let i = 0; i < 18; i++) rand(); // preserve v1 pattern
	const cells = [];
	for (let y = 0; y < 8; y++) {
		let row = [];
		for (let x = 0; x < 4; x++) row[x] = Math.floor(rand() * 2.3);
		cells.push(...row.concat(row.slice(0, 4).reverse()));
	}
	return { cells, colorIndex: Math.floor(rand() * 337) };
}

const inputs = ['urnetwork', 'hello world', '', 'a', 'identicon test 1234', 'ur éÿ'];
const fixtures = [];
for (const input of inputs) {
	const seed = createHash('sha256').update(Buffer.from(input, 'utf8')).digest('hex');
	const p = predict(seed);
	const cc = new FakeContext();
	const canvas = { width: 0, height: 0, getContext() { return cc; } };
	renderIcon({ seed, scale: 4 }, canvas);
	fixtures.push({
		input,
		seed,
		predictedCells: p.cells,
		predictedColorIndex: p.colorIndex,
		actual: { width: canvas.width, height: canvas.height, bg: cc.bg, fills: cc.fills },
	});
}
writeFileSync(join(genDir, '../testdata/fixtures.json'), JSON.stringify(fixtures, null, 1));
console.log('wrote fixtures.json,', fixtures.length, 'fixtures');
