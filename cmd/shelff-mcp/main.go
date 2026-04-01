package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/skoji/shelff-mcp/internal/mcpserver"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string) error {
	root, err := resolveRoot(args, getenv)
	if err != nil {
		return err
	}

	server, err := mcpserver.New(root)
	if err != nil {
		return err
	}
	return server.Run(ctx)
}

func resolveRoot(args []string, getenv func(string) string) (string, error) {
	fs := flag.NewFlagSet("shelff-mcp", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var root string
	fs.StringVar(&root, "root", "", "path to the shelff library root")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if len(fs.Args()) > 0 {
		return "", fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if root == "" {
		root = getenv("SHELFF_ROOT")
	}
	if root == "" {
		return "", mcpserver.ErrRootNotProvided
	}
	return root, nil
}
