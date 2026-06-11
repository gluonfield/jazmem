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

	"github.com/gluonfield/jazmem/pkg/jazmem"
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
	case "ask":
		return runAsk(args[1:])
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
	common, err := parseCommon("index", args)
	if err != nil {
		return err
	}
	b, err := openBackend(common.cfg, common.server, common.local)
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()
	report, err := b.Reindex(context.Background())
	if err != nil {
		return err
	}
	return printJSON(report)
}

type searchArgs struct {
	cfg     jazmem.Config
	query   string
	limit   int
	text    bool
	agentic bool
	deep    bool
	server  string
	local   bool
}

func runSearch(args []string) error {
	parsed, err := parseSearchArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if parsed.query == "" {
		return errors.New("search query is required")
	}
	b, err := openBackend(parsed.cfg, parsed.server, parsed.local)
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()
	if parsed.agentic {
		result, err := b.AgenticSearch(context.Background(), parsed.query, jazmem.AgenticOptions{Deep: parsed.deep})
		if err != nil {
			return err
		}
		if parsed.text {
			fmt.Print(jazmem.RenderAgenticText(result))
			return nil
		}
		return printJSON(result)
	}
	result, err := b.Retrieve(context.Background(), parsed.query, jazmem.SearchOptions{Limit: parsed.limit, Deep: parsed.deep})
	if err != nil {
		return err
	}
	if parsed.text {
		fmt.Print(jazmem.RenderSearchText(result))
		return nil
	}
	return printJSON(result)
}

// runAsk is the answer mode: agentic synthesis rendered as text. Equivalent
// to `jazmem --agentic --text`; --deep escalates retrieval.
func runAsk(args []string) error {
	parsed, err := parseSearchArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if parsed.query == "" {
		return errors.New("ask requires a question")
	}
	b, err := openBackend(parsed.cfg, parsed.server, parsed.local)
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()
	result, err := b.AgenticSearch(context.Background(), parsed.query, jazmem.AgenticOptions{Deep: parsed.deep})
	if err != nil {
		return err
	}
	fmt.Print(jazmem.RenderAgenticText(result))
	return nil
}

func parseSearchArgs(args []string) (searchArgs, error) {
	var root, path, db string
	parsed := searchArgs{limit: 10}
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
			parsed.text = true
		case arg == "-agentic" || arg == "--agentic":
			parsed.agentic = true
		case arg == "-deep" || arg == "--deep":
			parsed.deep = true
		case arg == "-local" || arg == "--local":
			parsed.local = true
		case strings.HasPrefix(arg, "-server=") || strings.HasPrefix(arg, "--server="):
			parsed.server = strings.TrimPrefix(strings.TrimPrefix(arg, "--server="), "-server=")
		case arg == "-server" || arg == "--server":
			if i+1 >= len(args) {
				return searchArgs{}, errors.New("server value is required")
			}
			parsed.server = args[i+1]
			i++
		case arg == "-h" || arg == "--help":
			usage(os.Stdout)
			return searchArgs{}, flag.ErrHelp
		case strings.HasPrefix(arg, "-limit=") || strings.HasPrefix(arg, "--limit="):
			value := strings.TrimPrefix(strings.TrimPrefix(arg, "--limit="), "-limit=")
			limit, err := parseLimit(value)
			if err != nil {
				return searchArgs{}, err
			}
			parsed.limit = limit
		case arg == "-limit" || arg == "--limit":
			if i+1 >= len(args) {
				return searchArgs{}, errors.New("limit value is required")
			}
			limit, err := parseLimit(args[i+1])
			if err != nil {
				return searchArgs{}, err
			}
			parsed.limit = limit
			i++
		case strings.HasPrefix(arg, "-root=") || strings.HasPrefix(arg, "--root="):
			root = strings.TrimPrefix(strings.TrimPrefix(arg, "--root="), "-root=")
		case arg == "-root" || arg == "--root":
			if i+1 >= len(args) {
				return searchArgs{}, errors.New("root value is required")
			}
			root = args[i+1]
			i++
		case strings.HasPrefix(arg, "-path=") || strings.HasPrefix(arg, "--path="):
			path = strings.TrimPrefix(strings.TrimPrefix(arg, "--path="), "-path=")
		case arg == "-path" || arg == "--path":
			if i+1 >= len(args) {
				return searchArgs{}, errors.New("path value is required")
			}
			path = args[i+1]
			i++
		case strings.HasPrefix(arg, "-db=") || strings.HasPrefix(arg, "--db="):
			db = strings.TrimPrefix(strings.TrimPrefix(arg, "--db="), "-db=")
		case arg == "-db" || arg == "--db":
			if i+1 >= len(args) {
				return searchArgs{}, errors.New("db value is required")
			}
			db = args[i+1]
			i++
		default:
			query = append(query, arg)
		}
	}
	selectedRoot, err := resolveRootArg(root, path, nil)
	if err != nil {
		return searchArgs{}, err
	}
	parsed.cfg = jazmem.Config{Root: selectedRoot, DBPath: db}
	parsed.query = strings.TrimSpace(strings.Join(query, " "))
	return parsed, nil
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
	server := fs.String("server", "", serverHelp)
	local := fs.Bool("local", false, localHelp)
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
	b, err := openBackend(cfg, *server, *local)
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()
	page, err := b.GetPage(context.Background(), rest[0])
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
	common, err := parseCommon("file", args)
	if err != nil {
		return err
	}
	if len(common.rest) != 1 {
		return errors.New("file requires exactly one slug")
	}
	b, err := openBackend(common.cfg, common.server, common.local)
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()
	page, err := b.GetPage(context.Background(), common.rest[0])
	if err != nil {
		return err
	}
	fmt.Println(page.Path)
	return nil
}

func runDream(args []string) error {
	common, err := parseCommon("dream", args)
	if err != nil {
		return err
	}
	b, err := openBackend(common.cfg, common.server, common.local)
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()
	report, err := b.Dream(context.Background())
	if err != nil {
		return err
	}
	return printJSON(report)
}

func runDoctor(args []string) error {
	common, err := parseCommon("doctor", args)
	if err != nil {
		return err
	}
	b, err := openBackend(common.cfg, common.server, common.local)
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()
	report, err := b.Doctor(context.Background())
	if err != nil {
		return err
	}
	return printJSON(report)
}

func runLinkHygiene(args []string) error {
	common, err := parseCommon("link-hygiene", args)
	if err != nil {
		return err
	}
	b, err := openBackend(common.cfg, common.server, common.local)
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()
	report, err := b.LinkHygiene(context.Background())
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

type commonArgs struct {
	cfg    jazmem.Config
	server string
	local  bool
	rest   []string
}

func parseCommon(name string, args []string) (commonArgs, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	root := fs.String("root", "", rootHelp)
	path := fs.String("path", "", "alias for --root")
	db := fs.String("db", "", dbHelp)
	server := fs.String("server", "", serverHelp)
	local := fs.Bool("local", false, localHelp)
	if err := fs.Parse(args); err != nil {
		return commonArgs{}, err
	}
	selectedRoot, err := resolveRootArg(*root, *path, nil)
	if err != nil {
		return commonArgs{}, err
	}
	return commonArgs{
		cfg:    jazmem.Config{Root: selectedRoot, DBPath: *db},
		server: *server,
		local:  *local,
		rest:   fs.Args(),
	}, nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func usage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: jazmem [--root path] [--db path] [--server url|--local] <query>")
	_, _ = fmt.Fprintln(w, "       jazmem ask [--deep] <question>          answer with citations (LLM)")
	_, _ = fmt.Fprintln(w, "       jazmem [--agentic] [--deep] [--text] [--limit n] <query>")
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
	rootHelp   = "markdown memory root; defaults to JAZMEM_ROOT or ~/.jaz/memory"
	dbHelp     = "sqlite index path; defaults to JAZMEM_DB, ~/.jaz/jazmem.sqlite, or <custom-root>/.jazmem/index.sqlite"
	serverHelp = "jazmem server URL; defaults to JAZMEM_SERVER, then auto-detects a local server"
	localHelp  = "force direct database access, skipping server detection"
)
