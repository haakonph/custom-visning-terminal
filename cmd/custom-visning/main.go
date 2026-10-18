package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/haakonph/custom-visning-terminal/internal/app"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	startJira := flag.Bool("jira", false, "start in Jira; enable GitHub switching when credentials are available")
	startCopilot := flag.Bool("copilot", false, "start in Copilot; collects local history and OTLP without GitHub credentials")
	flag.Parse()
	if *showVersion {
		fmt.Println("custom-visning " + version)
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := app.RunProviders(ctx, os.Stderr, *startJira, *startCopilot); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
