// Tests verify parity with the Nanoidenticons js library using fixtures recorded from
// the real js render through an instrumented canvas (testdata/fixtures.json), plus the
// rendered pixel colors, size rounding, and png encoding behavior.
package goidenticons

import (
	"bytes"
	"encoding/json"
	"image/png"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

// fixtureOp is one recorded canvas path op. arc: x, y, r; moveTo and lineTo: x, y;
// cubeTo: c1x, c1y, c2x, c2y with end point x, y.
type fixtureOp struct {
	Op  string  `json:"op"`
	X   float64 `json:"x"`
	Y   float64 `json:"y"`
	R   float64 `json:"r"`
	C1x float64 `json:"c1x"`
	C1y float64 `json:"c1y"`
	C2x float64 `json:"c2x"`
	C2y float64 `json:"c2y"`
}

// fixtureFill is one recorded canvas fill call with its active fill style.
type fixtureFill struct {
	Style string      `json:"style"`
	Path  []fixtureOp `json:"path"`
}

// fixtureBg is the recorded background fillRect.
type fixtureBg struct {
	Style string  `json:"style"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	W     float64 `json:"w"`
	H     float64 `json:"h"`
}

// fixtureRender is the full recording of one js renderIcon call at scale 4 (32 px).
type fixtureRender struct {
	Width  int           `json:"width"`
	Height int           `json:"height"`
	Bg     fixtureBg     `json:"bg"`
	Fills  []fixtureFill `json:"fills"`
}

// fixture pairs an input string with the js render of seed sha256hex(input).
type fixture struct {
	Input               string        `json:"input"`
	Seed                string        `json:"seed"`
	PredictedCells      []int         `json:"predictedCells"`
	PredictedColorIndex int           `json:"predictedColorIndex"`
	Actual              fixtureRender `json:"actual"`
}

// loadFixtures reads testdata/fixtures.json.
func loadFixtures(t *testing.T) []fixture {
	fixturesJson, err := os.ReadFile("testdata/fixtures.json")
	if err != nil {
		t.Fatalf("read fixtures: %s", err)
	}
	fixtures := []fixture{}
	if err := json.Unmarshal(fixturesJson, &fixtures); err != nil {
		t.Fatalf("parse fixtures: %s", err)
	}
	if len(fixtures) == 0 {
		t.Fatalf("no fixtures")
	}
	return fixtures
}

// parseHslStyle parses a js "hsl(H,S%,L%)" style string.
func parseHslStyle(t *testing.T, style string) hslColor {
	inner := strings.TrimSuffix(strings.TrimPrefix(style, "hsl("), ")")
	parts := strings.Split(inner, ",")
	if len(parts) != 3 {
		t.Fatalf("bad hsl style: %s", style)
	}
	values := [3]float64{}
	for i, part := range parts {
		value, err := strconv.ParseFloat(strings.TrimSuffix(part, "%"), 64)
		if err != nil {
			t.Fatalf("bad hsl style %s: %s", style, err)
		}
		values[i] = value
	}
	return hslColor{h: values[0], s: values[1], l: values[2]}
}

// TestJsParity checks the full pipeline against the js recordings: seed derivation,
// cell values, color combination choice, and the exact fill geometry and paint order.
func TestJsParity(t *testing.T) {
	const epsilon = 1e-9
	for _, f := range loadFixtures(t) {
		seed := seedForData([]byte(f.Input))
		if seed != f.Seed {
			t.Fatalf("%s: seed %s != %s", f.Input, seed, f.Seed)
		}

		iconRand := newIconRand(seed)
		cellValues := createCellValues(iconRand)
		if len(cellValues) != len(f.PredictedCells) {
			t.Fatalf("%s: cell count %d != %d", f.Input, len(cellValues), len(f.PredictedCells))
		}
		for i, cellValue := range cellValues {
			if cellValue != f.PredictedCells[i] {
				t.Fatalf("%s: cell %d value %d != %d", f.Input, i, cellValue, f.PredictedCells[i])
			}
		}

		combinationIndex := combinationIndexForRand(iconRand, colorCombinations)
		if combinationIndex != f.PredictedColorIndex {
			t.Fatalf("%s: combination index %d != %d", f.Input, combinationIndex, f.PredictedColorIndex)
		}
		combination := colorCombinations[combinationIndex]

		// background: same color, full canvas
		bg := parseHslStyle(t, f.Actual.Bg.Style)
		if bg != combination[0] {
			t.Fatalf("%s: bg %+v != %+v", f.Input, bg, combination[0])
		}
		if f.Actual.Width != 32 || f.Actual.Height != 32 || f.Actual.Bg.W != 32 || f.Actual.Bg.H != 32 {
			t.Fatalf("%s: unexpected fixture canvas geometry", f.Input)
		}

		// fills: same count, same order, same colors, same geometry
		fillPaths := createFillPaths(cellValues, 4)
		if len(fillPaths) != len(f.Actual.Fills) {
			t.Fatalf("%s: fill count %d != %d", f.Input, len(fillPaths), len(f.Actual.Fills))
		}
		for fillIndex, fp := range fillPaths {
			actualFill := f.Actual.Fills[fillIndex]
			expectedColor := combination[2]
			if fp.colorRole == 1 {
				expectedColor = combination[1]
			}
			actualColor := parseHslStyle(t, actualFill.Style)
			if actualColor != expectedColor {
				t.Fatalf("%s: fill %d color %+v != %+v", f.Input, fillIndex, actualColor, expectedColor)
			}
			if len(fp.ops) != len(actualFill.Path) {
				t.Fatalf("%s: fill %d op count %d != %d", f.Input, fillIndex, len(fp.ops), len(actualFill.Path))
			}
			for opIndex, op := range fp.ops {
				actualOp := actualFill.Path[opIndex]
				coords := map[string][2]float64{}
				switch op.kind {
				case pathOpCircle:
					if actualOp.Op != "arc" {
						t.Fatalf("%s: fill %d op %d kind circle != %s", f.Input, fillIndex, opIndex, actualOp.Op)
					}
					coords["center"] = [2]float64{op.x0 - actualOp.X, op.y0 - actualOp.Y}
					coords["radius"] = [2]float64{op.x1 - actualOp.R, 0}
				case pathOpMoveTo:
					if actualOp.Op != "moveTo" {
						t.Fatalf("%s: fill %d op %d kind moveTo != %s", f.Input, fillIndex, opIndex, actualOp.Op)
					}
					coords["point"] = [2]float64{op.x0 - actualOp.X, op.y0 - actualOp.Y}
				case pathOpLineTo:
					if actualOp.Op != "lineTo" {
						t.Fatalf("%s: fill %d op %d kind lineTo != %s", f.Input, fillIndex, opIndex, actualOp.Op)
					}
					coords["point"] = [2]float64{op.x0 - actualOp.X, op.y0 - actualOp.Y}
				case pathOpCubeTo:
					if actualOp.Op != "cubeTo" {
						t.Fatalf("%s: fill %d op %d kind cubeTo != %s", f.Input, fillIndex, opIndex, actualOp.Op)
					}
					coords["control1"] = [2]float64{op.x0 - actualOp.C1x, op.y0 - actualOp.C1y}
					coords["control2"] = [2]float64{op.x1 - actualOp.C2x, op.y1 - actualOp.C2y}
					coords["end"] = [2]float64{op.x2 - actualOp.X, op.y2 - actualOp.Y}
				}
				for name, delta := range coords {
					if epsilon < math.Abs(delta[0]) || epsilon < math.Abs(delta[1]) {
						t.Fatalf(
							"%s: fill %d op %d %s off by (%v, %v)",
							f.Input, fillIndex, opIndex, name, delta[0], delta[1],
						)
					}
				}
			}
		}
	}
}

// TestRenderPixels renders each fixture at the natural 32 px size and checks exact pixel
// colors: every non background cell center pixel has its role color, isolated background
// cell centers have the background color, and every pixel is fully opaque.
func TestRenderPixels(t *testing.T) {
	isolatedBgCount := 0
	for _, f := range loadFixtures(t) {
		renderImage, err := RenderImage([]byte(f.Input), 32)
		if err != nil {
			t.Fatalf("%s: %s", f.Input, err)
		}
		if renderImage.Bounds().Dx() != 32 || renderImage.Bounds().Dy() != 32 {
			t.Fatalf("%s: bounds %v", f.Input, renderImage.Bounds())
		}
		for y := 0; y < 32; y++ {
			for x := 0; x < 32; x++ {
				if a := renderImage.RGBAAt(x, y).A; a != 255 {
					t.Fatalf("%s: pixel (%d, %d) alpha %d != 255", f.Input, x, y, a)
				}
			}
		}

		combination := colorCombinations[f.PredictedColorIndex]
		bgRgba := combination[0].rgba()
		for i, cellValue := range f.PredictedCells {
			row := i / iconCells
			col := i % iconCells
			// the pixel at the cell center is fully covered by the cell circle, and no
			// different colored shape reaches it (connections require equal values)
			px := 2 + 4*col
			py := 2 + 4*row
			switch cellValue {
			case 0:
				// only assert background cells that no neighboring shape can reach
				isolated := true
				for _, otherIndex := range []int{i - iconCells - 1, i - iconCells, i - iconCells + 1, i - 1, i + 1, i + iconCells - 1, i + iconCells, i + iconCells + 1} {
					sameNeighborhood := 0 <= otherIndex && otherIndex < len(f.PredictedCells) &&
						math.Abs(float64(otherIndex%iconCells-col)) <= 1
					if sameNeighborhood && f.PredictedCells[otherIndex] != 0 {
						isolated = false
						break
					}
				}
				if isolated {
					isolatedBgCount++
					if rgba := renderImage.RGBAAt(px, py); rgba != bgRgba {
						t.Fatalf("%s: bg cell %d pixel %v != %v", f.Input, i, rgba, bgRgba)
					}
				}
			case 1:
				if rgba := renderImage.RGBAAt(px, py); rgba != combination[1].rgba() {
					t.Fatalf("%s: fg cell %d pixel %v != %v", f.Input, i, rgba, combination[1].rgba())
				}
			default:
				if rgba := renderImage.RGBAAt(px, py); rgba != combination[2].rgba() {
					t.Fatalf("%s: spot cell %d pixel %v != %v", f.Input, i, rgba, combination[2].rgba())
				}
			}
		}
	}
	if isolatedBgCount == 0 {
		t.Fatalf("no isolated background cells across fixtures; background color untested")
	}
}

// TestRenderSizeRounding checks the render size rounding rule: nearest positive multiple
// of 32 with ties rounding up, while the output always has the exact requested size.
func TestRenderSizeRounding(t *testing.T) {
	cases := []struct {
		size       int
		renderSize int
	}{
		{size: 1, renderSize: 32},
		{size: 16, renderSize: 32},
		{size: 31, renderSize: 32},
		{size: 32, renderSize: 32},
		{size: 47, renderSize: 32},
		{size: 48, renderSize: 64},
		{size: 100, renderSize: 96},
		{size: 112, renderSize: 128},
		{size: 128, renderSize: 128},
		{size: 129, renderSize: 128},
		{size: 500, renderSize: 512},
	}
	for _, c := range cases {
		if renderSize := renderSizeForSize(c.size); renderSize != c.renderSize {
			t.Errorf("size %d: render size %d != %d", c.size, renderSize, c.renderSize)
		}
		renderImage, err := RenderImage([]byte("size test"), c.size)
		if err != nil {
			t.Fatalf("size %d: %s", c.size, err)
		}
		if renderImage.Bounds().Dx() != c.size || renderImage.Bounds().Dy() != c.size {
			t.Errorf("size %d: bounds %v", c.size, renderImage.Bounds())
		}
	}

	for _, size := range []int{0, -1} {
		if _, err := RenderImage([]byte("size test"), size); err == nil {
			t.Errorf("size %d: expected error", size)
		}
		if _, err := RenderPng([]byte("size test"), size); err == nil {
			t.Errorf("size %d: expected png error", size)
		}
	}
}

// TestRenderPng checks that the png output decodes to the requested size, is fully
// opaque after resampling, and is deterministic per input and distinct across inputs.
func TestRenderPng(t *testing.T) {
	pngBytes, err := RenderPng([]byte("urnetwork"), 100)
	if err != nil {
		t.Fatalf("%s", err)
	}
	decoded, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("decode: %s", err)
	}
	if decoded.Bounds().Dx() != 100 || decoded.Bounds().Dy() != 100 {
		t.Fatalf("bounds %v", decoded.Bounds())
	}
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if _, _, _, a := decoded.At(x, y).RGBA(); a != 0xffff {
				t.Fatalf("pixel (%d, %d) alpha %d != opaque", x, y, a)
			}
		}
	}

	pngBytes2, err := RenderPng([]byte("urnetwork"), 100)
	if err != nil {
		t.Fatalf("%s", err)
	}
	if !bytes.Equal(pngBytes, pngBytes2) {
		t.Fatalf("render not deterministic")
	}

	otherPngBytes, err := RenderPng([]byte("hello world"), 100)
	if err != nil {
		t.Fatalf("%s", err)
	}
	if bytes.Equal(pngBytes, otherPngBytes) {
		t.Fatalf("distinct inputs rendered identically")
	}
}
