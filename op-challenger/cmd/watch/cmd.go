package watch

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	log "github.com/ethereum/go-ethereum/log"
	cli "github.com/urfave/cli"

	challenger "github.com/ethereum-optimism/optimism/op-challenger/challenger"
	config "github.com/ethereum-optimism/optimism/op-challenger/config"
	metrics "github.com/ethereum-optimism/optimism/op-challenger/metrics"

	oppprof "github.com/ethereum-optimism/optimism/op-service/pprof"
	oprpc "github.com/ethereum-optimism/optimism/op-service/rpc"
)

var Subcommands = cli.Commands{
	{
		Name:  "oracle",
		Usage: "Watches the L2OutputOracle for new output proposals",
		Action: func(ctx *cli.Context) error {
			logger, err := setupLogging(ctx)
			if err != nil {
				return err
			}
			logger.Info("Listening for new output proposals")

			cfg, err := config.NewConfigFromCLI(ctx)
			if err != nil {
				return err
			}

			return Oracle(logger, cfg)
		},
	},
	{
		Name:  "factory",
		Usage: "Watches the DisputeGameFactory for new dispute games",
		Action: func(ctx *cli.Context) error {
			key, err := Pub2PeerID(os.Stdin)
			if err != nil {
				return err
			}
			fmt.Println(key)
			return nil
		},
	},
}

// Oracle listens to the L2OutputOracle for newly proposed outputs.
func Oracle(logger log.Logger, cfg *config.Config) error {
	if err := cfg.Check(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	m := metrics.NewMetrics("default")
	logger.Info("Initializing Challenger")

	service, err := challenger.NewChallenger(*cfg, logger, m)
	if err != nil {
		logger.Error("Unable to create the Challenger", "error", err)
		return err
	}

	logger.Info("Starting Challenger")
	ctx, cancel := context.WithCancel(context.Background())
	if err := service.Start(); err != nil {
		cancel()
		logger.Error("Unable to start Challenger", "error", err)
		return err
	}
	defer service.Stop()

	logger.Info("Challenger started")
	pprofConfig := cfg.PprofConfig
	if pprofConfig.Enabled {
		logger.Info("starting pprof", "addr", pprofConfig.ListenAddr, "port", pprofConfig.ListenPort)
		go func() {
			if err := oppprof.ListenAndServe(ctx, pprofConfig.ListenAddr, pprofConfig.ListenPort); err != nil {
				logger.Error("error starting pprof", "err", err)
			}
		}()
	}

	metricsCfg := cfg.MetricsConfig
	if metricsCfg.Enabled {
		log.Info("starting metrics server", "addr", metricsCfg.ListenAddr, "port", metricsCfg.ListenPort)
		go func() {
			if err := m.Serve(ctx, metricsCfg.ListenAddr, metricsCfg.ListenPort); err != nil {
				logger.Error("error starting metrics server", err)
			}
		}()
		m.StartBalanceMetrics(ctx, logger, service.Client(), service.From())
	}

	rpcCfg := cfg.RPCConfig
	server := oprpc.NewServer(rpcCfg.ListenAddr, rpcCfg.ListenPort, version, oprpc.WithLogger(logger))
	if err := server.Start(); err != nil {
		cancel()
		return fmt.Errorf("error starting RPC server: %w", err)
	}

	m.RecordUp()

	interruptChannel := make(chan os.Signal, 1)
	signal.Notify(interruptChannel, []os.Signal{
		os.Interrupt,
		os.Kill,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	}...)
	<-interruptChannel
	cancel()

	return nil
}
