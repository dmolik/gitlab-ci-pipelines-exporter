package cmd

import (
	"fmt"
	stdlibLog "log"
	"net/url"
	"os"
	"time"

	"github.com/mvisonneau/gitlab-ci-pipelines-exporter/pkg/config"
	"github.com/mvisonneau/go-helpers/logger"
	"github.com/vmihailenco/taskq/v3"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var start time.Time

func configure(ctx *cli.Context) (cfg config.Config, err error) {
	start = ctx.App.Metadata["startTime"].(time.Time)

	assertStringVariableDefined(ctx, "config")

	cfg, err = config.ParseFile(ctx.String("config"))
	if err != nil {
		return
	}

	cfg.Global, err = parseGlobalFlags(ctx)
	if err != nil {
		return
	}

	configCliOverrides(ctx, &cfg)

	if err = cfg.Validate(); err != nil {
		return
	}

	// Configure logger
	if err = logger.Configure(logger.Config{
		Level:  cfg.Log.Level,
		Format: cfg.Log.Format,
	}); err != nil {
		return
	}

	// This hack is to embed taskq logs with logrus
	taskq.SetLogger(stdlibLog.New(log.StandardLogger().WriterLevel(log.WarnLevel), "taskq", 0))

	log.WithFields(
		log.Fields{
			"gitlab-endpoint":   cfg.Gitlab.URL,
			"gitlab-rate-limit": fmt.Sprintf("%drps", cfg.Gitlab.MaximumRequestsPerSecond),
		},
	).Info("configured")

	log.WithFields(config.SchedulerConfig(cfg.Pull.ProjectsFromWildcards).Log()).Info("pull projects from wildcards")
	log.WithFields(config.SchedulerConfig(cfg.Pull.EnvironmentsFromProjects).Log()).Info("pull environments from projects")
	log.WithFields(config.SchedulerConfig(cfg.Pull.RefsFromProjects).Log()).Info("pull refs from projects")
	log.WithFields(config.SchedulerConfig(cfg.Pull.Metrics).Log()).Info("pull metrics")

	log.WithFields(config.SchedulerConfig(cfg.GarbageCollect.Projects).Log()).Info("garbage collect projects")
	log.WithFields(config.SchedulerConfig(cfg.GarbageCollect.Environments).Log()).Info("garbage collect environments")
	log.WithFields(config.SchedulerConfig(cfg.GarbageCollect.Refs).Log()).Info("garbage collect refs")
	log.WithFields(config.SchedulerConfig(cfg.GarbageCollect.Metrics).Log()).Info("garbage collect metrics")

	return
}

func parseGlobalFlags(ctx *cli.Context) (cfg config.Global, err error) {
	cfg.InternalMonitoringListenerAddress, err = url.Parse(ctx.String("internal-monitoring-listener-address"))
	return
}

func exit(exitCode int, err error) cli.ExitCoder {
	defer log.WithFields(
		log.Fields{
			"execution-time": time.Since(start),
		},
	).Debug("exited..")

	if err != nil {
		log.Error(err.Error())
	}

	return cli.NewExitError("", exitCode)
}

// ExecWrapper gracefully logs and exits our `run` functions
func ExecWrapper(f func(ctx *cli.Context) (int, error)) cli.ActionFunc {
	return func(ctx *cli.Context) error {
		return exit(f(ctx))
	}
}

func configCliOverrides(ctx *cli.Context, cfg *config.Config) {
	if ctx.String("gitlab-token") != "" {
		cfg.Gitlab.Token = ctx.String("gitlab-token")
	}

	if cfg.Server.Webhook.Enabled {
		if ctx.String("webhook-secret-token") != "" {
			cfg.Server.Webhook.SecretToken = ctx.String("webhook-secret-token")
		}
	}

	if ctx.String("redis-url") != "" {
		cfg.Redis.URL = ctx.String("redis-url")
	}
}

func assertStringVariableDefined(ctx *cli.Context, k string) {
	if len(ctx.String(k)) == 0 {
		_ = cli.ShowAppHelp(ctx)
		log.Errorf("'--%s' must be set!", k)
		os.Exit(2)
	}
}
