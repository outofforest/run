package run

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/outofforest/ioc/v2"
	"github.com/outofforest/logger"
	"github.com/outofforest/parallel"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
)

var mu sync.Mutex

// Service runs service app
func Service(appName string, containerBuilder func(c *ioc.Container), appFunc interface{}) {
	run(filepath.Base(appName), logger.ServiceDefaultConfig, containerBuilder, appFunc, parallel.Fail)
}

// Tool runs tool app
func Tool(appName string, containerBuilder func(c *ioc.Container), appFunc interface{}) {
	run(filepath.Base(appName), logger.ToolDefaultConfig, containerBuilder, appFunc, parallel.Exit)
}

func run(appName string, loggerConfig logger.Config, containerBuilder func(c *ioc.Container), setupFunc interface{}, exit parallel.OnExit) {
	log := logger.New(logger.ConfigureWithCLI(loggerConfig))
	if appName != "" && appName != "." {
		log = log.Named(appName)
	}
	ctx := logger.WithLogger(context.Background(), log)

	c := ioc.New()
	c.Singleton(func() context.Context {
		return ctx
	})
	if containerBuilder != nil {
		containerBuilder(c)
	}

	err := parallel.Run(ctx, func(ctx context.Context, spawn parallel.SpawnFn) error {
		spawn("", exit, func(ctx context.Context) error {
			defer func() {
				_ = log.Sync()
			}()

			c.Singleton(func() context.Context {
				return ctx
			})
			var err error
			c.Call(setupFunc, &err)
			return err
		})
		spawn("signals", parallel.Exit, func(ctx context.Context) error {
			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case sig := <-sigs:
				log.Info("Signal received, terminating...", zap.Stringer("signal", sig))
			}
			return nil
		})
		return nil
	})

	switch {
	case err == nil:
	case errors.Is(err, ctx.Err()):
	case errors.Is(err, pflag.ErrHelp):
		os.Exit(2)
	default:
		log.Error(fmt.Sprintf("Application returned error: %s", err), zap.Error(err))
		os.Exit(1)
	}
}
