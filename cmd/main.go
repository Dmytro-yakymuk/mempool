package main

import (
	"context"
	"fmt"
	"os"

	"github.com/caarlos0/env/v6"
	"github.com/joho/godotenv"
	_ "github.com/joho/godotenv/autoload"
	"github.com/spf13/cobra"
	"github.com/zeebo/errs"

	"mempool"
	"mempool/internal/logger/zaplog"
	"mempool/internal/process"
)

// Error is a default error type for mempool cli.
var Error = errs.Class("mempool cli")

// Config contains configurable values for mempool project.
type Config struct {
	mempool.Config
}

// commands.
var (
	rootCmd = &cobra.Command{
		Use:   "mempool",
		Short: "cli for interacting with mempool project",
	}
	runCmd = &cobra.Command{
		Use:         "run",
		Short:       "runs the program",
		RunE:        cmdRun,
		Annotations: map[string]string{"type": "run"},
	}
)

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().String("host", "0.0.0.0", "host")
	runCmd.Flags().String("port", "12345", "port")
	runCmd.Flags().String("btcrpc", "http://user:password@rpchost:48000", "btcrpc")
}

func main() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func cmdRun(cmd *cobra.Command, args []string) (err error) {
	ctx, cancel := context.WithCancel(context.Background())
	process.OnSigInt(func() {
		// starting graceful exit on context cancellation.
		cancel()
	})

	log := zaplog.NewLog()

	err = godotenv.Overload("./configs/.mempool.env")
	if err != nil {
		log.Error("could not load config: %v", Error.Wrap(err))
		return Error.Wrap(err)
	}

	config := new(Config)
	envOpt := env.Options{RequiredIfNoDef: true}
	err = env.Parse(config, envOpt)
	if err != nil {
		log.Error("could not parse config: %v", Error.Wrap(err))
		return Error.Wrap(err)
	}

	hostEnv, err := cmd.Flags().GetString("host")
	if err != nil {
		return Error.Wrap(err)
	}

	portEnv, err := cmd.Flags().GetString("port")
	if err != nil {
		return Error.Wrap(err)
	}

	btcrpcEnv, err := cmd.Flags().GetString("btcrpc")
	if err != nil {
		return Error.Wrap(err)
	}

	config.Config.Console.Address = fmt.Sprintf("%s:%s", hostEnv, portEnv)
	config.Config.BTCRPCAddress = btcrpcEnv

	peer, err := mempool.New(ctx, log, config.Config)
	if err != nil {
		log.Error("could not initialize peer", Error.Wrap(err))
		return Error.Wrap(err)
	}

	return errs.Combine(peer.Run(ctx), peer.Close())
}
