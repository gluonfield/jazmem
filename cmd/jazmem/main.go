package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/wins/jazmem/pkg/jazmem"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		printError(err)
		os.Exit(1)
	}
}

func printError(err error) {
	var notFound *jazmem.NotFoundError
	if errors.As(err, &notFound) {
		fmt.Fprintf(os.Stderr, "jazmem: not found: %s\n", notFound.Slug)
		if len(notFound.Suggestions) > 0 {
			fmt.Fprintln(os.Stderr, "suggestions:")
			for _, suggestion := range notFound.Suggestions {
				if suggestion.Title == "" {
					fmt.Fprintf(os.Stderr, "- %s\n", suggestion.Slug)
					continue
				}
				fmt.Fprintf(os.Stderr, "- %s (%s)\n", suggestion.Slug, suggestion.Title)
			}
		}
		return
	}
	fmt.Fprintln(os.Stderr, "jazmem:", err)
}

func run(args []string) error {
	if len(args) == 0 {
		usage(os.Stderr)
		return errors.New("command is required")
	}
	switch args[0] {
	case "init":
		return runInit(args[1:])
	case "index":
		return runIndex(args[1:])
	case "search":
		return runSearch(args[1:])
	case "get", "page":
		return runGet(args[1:])
	case "file":
		return runFile(args[1:])
	case "checkpoint":
		return runCheckpoint(args[1:])
	case "dream":
		return runDream(args[1:])
	case "link-hygiene":
		return runLinkHygiene(args[1:])
	case "doctor":
		return runDoctor(args[1:])
	default:
		return runSearch(args)
	}
}

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	root := fs.String("root", "", rootHelp)
	path := fs.String("path", "", "alias for --root")
	db := fs.String("db", "", dbHelp)
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) > 1 {
		return errors.New("init accepts at most one positional root path")
	}
	selectedRoot, err := resolveRootArg(*root, *path, rest)
	if err != nil {
		return err
	}
	report, err := jazmem.Init(context.Background(), jazmem.Config{Root: selectedRoot, DBPath: *db})
	if err != nil {
		return err
	}
	return printJSON(report)
}

func runIndex(args []string) error {
	cfg, _, err := parseCommon("index", args)
	if err != nil {
		return err
	}
	m, err := jazmem.Open(cfg)
	if err != nil {
		return err
	}
	defer m.Close()
	report, err := m.Reindex(context.Background(), jazmem.ReindexOptions{})
	if err != nil {
		return err
	}
	return printJSON(report)
}

func runSearch(args []string) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	root := fs.String("root", "", rootHelp)
	path := fs.String("path", "", "alias for --root")
	db := fs.String("db", "", dbHelp)
	limit := fs.Int("limit", 10, "maximum results")
	text := fs.Bool("text", false, "print assembled context text instead of JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	selectedRoot, err := resolveRootArg(*root, *path, nil)
	if err != nil {
		return err
	}
	cfg := jazmem.Config{Root: selectedRoot, DBPath: *db}
	query := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if query == "" {
		return errors.New("search query is required")
	}
	m, err := jazmem.Open(cfg)
	if err != nil {
		return err
	}
	defer m.Close()
	result, err := m.Retrieve(context.Background(), query, jazmem.SearchOptions{Limit: *limit})
	if err != nil {
		return err
	}
	if *text {
		fmt.Print(result.Context)
		return nil
	}
	return printJSON(result)
}

func runGet(args []string) error {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	root := fs.String("root", "", rootHelp)
	path := fs.String("path", "", "alias for --root")
	db := fs.String("db", "", dbHelp)
	raw := fs.Bool("raw", false, "print raw markdown instead of JSON")
	body := fs.Bool("body", false, "print markdown body without frontmatter instead of JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	selectedRoot, err := resolveRootArg(*root, *path, nil)
	if err != nil {
		return err
	}
	cfg := jazmem.Config{Root: selectedRoot, DBPath: *db}
	if len(rest) != 1 {
		return errors.New("get requires exactly one slug")
	}
	m, err := jazmem.Open(cfg)
	if err != nil {
		return err
	}
	defer m.Close()
	page, err := m.GetPage(context.Background(), rest[0])
	if err != nil {
		return err
	}
	if *raw {
		fmt.Print(page.Raw)
		return nil
	}
	if *body {
		fmt.Print(page.Body)
		return nil
	}
	return printJSON(page)
}

func runFile(args []string) error {
	cfg, rest, err := parseCommon("file", args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return errors.New("file requires exactly one slug")
	}
	m, err := jazmem.Open(cfg)
	if err != nil {
		return err
	}
	defer m.Close()
	page, err := m.GetPage(context.Background(), rest[0])
	if err != nil {
		return err
	}
	fmt.Println(page.Path)
	return nil
}

func runCheckpoint(args []string) error {
	cfg, rest, err := parseCommon("checkpoint", args)
	if err != nil {
		return err
	}
	m, err := jazmem.Open(cfg)
	if err != nil {
		return err
	}
	defer m.Close()
	report, err := m.Checkpoint(context.Background(), strings.Join(rest, " "))
	if err != nil {
		return err
	}
	return printJSON(report)
}

func runDream(args []string) error {
	cfg, _, err := parseCommon("dream", args)
	if err != nil {
		return err
	}
	m, err := jazmem.Open(cfg)
	if err != nil {
		return err
	}
	defer m.Close()
	report, err := m.Dream(context.Background(), jazmem.DreamOptions{})
	if err != nil {
		return err
	}
	return printJSON(report)
}

func runDoctor(args []string) error {
	cfg, _, err := parseCommon("doctor", args)
	if err != nil {
		return err
	}
	m, err := jazmem.Open(cfg)
	if err != nil {
		return err
	}
	defer m.Close()
	report, err := m.Doctor(context.Background())
	if err != nil {
		return err
	}
	return printJSON(report)
}

func runLinkHygiene(args []string) error {
	cfg, _, err := parseCommon("link-hygiene", args)
	if err != nil {
		return err
	}
	m, err := jazmem.Open(cfg)
	if err != nil {
		return err
	}
	defer m.Close()
	report, err := m.LinkHygiene(context.Background())
	if err != nil {
		return err
	}
	return printJSON(report)
}

func parseCommon(name string, args []string) (jazmem.Config, []string, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	root := fs.String("root", "", rootHelp)
	path := fs.String("path", "", "alias for --root")
	db := fs.String("db", "", dbHelp)
	if err := fs.Parse(args); err != nil {
		return jazmem.Config{}, nil, err
	}
	selectedRoot, err := resolveRootArg(*root, *path, nil)
	if err != nil {
		return jazmem.Config{}, nil, err
	}
	return jazmem.Config{Root: selectedRoot, DBPath: *db}, fs.Args(), nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: jazmem [--root path] [--db path] <query>")
	fmt.Fprintln(w, "       jazmem init [--root path|--path path|path] [--db path]")
	fmt.Fprintln(w, "       jazmem <index|search|get|page|file|checkpoint|dream|link-hygiene|doctor> [--root path] [--db path]")
}

func resolveRootArg(root, path string, positional []string) (string, error) {
	root = strings.TrimSpace(root)
	path = strings.TrimSpace(path)
	if root != "" && path != "" {
		return "", errors.New("use only one of --root or --path")
	}
	selected := root
	if selected == "" {
		selected = path
	}
	if len(positional) > 0 {
		positionalRoot := strings.TrimSpace(positional[0])
		if selected != "" && positionalRoot != "" {
			return "", errors.New("use either a positional root path or --root/--path, not both")
		}
		if selected == "" {
			selected = positionalRoot
		}
	}
	return selected, nil
}

const (
	rootHelp = "markdown memory root; defaults to JAZMEM_ROOT or ~/.jaz/memory"
	dbHelp   = "sqlite index path; defaults to JAZMEM_DB, ~/.jaz/jazmem.sqlite, or <custom-root>/.jazmem/index.sqlite"
)
