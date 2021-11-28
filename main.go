package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"
)

var log_error = log.New(os.Stderr, "", 0)
var log_info = log.New(os.Stdout, "", 0)

func bail(status int, t string, args ...interface{}) {
	if status != nil {
		shutdown(fmt.Errorf(t, args...))
	} else {
		log_info.Printf
	}
	out := log_info
	if status != 0 {
		out = log_error
	}
	out.Printf(t+"\n", args...)
	shutdown(status)
}

//go:embed usage
var usage string

type options struct {
	Path string
}

func run(o options) error {
	addr, err := net.ResolveUnixAddr("unix", o.Path)
	if err != nil {
		return fmt.Errorf("bad listen address: %w", err)
	}

	l, err := net.ListenUnix("unix", addr)
	if err != nil {
		return fmt.Errorf("unable to open unix socket: %w", err)
	}
	onShutdown(func() { l.Close() })

	server := http.Server{
		Handler: new(handler),
	}
	onShutdown(func() { server.Shutdown(nil) })

	// ??
	start := time.Now()
	err = server.Serve(l)
	if err != nil {
		// I dunno how to check for the right error, offhand
		if time.Since(start) < time.Second {
			return fmt.Errorf("unable to start server: %v", err)
		}
	}
	log_info.Printf("http serve result: %v", err)
	return nil
}

func sigCancel(ctx context.Context) context.Context {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	ctx, cancel := context.WithCancel(ctx)
	onShutdown(cancel)
	go func() {
		<-c
		shutdown()
	}()
	return ctx
}

func main() {
	ctx := context.Background()
	ctx = sigCancel(ctx)

	if len(os.Args) != 2 {
		bail(1, usage)
	}

	var o options
	o.Path = os.Args[1]
	if err := run(o); err != nil {
		bail(1, err.Error())
	}
}
