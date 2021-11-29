package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"orel.li/modularium/internal/ref"
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
		path := "/var/run/orel.li/http.sock"
		index := pathArg{path: "./modules-index.json"}
		h := handler{
			path:  ref.New(&path),
			index: ref.New(&index),
		}

		serveFlags := flag.NewFlagSet("serve", flag.ExitOnError)
		serveFlags.StringVar(&path, "l", path, "path for a unix domain socket to listen on")

		serveFlags.Var(&index, "index", "an index config")
		serveFlags.Parse(root.Args()[1:])

		if err := h.run(); err != nil {
			bail(1, err.Error())
		}
	default:
		bail(0, usage)
	}
}
