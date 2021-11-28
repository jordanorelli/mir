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
	if status != 0 {
		shutdown(fmt.Errorf(t, args...))
	} else {
		log_info.Printf(t, args...)
		shutdown(nil)
	}
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
	os.Chmod(o.Path, 0777)

	server := http.Server{
		Handler: new(handler),
	}
	onShutdown(func() error {
		log_info.Print("shutting down http server")
		return server.Shutdown(context.TODO())
	})

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
	onShutdown(func() error { cancel(); return nil })
	go func() {
		<-c
		shutdown(nil)
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
