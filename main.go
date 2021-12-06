package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"os/signal"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"

	"orel.li/modularium/internal/index"
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

func archive(args []string) {
	var (
		version string
	)

	flags := flag.NewFlagSet("archive", flag.ExitOnError)
	flags.StringVar(&version, "version", "", "package version")
	flags.Parse(args)
	if version == "" {
		bail(1, "target release version is required")
	}

	pkgdir := flags.Arg(0)
	modfilePath := filepath.Join(pkgdir, "go.mod")
	b, err := ioutil.ReadFile(modfilePath)
	if err != nil {
		bail(1, "unable to read modfile: %v", err)
	}

	log_info.Printf("checking modfile at path %q", modfilePath)
	f, err := modfile.Parse(modfilePath, b, nil)
	if err != nil {
		bail(1, "unable to parse modfile: %v", err)
	}
	modpath := f.Module.Mod.Path
	log_info.Print("parsed modfile")
	log_info.Printf("module path in modfile: %s", modpath)
	log_info.Printf("module major version in modfile: %s", f.Module.Mod.Version)
	log_info.Printf("target release version: %s", version)
	if err := module.Check(modpath, version); err != nil {
		shutdown(err)
	}
}

func main() {
	sigCancel(context.Background())
	root := flag.NewFlagSet("", flag.ExitOnError)
	root.Parse(os.Args[1:])

	switch root.Arg(0) {
	case "serve":
		serve(root.Args()[1:])
	case "archive":
		archive(root.Args()[1:])
	default:
		bail(0, usage)
	}
}
