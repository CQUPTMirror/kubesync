package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/moby/moby/pkg/reexec"
	"github.com/pkg/profile"
	"github.com/urfave/cli"
	"gopkg.in/op/go-logging.v1"

	"github.com/ztelliot/kubesync/worker"
)

var logger = logging.MustGetLogger("tunasync")

func startWorker(c *cli.Context) error {
	gin.SetMode(gin.ReleaseMode)

	cfg, err := worker.LoadConfig(c.String("config"))
	if err != nil {
		logger.Errorf("Error loading config: %s", err.Error())
		os.Exit(1)
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
	app.Commands = []cli.Command{
		{
			Name:    "worker",
			Aliases: []string{"w"},
			Usage:   "start the tunasync worker",
			Action:  startWorker,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "config, c",
					Usage: "Load worker configurations from `FILE`",
				},
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
			},
		},
	}
	app.Run(os.Args)
}
