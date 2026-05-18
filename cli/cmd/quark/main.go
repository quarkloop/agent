package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/quarkloop/cli/pkg"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil).WithAttrs([]slog.Attr{
		slog.String("process", "cli"),
	})))

	root := pkg.Root()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
