goidenticons
========

A go port of [Nanoidenticons](https://github.com/keerifox/Nanoidenticons) (itself based on
[Blockies](https://github.com/download13/blockies)) that generates identicon png images
for arbitrary `[]byte` input.

The pattern, colors, and geometry match the js library exactly for the same seed string
(verified against recordings of the js render, see `testdata/fixtures.json`). The input
bytes are reduced to the seed internally: the seed is the hex sha256 digest of the input.

An icon is 8x8 cells, so the natural render sizes are multiples of 32px. A requested
target size is rounded to the nearest multiple of 32, rendered, then resampled to fit the
target square exactly.

Library
---

```go
import "github.com/urnetwork/goidenticons"

// a 128x128 opaque png
pngBytes, err := goidenticons.RenderPng(clientId.Bytes(), 128)

// or an image.Image
img, err := goidenticons.RenderImage(clientId.Bytes(), 128)
```

identiconctl
---

```
go run ./identiconctl generate "some input string" --size=128 --out=identicon.png
```

Generated files
---

`colors.go` and `testdata/fixtures.json` are generated from a Nanoidenticons checkout by
the scripts in `gen/` (see the script headers).

License
---

Ported from Nanoidenticons ([WTFPL](http://www.wtfpl.net/)).

Compatibility contract
---

The rendered output is FROZEN: for a given input and size, the pattern,
palette, and geometry must never change. Users visually verify identity keys
by comparing identicons across devices, platforms, and app versions — any
change to the algorithm breaks human verification. `testdata/fixtures.json`
is the contract; a change that alters any fixture must ship as a new,
explicitly versioned scheme selected by callers, never as a change to the
default output.
