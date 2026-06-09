package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
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
	case "-h", "--help", "help":
		usage(os.Stdout)
		return nil
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
	case "dream":
		return runDream(args[1:])
	case "link-hygiene":
		return runLinkHygiene(args[1:])
	case "eval":
		return runEval(args[1:])
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
	defer func() { _ = m.Close() }()
	report, err := m.Reindex(context.Background(), jazmem.ReindexOptions{})
	if err != nil {
		return err
	}
	return printJSON(report)
}

func runSearch(args []string) error {
	cfg, query, limit, text, agentic, err := parseSearchArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if query == "" {
		return errors.New("search query is required")
	}
	m, err := jazmem.Open(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = m.Close() }()
	if agentic {
		result, err := m.AgenticSearch(context.Background(), query, jazmem.AgenticOptions{Limit: limit})
		if err != nil {
			return err
		}
		if text {
			fmt.Print(jazmem.RenderAgenticText(result))
			return nil
		}
		return printJSON(result)
	}
	result, err := m.Retrieve(context.Background(), query, jazmem.SearchOptions{Limit: limit})
	if err != nil {
		return err
	}
	if text {
		fmt.Print(jazmem.RenderSearchText(result))
		return nil
	}
	return printJSON(result)
}

func parseSearchArgs(args []string) (jazmem.Config, string, int, bool, bool, error) {
	var root, path, db string
	limit := 10
	text := false
	agentic := false
	var query []string
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "":
			continue
		case arg == "--":
			query = append(query, args[i+1:]...)
			i = len(args)
		case arg == "-text" || arg == "--text":
			text = true
		case arg == "-agentic" || arg == "--agentic":
			agentic = true
		case arg == "-h" || arg == "--help":
			usage(os.Stdout)
			return jazmem.Config{}, "", 0, text, agentic, flag.ErrHelp
		case strings.HasPrefix(arg, "-limit=") || strings.HasPrefix(arg, "--limit="):
			value := strings.TrimPrefix(strings.TrimPrefix(arg, "--limit="), "-limit=")
			parsed, err := parseLimit(value)
			if err != nil {
				return jazmem.Config{}, "", 0, false, false, err
			}
			limit = parsed
		case arg == "-limit" || arg == "--limit":
			if i+1 >= len(args) {
				return jazmem.Config{}, "", 0, false, false, errors.New("limit value is required")
			}
			parsed, err := parseLimit(args[i+1])
			if err != nil {
				return jazmem.Config{}, "", 0, false, false, err
			}
			limit = parsed
			i++
		case strings.HasPrefix(arg, "-root=") || strings.HasPrefix(arg, "--root="):
			root = strings.TrimPrefix(strings.TrimPrefix(arg, "--root="), "-root=")
		case arg == "-root" || arg == "--root":
			if i+1 >= len(args) {
				return jazmem.Config{}, "", 0, false, false, errors.New("root value is required")
			}
			root = args[i+1]
			i++
		case strings.HasPrefix(arg, "-path=") || strings.HasPrefix(arg, "--path="):
			path = strings.TrimPrefix(strings.TrimPrefix(arg, "--path="), "-path=")
		case arg == "-path" || arg == "--path":
			if i+1 >= len(args) {
				return jazmem.Config{}, "", 0, false, false, errors.New("path value is required")
			}
			path = args[i+1]
			i++
		case strings.HasPrefix(arg, "-db=") || strings.HasPrefix(arg, "--db="):
			db = strings.TrimPrefix(strings.TrimPrefix(arg, "--db="), "-db=")
		case arg == "-db" || arg == "--db":
			if i+1 >= len(args) {
				return jazmem.Config{}, "", 0, false, false, errors.New("db value is required")
			}
			db = args[i+1]
			i++
		default:
			query = append(query, arg)
		}
	}
	selectedRoot, err := resolveRootArg(root, path, nil)
	if err != nil {
		return jazmem.Config{}, "", 0, false, false, err
	}
	return jazmem.Config{Root: selectedRoot, DBPath: db}, strings.TrimSpace(strings.Join(query, " ")), limit, text, agentic, nil
}

func parseLimit(value string) (int, error) {
	limit, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, err
	}
	if limit <= 0 {
		return 0, errors.New("limit must be positive")
	}
	return limit, nil
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
	defer func() { _ = m.Close() }()
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
	defer func() { _ = m.Close() }()
	page, err := m.GetPage(context.Background(), rest[0])
	if err != nil {
		return err
	}
	fmt.Println(page.Path)
	return nil
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
	defer func() { _ = m.Close() }()
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
	defer func() { _ = m.Close() }()
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
	defer func() { _ = m.Close() }()
	report, err := m.LinkHygiene(context.Background())
	if err != nil {
		return err
	}
	return printJSON(report)
}

func runEval(args []string) error {
	fs := flag.NewFlagSet("eval", flag.ContinueOnError)
	root := fs.String("root", "", rootHelp)
	path := fs.String("path", "", "alias for --root")
	db := fs.String("db", "", dbHelp)
	file := fs.String("file", "", "eval JSON file; defaults to embedded jazmem eval set")
	limit := fs.Int("limit", 5, "retrieval limit per eval case")
	if err := fs.Parse(args); err != nil {
		return err
	}
	selectedRoot, err := resolveRootArg(*root, *path, nil)
	if err != nil {
		return err
	}
	var cases []jazmem.EvalCase
	if strings.TrimSpace(*file) != "" {
		data, err := os.ReadFile(*file)
		if err != nil {
			return err
		}
		cases, err = jazmem.ParseEvalCases(data)
		if err != nil {
			return err
		}
	}
	m, err := jazmem.Open(jazmem.Config{Root: selectedRoot, DBPath: *db})
	if err != nil {
		return err
	}
	defer func() { _ = m.Close() }()
	report, err := m.Evaluate(context.Background(), jazmem.EvalOptions{Cases: cases, Limit: *limit})
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
	_, _ = fmt.Fprintln(w, "usage: jazmem [--root path] [--db path] <query>")
	_, _ = fmt.Fprintln(w, "       jazmem [--agentic] [--text] [--limit n] <query>")
	_, _ = fmt.Fprintln(w, "       jazmem init [--root path|--path path|path] [--db path]")
	_, _ = fmt.Fprintln(w, "       jazmem <index|search|get|page|file|dream|link-hygiene|eval|doctor> [--root path] [--db path]")
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
