package mcpapp

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jazmem/internal/mcpapi"
	"github.com/wins/jazmem/pkg/jazmem"
	"go.uber.org/fx"
)

type Args struct {
	Values []string
}

type Options struct {
	Root   string
	DBPath string
}

func ParseOptions(args Args) (Options, error) {
	fs := flag.NewFlagSet("jazmem-mcp", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("root", "", "markdown memory root; defaults to JAZMEM_ROOT or ~/.jaz/memory")
	dbPath := fs.String("db", "", "sqlite index path; defaults to JAZMEM_DB, ~/.jaz/jazmem.sqlite, or <custom-root>/.jazmem/index.sqlite")
	if err := fs.Parse(args.Values); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage(os.Stderr)
		}
		return Options{}, err
	}
	if len(fs.Args()) > 0 {
		return Options{}, fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	return Options{Root: *root, DBPath: *dbPath}, nil
}

func NewMemory(opts Options) (*jazmem.Memory, error) {
	return jazmem.Open(jazmem.Config{Root: opts.Root, DBPath: opts.DBPath})
}

func NewServer(memory *jazmem.Memory) *mcp.Server {
	return mcpapi.New(memory)
}

func RegisterMemoryLifecycle(lc fx.Lifecycle, memory *jazmem.Memory) {
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			return memory.Close()
		},
	})
}

func RunStdioServer(lc fx.Lifecycle, shutdowner fx.Shutdowner, server *mcp.Server) {
	var cancel context.CancelFunc
	done := make(chan error, 1)
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			ctx, stop := context.WithCancel(context.Background())
			cancel = stop
			go func() {
				err := server.Run(ctx, &mcp.StdioTransport{})
				if err != nil && !normalRunError(err) {
					fmt.Fprintln(os.Stderr, "jazmem-mcp:", err)
				}
				done <- err
				_ = shutdowner.Shutdown()
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			if cancel != nil {
				cancel()
			}
			select {
			case <-done:
				return nil
			default:
				return nil
			}
		},
	})
}

func normalRunError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, io.EOF)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: jazmem-mcp [--root path] [--db path]")
}
