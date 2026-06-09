package main

import (
	"os"
	"time"

	"github.com/gluonfield/jazmem/internal/serverapp"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
)

func main() {
	fx.New(
		fx.StopTimeout(15*time.Second),
		fx.WithLogger(func() fxevent.Logger { return fxevent.NopLogger }),
		fx.Supply(serverapp.Args{Values: os.Args[1:]}),
		fx.Provide(
			serverapp.ParseOptions,
			serverapp.NewMemory,
			serverapp.NewHTTPServer,
		),
		fx.Invoke(
			serverapp.RegisterMemoryLifecycle,
			serverapp.StartScheduler,
			serverapp.StartHTTPServer,
		),
	).Run()
}
