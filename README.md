goidenticons
========

A go port of [Nanoidenticons](https://github.com/keerifox/Nanoidenticons) (itself based on
[Blockies](https://github.com/download13/blockies)) that generates identicon png images
for arbitrary `[]byte` input.

The pattern and geometry match the js library exactly for the same seed string
(verified against recordings of the js render, see `testdata/fixtures.json`). The input
bytes are reduced to the seed internally: the seed is the hex sha256 digest of the input.

Two frozen color schemes share the engine; for a given input both produce the same cell
pattern:

- v1 (`RenderImage`, `RenderPng`): the js library's palette, an exact color match of the
  js render.
- v2 (`RenderImageV2`, `RenderPngV2`): the URnetwork brand palette. Backgrounds are the
  two product surfaces (Black, Blue900) and foreground/spot pairs are drawn from the
  flagship brand accents, contrast filtered (see `gen/gen_colors_v2.mjs`).

An icon is 8x8 cells, so the natural render sizes are multiples of 32px. A requested
target size is rounded to the nearest multiple of 32, rendered, then resampled to fit the
target square exactly.

Library
---

```go
import "github.com/urnetwork/goidenticons"

// a 128x128 opaque png, URnetwork brand scheme
pngBytes, err := goidenticons.RenderPngV2(clientId.Bytes(), 128)

// or an image.Image
img, err := goidenticons.RenderImageV2(clientId.Bytes(), 128)

// the original js palette (v1)
pngBytes, err = goidenticons.RenderPng(clientId.Bytes(), 128)
```

identiconctl
---

```
go run ./identiconctl generate "some input string" --size=128 --scheme=2 --out=identicon.png
```

Generated files
---

`colors.go` and `testdata/fixtures.json` are generated from a Nanoidenticons checkout by
the scripts in `gen/` (see the script headers). `colors_v2.go` is generated from the
URnetwork brand palette by `gen/gen_colors_v2.mjs`; the brand source of truth is the
android app `ui/theme/Color.kt`.

License
---

Ported from Nanoidenticons ([WTFPL](http://www.wtfpl.net/)).

Compatibility contract
---

The rendered output of every shipped scheme is FROZEN: for a given input and
size, the pattern, palette, and geometry must never change. Users visually
verify identity keys by comparing identicons across devices, platforms, and
app versions — any change to the algorithm breaks human verification.
`testdata/fixtures.json` is the v1 contract and `testdata/v2_golden` is the
v2 contract; a change that alters any fixture or golden must ship as a new,
explicitly versioned scheme selected by callers, never as a change to an
existing scheme's output.
