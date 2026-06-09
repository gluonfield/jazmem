package serverapp

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/wins/jazmem/internal/httpapi"
	"github.com/wins/jazmem/pkg/jazmem"
	"go.uber.org/fx"
)

type Args struct {
	Values []string
}

type Options struct {
	Addr      string
	Root      string
	DBPath    string
	Scheduler bool
}

func ParseOptions(args Args) (Options, error) {
	fs := flag.NewFlagSet("jazmem-server", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:9477", "HTTP listen address")
	root := fs.String("root", "", "markdown memory root; defaults to JAZMEM_ROOT or ~/.jaz/memory")
	dbPath := fs.String("db", "", "sqlite index path; defaults to JAZMEM_DB, ~/.jaz/jazmem.sqlite, or <custom-root>/.jazmem/index.sqlite")
	scheduler := fs.Bool("scheduler", false, "run scheduled maintenance tasks")
	if err := fs.Parse(args.Values); err != nil {
		return Options{}, err
	}
	return Options{Addr: *addr, Root: *root, DBPath: *dbPath, Scheduler: *scheduler}, nil
}

func NewMemory(opts Options) (*jazmem.Memory, error) {
	return jazmem.Open(jazmem.Config{Root: opts.Root, DBPath: opts.DBPath})
}

func NewHTTPServer(opts Options, memory *jazmem.Memory) *http.Server {
	return &http.Server{
		Addr:              opts.Addr,
		Handler:           httpapi.New(memory),
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func RegisterMemoryLifecycle(lc fx.Lifecycle, memory *jazmem.Memory) {
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			return memory.Close()
		},
	})
}

func StartScheduler(lc fx.Lifecycle, opts Options, memory *jazmem.Memory) {
	if !opts.Scheduler {
		return
	}
	var cancel context.CancelFunc
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			ctx, stop := context.WithCancel(context.Background())
			cancel = stop
			go func() {
				if err := memory.StartScheduler(ctx); err != nil && ctx.Err() == nil {
					fmt.Fprintln(os.Stderr, "scheduler:", err)
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			if cancel != nil {
				cancel()
			}
			return nil
		},
	})
}

func StartHTTPServer(lc fx.Lifecycle, opts Options, memory *jazmem.Memory, server *http.Server) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			fmt.Printf("jazmem server listening on http://%s\n", opts.Addr)
			fmt.Printf("mcp endpoint: http://%s/mcp\n", opts.Addr)
			fmt.Printf("root: %s\n", memory.Root())
			fmt.Printf("db: %s\n", memory.DBPath())
			go func() {
				if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					fmt.Fprintln(os.Stderr, "serve:", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			done := make(chan error, 1)
			go func() { done <- server.Shutdown(ctx) }()
			select {
			case err := <-done:
				if errors.Is(err, http.ErrServerClosed) {
					return nil
				}
				return err
			case <-ctx.Done():
				return server.Close()
			}
		},
	})
}
