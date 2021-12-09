package main

import (
	"flag"

	"orel.li/mir/internal/index"
	"orel.li/mir/internal/ref"
)

func serve(args []string) {
	path := "./modularium.sock"
	indexPath := pathArg{path: "./modules-index.json"}

	serveFlags := flag.NewFlagSet("serve", flag.ExitOnError)
	serveFlags.StringVar(&path, "l", path, "path for a unix domain socket to listen on")
	serveFlags.Var(&indexPath, "index", "an index config")
	serveFlags.Parse(args)

	idx, err := index.Load(indexPath.path)
	if err != nil {
		shutdown(err)
	}
	log_info.Printf("index: %v", idx)

	h := handler{
		path:  ref.New(&path),
		index: ref.New(&indexPath),
	}
	if err := h.run(); err != nil {
		bail(1, err.Error())
	}
}
