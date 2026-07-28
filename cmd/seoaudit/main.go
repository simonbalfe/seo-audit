package main

import (
	"context"
	"fmt"
	"os"

	"github.com/simonbalfe/seo-audit/internal/cli"
)

func main() {
	if err := cli.Execute(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
