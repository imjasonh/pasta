// Command pastals is the pasta language server. It speaks LSP over
// stdio: spawn it from any LSP-capable editor (VS Code, Neovim, Helix)
// and it will publish pasta diagnostics for files matching one of the
// registered languages.
//
// All the logic lives in internal/pastals; this binary is just an entry
// point that wires the server up to stdin/stdout.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/imjasonh/pasta/internal/pastals"
)

func main() {
	srv := pastals.New(os.Stdin, os.Stdout)
	if err := srv.Run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "pastals:", err)
		os.Exit(1)
	}
}
