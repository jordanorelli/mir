package main

import (
	"flag"
)

func serve(args []string) {
	// listen on this unix domain socket
	socketPath := ""

	// serve modules out of this root directory
	rootDir := "/srv/mir"

	// serve module traffic on this hostname
	hostname := ""

	httpAddr := ""

	serveFlags := flag.NewFlagSet("serve", flag.ExitOnError)
	serveFlags.StringVar(&socketPath, "unix", socketPath, "path for a unix domain socket to listen on")
	serveFlags.StringVar(&httpAddr, "http", httpAddr, "http address to listen on")
	serveFlags.StringVar(&rootDir, "root", rootDir, "root directory for module storage")
	serveFlags.StringVar(&hostname, "hostname", hostname, "domain name on which mir serves modules")
	serveFlags.Parse(args)

	h := handler{
		socketPath: socketPath,
		httpAddr:   httpAddr,
		root:       rootDir,
		hostname:   hostname,
	}
	if err := h.run(); err != nil {
		bail(1, err.Error())
	}
}
