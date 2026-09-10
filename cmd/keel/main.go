// keel — 给 vibe coding 装一根龙骨。
package main

import (
	"os"

	"github.com/shiftu/keel/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr, os.Stdin))
}
