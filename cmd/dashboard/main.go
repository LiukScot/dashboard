package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LiukScot/dashboard/internal/auth"
	"github.com/LiukScot/dashboard/internal/collectors"
	"github.com/LiukScot/dashboard/internal/config"
	"github.com/LiukScot/dashboard/internal/db"
	"github.com/LiukScot/dashboard/internal/server"
)

func main() {
	// run() owns all deferred cleanup; main() only translates the result into an
	// exit code. log.Fatal calls os.Exit, which skips defers, so it must never
	// run while cleanup is pending.
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := config.Load()

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer database.Close()

	if err := db.RunMigrations(database); err != nil {
		return err
	}

	authSvc := auth.NewService(database, cfg.SessionTTL)
	sysColl := collectors.NewSystemCollector(cfg.ProcPath)
	sysHist := collectors.NewSystemHistory(database, sysColl, time.Duration(cfg.MetricsInterval)*time.Second)
	dockerColl := collectors.NewDockerCollector(cfg.DockerSocket)
	defer dockerColl.Close()
	f2bColl := collectors.NewFail2BanCollector(cfg.LogPath)
	logColl := collectors.NewLogCollector(cfg.LogPath)
	cronColl := collectors.NewCronCollector(database, cfg.CronPaths, cfg.LogPath)

	// SIGINT/SIGTERM cancels the root context, which drains the HTTP server and
	// stops the background collectors so the deferred Close calls actually run.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	sysHist.Run(ctx)

	srv := server.New(cfg, authSvc, sysColl, sysHist, dockerColl, f2bColl, logColl, cronColl)

	// Start returns nil on a clean ctx-driven shutdown and an error only on a
	// real serve failure (e.g. the port is already bound).
	return srv.Start(ctx)
}
