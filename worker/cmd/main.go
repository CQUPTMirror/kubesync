package main

import (
	"github.com/gin-gonic/gin"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/moby/moby/pkg/reexec"
	"github.com/pkg/profile"
	"github.com/urfave/cli"
	"gopkg.in/op/go-logging.v1"

	"github.com/CQUPTMirror/kubesync/worker"
)

var logger = logging.MustGetLogger("tunasync")

func startWorker(c *cli.Context) error {
	cfg, err := worker.LoadConfig()
	if err != nil {
		logger.Errorf("Error loading config: %s", err.Error())
		os.Exit(1)
	}

	worker.InitLogger(c.Bool("verbose") || cfg.Verbose, c.Bool("debug") || cfg.Debug)
	if !(c.Bool("debug") || cfg.Debug) {
		gin.SetMode(gin.ReleaseMode)
	}

	w := worker.NewTUNASyncWorker(cfg)
	if w == nil {
		logger.Errorf("Error intializing TUNA sync worker.")
		os.Exit(1)
	}

	if profPath := c.String("prof-path"); profPath != "" {
		valid := false
		if fi, err := os.Stat(profPath); err == nil {
			if fi.IsDir() {
				valid = true
				defer profile.Start(profile.ProfilePath(profPath)).Stop()
			}
		}
		if !valid {
			logger.Errorf("Invalid profiling path: %s", profPath)
			os.Exit(1)
		}
	}

	go func() {
		time.Sleep(1 * time.Second)
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT)
		signal.Notify(sigChan, syscall.SIGTERM)
		for s := range sigChan {
			switch s {
			case syscall.SIGINT, syscall.SIGTERM:
				w.Halt()
			}
		}
	}()

	logger.Info("Run tunasync worker.")
	w.Run()
	return nil
}

func main() {

	if reexec.Init() {
		return
	}

	app := cli.NewApp()
	app.Name = "tunasync"
	app.Usage = "tunasync mirror job management tool"
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "verbose, v",
			Usage: "Enable verbose logging",
		},
		cli.BoolFlag{
			Name:  "debug",
			Usage: "Run worker in debug mode",
		},
		cli.StringFlag{
			Name:  "prof-path",
			Value: "",
			Usage: "Go profiling file path",
		},
	}
	app.Action = startWorker
	app.Run(os.Args)
}
