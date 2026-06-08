package main

import (
	"os"
	"time"

	"github.com/wins/jazmem/internal/mcpapp"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
)

func main() {
	fx.New(
		fx.StopTimeout(15*time.Second),
		fx.WithLogger(func() fxevent.Logger { return fxevent.NopLogger }),
		fx.Supply(mcpapp.Args{Values: os.Args[1:]}),
		fx.Provide(
			mcpapp.ParseOptions,
			mcpapp.NewMemory,
			mcpapp.NewServer,
		),
		fx.Invoke(
			mcpapp.RegisterMemoryLifecycle,
			mcpapp.RunStdioServer,
		),
	).Run()
}
