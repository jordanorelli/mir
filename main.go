package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
)

var log_error = log.New(os.Stderr, "", 0)
var log_info = log.New(os.Stdout, "", 0)

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
	sigCancel(context.Background())
	root := flag.NewFlagSet("", flag.ExitOnError)
	root.Parse(os.Args[1:])

	switch root.Arg(0) {
	case "serve":
		serve(root.Args()[1:])
	case "zip":
		zipcmd(root.Args()[1:])
	case "pwhash":
		pwhashcmd(root.Args()[1:])
	default:
		bail(0, usage)
	}
}
