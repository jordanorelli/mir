package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
)

var (
	log_error = log.New(os.Stderr, "", 0)
	log_info  = log.New(os.Stdout, "", 0)
	log_debug = log.New(io.Discard, "", 0)
)

func bail(status int, t string, args ...interface{}) {
	if status != 0 {
		shutdown(fmt.Errorf(t, args...))
	} else {
		log_info.Printf(t, args...)
		shutdown(nil)
	}
}

//go:embed usage
var usage string

func sigCancel(ctx context.Context) context.Context {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	ctx, cancel := context.WithCancel(ctx)
	onShutdown(func() error { cancel(); return nil })
	go func() {
		<-c
		shutdown(nil)
	}()
	return ctx
}

func main() {
	var (
		quiet   bool
		verbose bool
	)

	sigCancel(context.Background())
	root := flag.NewFlagSet("", flag.ExitOnError)
	root.BoolVar(&quiet, "q", false, "suppress non-error output")
	root.BoolVar(&verbose, "v", false, "show additional debug output")
	root.Parse(os.Args[1:])

	if quiet {
		log_info = log.New(io.Discard, "", 0)
	}
	if !quiet && verbose {
		log_debug = log.New(os.Stdout, "", 0)
	}

	rest := root.Args()[1:]

	switch root.Arg(0) {
	case "serve":
		serve(rest)
	case "zip":
		zipcmd(rest)
	case "pwhash":
		pwhashcmd(rest)
	case "next":
		nextcmd(rest)
		// mir next major
		// mir next minor
		// mir next patch
		// mir next pre fartstorm

	default:
		bail(0, usage)
	}
}
