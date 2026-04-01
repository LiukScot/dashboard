package main

import (
	"log"

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
	dockerColl := collectors.NewDockerCollector(cfg.DockerSocket)
	f2bColl := collectors.NewFail2BanCollector(cfg.LogPath)
	logColl := collectors.NewLogCollector(cfg.LogPath)

	srv := server.New(cfg, authSvc, sysColl, dockerColl, f2bColl, logColl)

	log.Fatal(srv.Start())
}
