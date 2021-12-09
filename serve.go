package main

import (
	"flag"

	"orel.li/mir/internal/ref"
)

func serve(args []string) {
	path := "./mir.sock"
	rootDir := "/srv/mir"

	serveFlags := flag.NewFlagSet("serve", flag.ExitOnError)
	serveFlags.StringVar(&path, "l", path, "path for a unix domain socket to listen on")
	serveFlags.StringVar(&rootDir, "root", rootDir, "root directory for module storage")
	serveFlags.Parse(args)

	h := handler{
		path: ref.New(&path),
	}
	if err := h.run(); err != nil {
		bail(1, err.Error())
	}
}
