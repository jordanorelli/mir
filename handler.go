package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/mod/module"
	"golang.org/x/mod/zip"
)

//go:embed meta
var content embed.FS

type handler struct {
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log_info.Printf("%s %s", r.Method, r.URL.String())

	switch r.URL.Path {
	case "/fart":
		serveFile(w, "meta/fart/root.html")
	case "/modules/orel.li/fart/@v/list":
		serveFile(w, "meta/fart/version-list")
	case "/modules/orel.li/fart/@latest",
		"/modules/orel.li/fart/@v/v0.0.3.info":
		e := json.NewEncoder(w)
		e.Encode(versionInfo{
			Version: "v0.0.3",
			Time:    time.Now(),
		})
	case "/modules/orel.li/fart/@v/v0.0.3.mod":
		serveFile(w, "meta/fart/modfile")
	case "/modules/orel.li/fart/@v/v0.0.3.zip":
		err := zip.CreateFromDir(w, module.Version{
			Path:    "orel.li/fart",
			Version: "v0.0.3",
		}, "/home/jorelli/modularium/modules/orel.li/fart")
		if err != nil {
			log_error.Printf("zip error: %v", err)
		}
	case "/":
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusNotFound)
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
