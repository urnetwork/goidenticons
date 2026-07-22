// identiconctl generates identicon png images from an input string.
package main

import (
	"fmt"
	"os"

	"github.com/docopt/docopt-go"

	"github.com/urnetwork/goidenticons/v2026"
)

const IdenticonCtlVersion = "0.0.1"

func main() {
	usage := `Identicon control.

Generates an identicon png image from an input string. The icon is rendered at the
nearest multiple of 32px and scaled to fit size x size.

Usage:
    identiconctl generate <input> [--size=<size>] [--out=<out>]

Options:
    -h --help        Show this screen.
    --version        Show version.
    --size=<size>    Target square size in pixels [default: 128].
    --out=<out>      Output png file path [default: identicon.png].`

	opts, err := docopt.ParseArgs(usage, os.Args[1:], IdenticonCtlVersion)
	if err != nil {
		panic(err)
	}

	if generate_, _ := opts.Bool("generate"); generate_ {
		generate(opts)
	}
}

// generate renders the identicon for <input> and writes the png file.
func generate(opts docopt.Opts) {
	input, _ := opts.String("<input>")
	size, err := opts.Int("--size")
	if err != nil {
		panic(err)
	}
	outPath, _ := opts.String("--out")

	pngBytes, err := goidenticons.RenderPng([]byte(input), size)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(outPath, pngBytes, 0644); err != nil {
		panic(err)
	}
	fmt.Printf("%s\n", outPath)
}
