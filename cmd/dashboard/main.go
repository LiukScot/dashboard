package main

import (
	"context"
	"log"
	"time"

	"github.com/LiukScot/dashboard/internal/auth"
	"github.com/LiukScot/dashboard/internal/collectors"
	"github.com/LiukScot/dashboard/internal/config"
	"github.com/LiukScot/dashboard/internal/db"
	"github.com/LiukScot/dashboard/internal/server"
)

func main() {
	cfg := config.Load()

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	if err := db.RunMigrations(database); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	authSvc := auth.NewService(database, cfg.SessionTTL)
	sysColl := collectors.NewSystemCollector(cfg.ProcPath)
	sysHist := collectors.NewSystemHistory(database, sysColl, time.Duration(cfg.MetricsInterval)*time.Second)
	dockerColl := collectors.NewDockerCollector(cfg.DockerSocket)
	f2bColl := collectors.NewFail2BanCollector(cfg.LogPath)
	logColl := collectors.NewLogCollector(cfg.LogPath)
	cronColl := collectors.NewCronCollector(database, cfg.CronPaths, cfg.LogPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sysHist.Run(ctx)

	srv := server.New(cfg, authSvc, sysColl, sysHist, dockerColl, f2bColl, logColl, cronColl)

	log.Fatal(srv.Start())
}
