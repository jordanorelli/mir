package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"golang.org/x/mod/module"
	"golang.org/x/mod/zip"

	"orel.li/mir/internal/ref"
)

//go:embed meta
var content embed.FS

type handler struct {
	path ref.Ref[string]
	index ref.Ref[pathArg]
}

func (h handler) run() error {
	addr, err := net.ResolveUnixAddr("unix", h.path.Val())
	if err != nil {
		return fmt.Errorf("bad listen address: %w", err)
	}

	l, err := net.ListenUnix("unix", addr)
	if err != nil {
		return fmt.Errorf("unable to open unix socket: %w", err)
	}
	os.Chmod(h.path.Val(), 0777)

	server := http.Server{
		Handler: h,
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

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log_info.Printf("%s %s %s", r.Method, r.Host, r.URL.String())

	switch r.URL.Path {
	case "/fart":
		// Step 1: a request comes in at orel.li. This page contains a meta tag
		// indicating where the package contents may be found, and which backend
		// is serving the package.
		serveFile(w, "meta/fart/root.html")
	case "/modules/orel.li/fart/@v/list":
		// Step 2: list all of the versions for the package. Versions may be
		// available but unlisted.
		serveFile(w, "meta/fart/version-list")
	case "/modules/orel.li/fart/@latest",
		"/modules/orel.li/fart/@v/v0.0.3.info":
		// Step 3: get info for the version, which is just a timestamp at the
		// moment.
		e := json.NewEncoder(w)
		e.Encode(versionInfo{
			Version: "v0.0.3",
			Time:    time.Now(),
		})
	case "/modules/orel.li/fart/@v/v0.0.3.mod":
		// Step 4: retrieve the modfile for the package, informing go mod of
		// any transitive dependencies.
		serveFile(w, "meta/fart/modfile")
	case "/modules/orel.li/fart/@v/v0.0.3.zip":
		// Step 5: retrieve the source code contents for a package, as a
		// specially-formatted zip file.
		err := zip.CreateFromDir(w, module.Version{
			Path:    "orel.li/fart",
			Version: "v0.0.3",
		}, "/home/jorelli/mir/modules/orel.li/fart")
		if err != nil {
			log_error.Printf("zip error: %v", err)
		}
	case "/":
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}
}

func serveFile(w http.ResponseWriter, path string) {
	b, err := content.ReadFile(path)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, err.Error())
	}
	w.Write(b)
}

type versionInfo struct {
	Version string    // version string
	Time    time.Time // commit time
}
