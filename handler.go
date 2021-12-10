package main

import (
	"context"
	"embed"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

//go:embed meta
var content embed.FS

// this is pretty janky, but I didn't want to import a routing library
var (
	latestP = regexp.MustCompile(`^/(.+)/@latest$`)
	listP   = regexp.MustCompile(`^/(.+)/@v/list$`)
	infoP   = regexp.MustCompile(`^/(.+)/@v/(.+)\.info$`)
	modP    = regexp.MustCompile(`^/(.+)/@v/(.+)\.mod$`)
	zipP    = regexp.MustCompile(`^/(.+)/@v/(.+)\.zip$`)
)

type handler struct {
	httpAddr   string
	socketPath string
	root       string
	hostname   string
}

func (h handler) run() error {
	if h.hostname == "" {
		return fmt.Errorf("hostname missing but hostname is required")
	}

	l, err := h.listen()
	if err != nil {
		return err
	}

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

func (h handler) listen() (net.Listener, error) {
	if h.httpAddr == "" && h.socketPath == "" {
		return nil, fmt.Errorf("must supply one of -http or -unix")
	}

	if h.httpAddr != "" {
		if h.socketPath != "" {
			return nil, fmt.Errorf("must supply (only) one of -http or -unix: supplied both")
		}

		lis, err := net.Listen("tcp", h.httpAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to start http listener: %w", err)
		}
		return lis, nil
	}

	lis, err := net.Listen("unix", h.socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to start unix socket listener: %w", err)
	}
	// TODO: what should this permission set be? hrm
	// TODO: what about the error here?
	os.Chmod(h.socketPath, 0777)
	return lis, nil
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log_info.Printf("%s %s %s %s", r.Method, r.Host, r.URL.Host, r.URL.String())

	// this is very stupid but I didn't want to add a routing library
	// dependency for five endpoints

	// if matches := latestP.FindStringSubmatch(r.URL.Path); matches != nil {
	// 	modpath := matches[1]
	// 	h.latest(modpath, w, r)
	// 	return
	// }

	if matches := listP.FindStringSubmatch(r.URL.Path); matches != nil {
		modpath := matches[1]
		h.list(modpath, w, r)
		return
	}

	// if matches := infoP.FindStringSubmatch(r.URL.Path); matches != nil {
	// 	modpath := matches[1]
	// 	modversion := matches[2]
	// 	h.info(modpath, modversion, w, r)
	// 	return
	// }

	// if matches := modP.FindStringSubmatch(r.URL.Path); matches != nil {
	// 	modpath := matches[1]
	// 	modversion := matches[2]
	// 	h.modfile(modpath, modversion, w, r)
	// 	return
	// }

	// if matches := zipP.FindStringSubmatch(r.URL.Path); matches != nil {
	// 	modpath := matches[1]
	// 	modversion := matches[2]
	// 	h.zipfile(modpath, modversion, w, r)
	// 	return
	// }

	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte("not found"))
	return

	// switch r.URL.Path {
	// case "/fart":
	// 	// Step 1: a request comes in at orel.li. This page contains a meta tag
	// 	// indicating where the package contents may be found, and which backend
	// 	// is serving the package.
	// 	serveFile(w, "meta/fart/root.html")
	// case "/modules/orel.li/fart/@v/list":
	// 	// Step 2: list all of the versions for the package. Versions may be
	// 	// available but unlisted.
	// 	serveFile(w, "meta/fart/version-list")
	// case "/modules/orel.li/fart/@latest",
	// 	"/modules/orel.li/fart/@v/v0.0.3.info":
	// 	// Step 3: get info for the version, which is just a timestamp at the
	// 	// moment.
	// 	e := json.NewEncoder(w)
	// 	e.Encode(versionInfo{
	// 		Version: "v0.0.3",
	// 		Time:    time.Now(),
	// 	})
	// case "/modules/orel.li/fart/@v/v0.0.3.mod":
	// 	// Step 4: retrieve the modfile for the package, informing go mod of
	// 	// any transitive dependencies.
	// 	serveFile(w, "meta/fart/modfile")
	// case "/modules/orel.li/fart/@v/v0.0.3.zip":
	// 	// Step 5: retrieve the source code contents for a package, as a
	// 	// specially-formatted zip file.
	// 	err := zip.CreateFromDir(w, module.Version{
	// 		Path:    "orel.li/fart",
	// 		Version: "v0.0.3",
	// 	}, "/home/jorelli/mir/modules/orel.li/fart")
	// 	if err != nil {
	// 		log_error.Printf("zip error: %v", err)
	// 	}
	// case "/":
	// 	w.WriteHeader(http.StatusOK)
	// default:
	// 	w.WriteHeader(http.StatusNotFound)
	// 	w.Write([]byte("not found"))
	// }
}

// locate searches our module root for a given modpath
func (h handler) locate(modpath string) ([]os.DirEntry, error) {
	localDir := filepath.Join(h.root, "modules", modpath)
	return os.ReadDir(localDir)
}

func writeError(w http.ResponseWriter, err error) {
	if os.IsNotExist(err) {
		log_info.Printf("404 %v", err)
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "not found")
		return
	}

	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, "internal server error")
	log_error.Printf("500 %v", err)
	return
}

// latest serves the @latest endpoint
func (h handler) latest(modpath string, w http.ResponseWriter, r *http.Request) {
}

// list serves the $base/$module/@v/list endpoint
func (h handler) list(modpath string, w http.ResponseWriter, r *http.Request) {
	log_info.Printf("list: %s", modpath)
	dirpath, _ := filepath.Split(modpath)
	log_info.Printf("dirpath: %s", dirpath)
	localDir := filepath.Join(h.root, "modules", dirpath)
	log_info.Printf("localDir: %s", localDir)
	files, err := os.ReadDir(localDir)
	if err != nil {
		writeError(w, err)
		return
	}

	allVersions := make([]string, 0, len(files))
	for _, f := range files {
		name := f.Name()
		if filepath.Ext(name) != ".zip" {
			log_info.Printf("not a zip: %s", name)
			continue
		}
		parts := strings.Split(name, "@")
		if len(parts) != 2 {
			continue
		}
		if !semver.IsValid(parts[1]) {
			continue
		}
		allVersions = append(allVersions, parts[1])
	}

	semver.Sort(allVersions)
	if len(allVersions) == 0 {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "not found")
		return
	}
	for _, version := range allVersions {
		fmt.Fprint(w, version)
	}
}

// info serves the $base/$module/@v/$version.info endpoint
func (h handler) info(modpath, modversion string, w http.ResponseWriter, r *http.Request) {
}

// modfile serves the $base/$module/@v/$version.mod endpoint
func (h handler) modfile(modpath, modversion string, w http.ResponseWriter, r *http.Request) {
}

// zipfile serves the $base/$module/@v/$version.zip endpoint
func (h handler) zipfile(modpath, modversion string, w http.ResponseWriter, r *http.Request) {
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
