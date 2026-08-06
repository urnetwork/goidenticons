// Package goidenticons generates identicon png images from arbitrary byte slices.
//
// The engine is a port of the Nanoidenticons js library (wtfpl, itself based on
// Blockies). The pattern and geometry are an exact match of the js render for the
// same seed string (verified against recordings of the js library in the tests).
//
// Two frozen color schemes share the engine (see the compatibility contract in the
// README). For a given input both schemes produce the same cell pattern; only the
// color table differs:
//   - v1 (RenderImage, RenderPng): the js library's palette, an exact color match
//     of the js render.
//   - v2 (RenderImageV2, RenderPngV2): the URnetwork brand palette.
//
// The input bytes are reduced to the prng seed by hashing: the seed string is the
// lowercase hex sha256 digest of the input, which maps arbitrary length input onto the
// 4x32 bit prng state with a uniform distribution.
//
// An icon is 8x8 cells, so the natural render sizes are multiples of 32 px (cell scale
// multiples of 4, the scales the js library was designed around). A requested target
// size is rounded to the nearest multiple of 32, rendered, then resampled to fit the
// target square exactly.
//
// Package-level functions are safe for concurrent use.
package goidenticons

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"unicode/utf16"

	"golang.org/x/image/draw"
	"golang.org/x/image/vector"
)

// icon geometry fixed by the js library
const (
	// iconCells is the number of cells per icon side
	iconCells = 8
	// renderSizeUnit is the natural render size step in px: iconCells cells at the
	// base cell scale of 4 px
	renderSizeUnit = 32
)

// hslColor is a color in hsl space: hue in degrees, saturation and lightness in percent.
type hslColor struct {
	h float64
	s float64
	l float64
}

// rgba converts to 8 bit srgb, fully opaque, following the css hsl conversion that the
// js library relies on the canvas to do.
func (self hslColor) rgba() color.RGBA {
	h := math.Mod(self.h, 360)
	if h < 0 {
		h += 360
	}
	s := self.s / 100
	l := self.l / 100
	c := (1 - math.Abs(2*l-1)) * s
	hPrime := h / 60
	x := c * (1 - math.Abs(math.Mod(hPrime, 2)-1))
	m := l - c/2
	var r1, g1, b1 float64
	switch {
	case hPrime < 1:
		r1, g1, b1 = c, x, 0
	case hPrime < 2:
		r1, g1, b1 = x, c, 0
	case hPrime < 3:
		r1, g1, b1 = 0, c, x
	case hPrime < 4:
		r1, g1, b1 = 0, x, c
	case hPrime < 5:
		r1, g1, b1 = x, 0, c
	default:
		r1, g1, b1 = c, 0, x
	}
	return color.RGBA{
		R: uint8(math.Round(255 * (r1 + m))),
		G: uint8(math.Round(255 * (g1 + m))),
		B: uint8(math.Round(255 * (b1 + m))),
		A: 255,
	}
}

// xorshiftRand is the xorshift prng of the js library, with js number semantics.
// Not safe for concurrent use; create one per render.
type xorshiftRand struct {
	state [4]int32
}

// newXorshiftRand seeds the prng from the seed string with the same arithmetic as the js
// seedrand (Java String.hashCode expanded to 4 32 bit values). The js accumulator holds
// unwrapped intermediate values exactly in float64; int64 holds them exactly here (they
// grow by at most 2^31+2^16 per character, so any practical seed length fits). Each use
// of an accumulator wraps to int32, like the js << coercion.
func newXorshiftRand(seed string) *xorshiftRand {
	// iterate utf16 code units to match js charCodeAt; seeds are ascii hex in practice
	rawValues := [4]int64{}
	for i, c := range utf16.Encode([]rune(seed)) {
		k := i % 4
		raw := rawValues[k]
		rawValues[k] = int64(int32(raw)<<5) - raw + int64(c)
	}
	state := [4]int32{}
	for i, raw := range rawValues {
		state[i] = int32(raw)
	}
	return &xorshiftRand{
		state: state,
	}
}

// rand returns the next value in [0, 1). The js library divides a uint32 by 2^31, which
// looks like it could reach 2, but the signed >> shifts make the sign bits cancel in the
// xor chain, so the new state word is always non negative and the result is always < 1.
func (self *xorshiftRand) rand() float64 {
	t := self.state[0] ^ (self.state[0] << 11)
	self.state[0] = self.state[1]
	self.state[1] = self.state[2]
	self.state[2] = self.state[3]
	self.state[3] = self.state[3] ^ (self.state[3] >> 19) ^ t ^ (t >> 8)
	return float64(uint32(self.state[3])) / (1 << 31)
}

// seedForData reduces arbitrary input bytes to the prng seed string, the lowercase hex
// sha256 digest of the input.
func seedForData(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// newIconRand creates the render prng for a seed, applying the 18 warmup calls the js
// library uses to preserve its v1 patterns.
func newIconRand(seed string) *xorshiftRand {
	iconRand := newXorshiftRand(seed)
	for i := 0; i < 18; i++ {
		iconRand.rand()
	}
	return iconRand
}

// createCellValues generates the iconCells x iconCells cells in row major order.
// Values: 0 background, 1 foreground, 2 spot. Each row generates the left half and
// mirrors it, so icons are symmetric about the vertical axis.
func createCellValues(iconRand *xorshiftRand) []int {
	// floor(rand * 2.3) gives background and foreground ~43% each and spot ~13%
	dataWidth := iconCells / 2
	cellValues := make([]int, 0, iconCells*iconCells)
	for y := 0; y < iconCells; y++ {
		rowValues := make([]int, dataWidth)
		for x := 0; x < dataWidth; x++ {
			rowValues[x] = int(math.Floor(iconRand.rand() * 2.3))
		}
		cellValues = append(cellValues, rowValues...)
		for x := dataWidth - 1; 0 <= x; x-- {
			cellValues = append(cellValues, rowValues[x])
		}
	}
	return cellValues
}

// combinationIndexForRand picks the color combination index like the js addColorOpts.
// rand() < 1 always, so the index is always in range.
func combinationIndexForRand(iconRand *xorshiftRand, combinations [][3]hslColor) int {
	return int(math.Floor(iconRand.rand() * float64(len(combinations))))
}

// pathOpKind enumerates the fill path drawing ops.
type pathOpKind int

const (
	pathOpCircle pathOpKind = iota
	pathOpMoveTo
	pathOpLineTo
	pathOpCubeTo
)

// pathOp is one drawing op of a fill path. Coordinate use by kind:
// circle: center (x0, y0) with radius x1; moveTo and lineTo: point (x0, y0);
// cubeTo: control points (x0, y0) and (x1, y1) with end point (x2, y2).
type pathOp struct {
	kind pathOpKind
	x0   float64
	y0   float64
	x1   float64
	y1   float64
	x2   float64
	y2   float64
}

// fillPath is one filled shape, a circle or a connection ribbon, in a single color role.
type fillPath struct {
	// colorRole selects the fill color like the js fillStyle choice:
	// 1 is foreground, any other non background value is spot
	colorRole int
	ops       []pathOp
}

// connectionOps builds the ribbon connecting the cell at (row, col) to its diagonal
// neighbor one row down in direction dir: -1 south west, +1 south east. The shape is a
// direct port of the js bezier construction, which is horizontally mirrored by dir.
func connectionOps(row float64, col float64, scale float64, dir float64) []pathOp {
	x := func(d float64) float64 {
		return scale/2 + (col+dir*d)*scale
	}
	y := func(d float64) float64 {
		return scale/2 + (row+d)*scale
	}
	return []pathOp{
		{kind: pathOpMoveTo, x0: x(0.40), y0: y(-0.20)},
		{kind: pathOpCubeTo, x0: x(0.40), y0: y(0.40), x1: x(0.60), y1: y(0.60), x2: x(1.20), y2: y(0.60)},
		{kind: pathOpLineTo, x0: x(0.60), y0: y(1.20)},
		{kind: pathOpCubeTo, x0: x(0.60), y0: y(0.60), x1: x(0.40), y1: y(0.40), x2: x(-0.20), y2: y(0.40)},
	}
}

// createFillPaths lays out the filled shapes for the cells at the given cell scale in px:
// a circle per non background cell, plus ribbons connecting equal valued diagonal
// neighbors to the south west and south east, in the same paint order as the js render.
func createFillPaths(cellValues []int, scale float64) []fillPath {
	circleOffset := scale / 2
	circleRadius := circleOffset * 0.9
	fillPaths := []fillPath{}
	for i, cellValue := range cellValues {
		if cellValue == 0 {
			continue
		}
		row := float64(i / iconCells)
		col := float64(i % iconCells)
		fillPaths = append(fillPaths, fillPath{
			colorRole: cellValue,
			ops: []pathOp{
				{kind: pathOpCircle, x0: circleOffset + col*scale, y0: circleOffset + row*scale, x1: circleRadius},
			},
		})
		// connection south west
		if 0 < i%iconCells && i+iconCells-1 < len(cellValues) && cellValues[i+iconCells-1] == cellValue {
			fillPaths = append(fillPaths, fillPath{
				colorRole: cellValue,
				ops:       connectionOps(row, col, scale, -1),
			})
		}
		// connection south east
		if i%iconCells < iconCells-1 && i+iconCells+1 < len(cellValues) && cellValues[i+iconCells+1] == cellValue {
			fillPaths = append(fillPaths, fillPath{
				colorRole: cellValue,
				ops:       connectionOps(row, col, scale, +1),
			})
		}
	}
	return fillPaths
}

// renderSizeForSize rounds the target size to the nearest natural render size, a
// positive multiple of renderSizeUnit px. Ties round up.
func renderSizeForSize(size int) int {
	renderSize := renderSizeUnit * int(math.Round(float64(size)/renderSizeUnit))
	if renderSize < renderSizeUnit {
		renderSize = renderSizeUnit
	}
	return renderSize
}

// rasterize renders the cells at renderSize x renderSize px: the background fill, then
// each fill path anti aliased over it.
func rasterize(cellValues []int, combination [3]hslColor, renderSize int) *image.RGBA {
	renderImage := image.NewRGBA(image.Rect(0, 0, renderSize, renderSize))
	draw.Draw(renderImage, renderImage.Bounds(), image.NewUniform(combination[0].rgba()), image.Point{}, draw.Src)

	fgUniform := image.NewUniform(combination[1].rgba())
	spotUniform := image.NewUniform(combination[2].rgba())

	rasterizer := vector.NewRasterizer(renderSize, renderSize)
	// circle path approximated by four cubic beziers with the standard kappa control offset
	fillCircle := func(cx float64, cy float64, r float64) {
		const kappa = 0.5522847498307936
		k := r * kappa
		rasterizer.MoveTo(float32(cx+r), float32(cy))
		rasterizer.CubeTo(float32(cx+r), float32(cy+k), float32(cx+k), float32(cy+r), float32(cx), float32(cy+r))
		rasterizer.CubeTo(float32(cx-k), float32(cy+r), float32(cx-r), float32(cy+k), float32(cx-r), float32(cy))
		rasterizer.CubeTo(float32(cx-r), float32(cy-k), float32(cx-k), float32(cy-r), float32(cx), float32(cy-r))
		rasterizer.CubeTo(float32(cx+k), float32(cy-r), float32(cx+r), float32(cy-k), float32(cx+r), float32(cy))
	}

	scale := float64(renderSize) / iconCells
	for _, fp := range createFillPaths(cellValues, scale) {
		rasterizer.Reset(renderSize, renderSize)
		for _, op := range fp.ops {
			switch op.kind {
			case pathOpCircle:
				fillCircle(op.x0, op.y0, op.x1)
			case pathOpMoveTo:
				rasterizer.MoveTo(float32(op.x0), float32(op.y0))
			case pathOpLineTo:
				rasterizer.LineTo(float32(op.x0), float32(op.y0))
			case pathOpCubeTo:
				rasterizer.CubeTo(float32(op.x0), float32(op.y0), float32(op.x1), float32(op.y1), float32(op.x2), float32(op.y2))
			}
		}
		rasterizer.ClosePath()
		fillUniform := spotUniform
		if fp.colorRole == 1 {
			fillUniform = fgUniform
		}
		rasterizer.Draw(renderImage, renderImage.Bounds(), fillUniform, image.Point{})
	}
	return renderImage
}

// renderImageWithCombinations renders the identicon for data against the given color
// combination table.
func renderImageWithCombinations(data []byte, size int, combinations [][3]hslColor) (*image.RGBA, error) {
	if size < 1 {
		return nil, fmt.Errorf("size must be positive: %d", size)
	}
	iconRand := newIconRand(seedForData(data))
	cellValues := createCellValues(iconRand)
	combination := combinations[combinationIndexForRand(iconRand, combinations)]

	renderSize := renderSizeForSize(size)
	renderImage := rasterize(cellValues, combination, renderSize)
	if renderSize == size {
		return renderImage, nil
	}
	// resample the natural render to fit the target square
	targetImage := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(targetImage, targetImage.Bounds(), renderImage, renderImage.Bounds(), draw.Src, nil)
	return targetImage, nil
}

// renderPngWithCombinations renders the identicon for data as a png against the given
// color combination table.
func renderPngWithCombinations(data []byte, size int, combinations [][3]hslColor) ([]byte, error) {
	renderImage, err := renderImageWithCombinations(data, size, combinations)
	if err != nil {
		return nil, err
	}
	pngBuffer := &bytes.Buffer{}
	if err := png.Encode(pngBuffer, renderImage); err != nil {
		return nil, err
	}
	return pngBuffer.Bytes(), nil
}

// RenderImage renders the v1 scheme identicon for data as an opaque size x size image.
// The icon is rendered at the nearest multiple of 32 px and resampled to fit the target
// size exactly.
func RenderImage(data []byte, size int) (*image.RGBA, error) {
	return renderImageWithCombinations(data, size, colorCombinations)
}

// RenderPng renders the v1 scheme identicon for data as an opaque size x size png.
func RenderPng(data []byte, size int) ([]byte, error) {
	return renderPngWithCombinations(data, size, colorCombinations)
}

// RenderImageV2 renders the v2 scheme identicon for data as an opaque size x size
// image: the same pattern and geometry as v1, colored with the URnetwork brand
// palette (colorCombinationsV2).
func RenderImageV2(data []byte, size int) (*image.RGBA, error) {
	return renderImageWithCombinations(data, size, colorCombinationsV2)
}

// RenderPngV2 renders the v2 scheme identicon for data as an opaque size x size png.
func RenderPngV2(data []byte, size int) ([]byte, error) {
	return renderPngWithCombinations(data, size, colorCombinationsV2)
}
