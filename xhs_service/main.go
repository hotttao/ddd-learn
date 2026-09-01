package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"media_agent/hertz_infra/config"
	"media_agent/hertz_infra/serverhertz"
	crawlhandler "media_agent/xhs_service/biz/handler/crawl"
	crawlservice "media_agent/xhs_service/biz/service/crawl"
	"media_agent/xhs_service/biz/shared"
	ketoclient "media_agent/xhs_service/biz/shared/client/keto"
)

func main() {
	loader, err := config.NewLoader()
	if err != nil {
		log.Fatalf("init config loader: %v", err)
	}
	defer loader.Close()
	cfg := loader.Current()

	logCfg := cfg.GetLog()
	if logCfg.GetDir() == "" {
		logCfg.Dir = resolveDefaultLogDir(cfg.GetApp().GetName())
	}

	suite := serverhertz.NewServerSuite(loader)

	if err := loader.Attach(suite.KVSource()); err != nil {
		log.Fatalf("attach kv source: %v", err)
	}
	if err := loader.Watch(); err != nil {
		log.Fatalf("start config watch: %v", err)
	}

	shutdownObs, err := suite.InitObservability()
	if err != nil {
		log.Fatalf("init observability: %v", err)
	}
	defer shutdownObs(context.Background())

	ketoClient, err := ketoclient.New(cfg.GetKeto())
	if err != nil {
		log.Fatalf("init Keto client: %v", err)
	}
	crawlhandler.SetService(crawlservice.New(
		crawlservice.NewMockRepository(), ketoClient, shared.NewID, time.Now,
	))

	h, err := suite.NewServer(context.Background())
	if err != nil {
		log.Fatalf("init hertz server: %v", err)
	}
	register(h)
	suite.RegisterRoutes(h)

	if err := suite.RegisterService(); err != nil {
		log.Fatalf("register service: %v", err)
	}
	defer func() {
		if err := suite.DeregisterService(); err != nil {
			log.Printf("deregister service: %v", err)
		}
	}()

	log.Printf("xhs-service: listening on %s", cfg.GetServer().GetAddress())
	h.Spin()
}

func resolveDefaultLogDir(appName string) string {
	wd, _ := os.Getwd()
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return filepath.Join(dir, "media_logs", appName)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
