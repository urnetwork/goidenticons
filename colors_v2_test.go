// Tests for the v2 scheme: colorCombinationsV2 invariants against the URnetwork
// brand hex values, and golden renders (testdata/v2_golden) that freeze the v2
// output per the compatibility contract in the README. Regenerate goldens with
// `go test -run TestV2Golden -update` only when deliberately shipping a new scheme
// change (which per the contract should instead be a new scheme).
package goidenticons

import (
	"bytes"
	"flag"
	"fmt"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

var updateV2Golden = flag.Bool("update", false, "rewrite testdata/v2_golden from the current render output")

// the brand hex values allowed in the v2 table, from android ui/theme/Color.kt
var (
	v2BackgroundHexes = map[string]string{
		"101010": "Black",
		"1A1460": "Blue900",
	}
	v2AccentHexes = map[string]string{
		"ED8FFF": "Pink300", // --brand-ur
		"DC40F5": "Pink500",
		"87FB67": "Green300", // --brand-usd
		"31D70B": "Green500",
		"D6E6F4": "Blue200", // --brand-ui
		"638BFC": "Blue400",
		"2A60FF": "Blue500",
		"FF6C58": "Red400", // --brand-burn
		"EFF7BB": "Yellow200",
		"E6EA23": "Yellow400",
		"F8F8F8": "OffWhite", // --brand-white
	}
)

func hexForHsl(c hslColor) string {
	rgba := c.rgba()
	return fmt.Sprintf("%02X%02X%02X", rgba.R, rgba.G, rgba.B)
}

// TestV2Combinations checks that every v2 combination is exact brand colors in the
// intended roles: a product surface background and two distinct brand accents.
func TestV2Combinations(t *testing.T) {
	if len(colorCombinationsV2) != 140 {
		t.Fatalf("v2 table length %d != 140", len(colorCombinationsV2))
	}
	for i, combination := range colorCombinationsV2 {
		bg := hexForHsl(combination[0])
		fg := hexForHsl(combination[1])
		spot := hexForHsl(combination[2])
		if _, ok := v2BackgroundHexes[bg]; !ok {
			t.Errorf("combination %d: background %s is not a v2 surface", i, bg)
		}
		if _, ok := v2AccentHexes[fg]; !ok {
			t.Errorf("combination %d: foreground %s is not a v2 accent", i, fg)
		}
		if _, ok := v2AccentHexes[spot]; !ok {
			t.Errorf("combination %d: spot %s is not a v2 accent", i, spot)
		}
		if fg == spot || fg == bg || spot == bg {
			t.Errorf("combination %d: roles not distinct: %s/%s/%s", i, bg, fg, spot)
		}
	}
}

// v2GoldenRenders lists the frozen golden renders: the fixture inputs at the natural
// 32 px size, plus one resampled size to freeze the resample path.
func v2GoldenRenders(t *testing.T) map[string]struct {
	input string
	size  int
} {
	renders := map[string]struct {
		input string
		size  int
	}{}
	for i, f := range loadFixtures(t) {
		renders[fmt.Sprintf("fixture_%02d_32.png", i)] = struct {
			input string
			size  int
		}{input: f.Input, size: 32}
	}
	renders["urnetwork_100.png"] = struct {
		input string
		size  int
	}{input: "urnetwork", size: 100}
	return renders
}

// TestV2Golden compares the v2 renders against testdata/v2_golden pixel for pixel.
// These goldens freeze the v2 output the way testdata/fixtures.json freezes v1.
func TestV2Golden(t *testing.T) {
	goldenDir := filepath.Join("testdata", "v2_golden")
	if *updateV2Golden {
		if err := os.MkdirAll(goldenDir, 0o755); err != nil {
			t.Fatalf("create %s: %s", goldenDir, err)
		}
	}
	for name, render := range v2GoldenRenders(t) {
		goldenPath := filepath.Join(goldenDir, name)
		renderImage, err := RenderImageV2([]byte(render.input), render.size)
		if err != nil {
			t.Fatalf("%s: %s", name, err)
		}
		if *updateV2Golden {
			pngBuffer := &bytes.Buffer{}
			if err := png.Encode(pngBuffer, renderImage); err != nil {
				t.Fatalf("%s: encode: %s", name, err)
			}
			if err := os.WriteFile(goldenPath, pngBuffer.Bytes(), 0o644); err != nil {
				t.Fatalf("%s: write: %s", name, err)
			}
			continue
		}
		goldenBytes, err := os.ReadFile(goldenPath)
		if err != nil {
			t.Fatalf("%s: read golden (regenerate with -update): %s", name, err)
		}
		goldenImage, err := png.Decode(bytes.NewReader(goldenBytes))
		if err != nil {
			t.Fatalf("%s: decode golden: %s", name, err)
		}
		if goldenImage.Bounds() != renderImage.Bounds() {
			t.Fatalf("%s: bounds %v != %v", name, renderImage.Bounds(), goldenImage.Bounds())
		}
		for y := 0; y < renderImage.Bounds().Dy(); y++ {
			for x := 0; x < renderImage.Bounds().Dx(); x++ {
				golden := color.RGBAModel.Convert(goldenImage.At(x, y)).(color.RGBA)
				if rgba := renderImage.RGBAAt(x, y); rgba != golden {
					t.Fatalf("%s: pixel (%d, %d) %v != golden %v", name, x, y, rgba, golden)
				}
			}
		}
	}
}

// TestRenderPngV2 checks the v2 png output basics: exact requested size, opaque,
// deterministic per input, distinct across inputs, distinct from the v1 scheme, and
// erroring on non positive sizes.
func TestRenderPngV2(t *testing.T) {
	pngBytes, err := RenderPngV2([]byte("urnetwork"), 100)
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

	pngBytes2, err := RenderPngV2([]byte("urnetwork"), 100)
	if err != nil {
		t.Fatalf("%s", err)
	}
	if !bytes.Equal(pngBytes, pngBytes2) {
		t.Fatalf("v2 render not deterministic")
	}

	otherPngBytes, err := RenderPngV2([]byte("hello world"), 100)
	if err != nil {
		t.Fatalf("%s", err)
	}
	if bytes.Equal(pngBytes, otherPngBytes) {
		t.Fatalf("distinct inputs rendered identically")
	}

	v1PngBytes, err := RenderPng([]byte("urnetwork"), 100)
	if err != nil {
		t.Fatalf("%s", err)
	}
	if bytes.Equal(pngBytes, v1PngBytes) {
		t.Fatalf("v2 render identical to v1; scheme not selected")
	}

	for _, size := range []int{0, -1} {
		if _, err := RenderPngV2([]byte("size test"), size); err == nil {
			t.Errorf("size %d: expected error", size)
		}
		if _, err := RenderImageV2([]byte("size test"), size); err == nil {
			t.Errorf("size %d: expected image error", size)
		}
	}
}
