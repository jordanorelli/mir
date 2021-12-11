package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// this is pretty janky, but I didn't want to import a routing library
var (
	latestP = regexp.MustCompile(`^/dl/(.+)/@latest$`)
	listP   = regexp.MustCompile(`^/dl/(.+)/@v/list$`)
	infoP   = regexp.MustCompile(`^/dl/(.+)/@v/(.+)\.info$`)
	modP    = regexp.MustCompile(`^/dl/(.+)/@v/(.+)\.mod$`)
	zipP    = regexp.MustCompile(`^/dl/(.+)/@v/(.+)\.zip$`)
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
	// dependency for five endpoints, since part of my goal is to not depend on
	// anything with github.com in the import path.

	// $base/$module/@v/list - list versions for a module
	if matches := listP.FindStringSubmatch(r.URL.Path); matches != nil {
		modpath := matches[1]
		h.list(modpath, w, r)
		return
	}

	// $base/$module/@latest - get latest version number with timestamp
	if matches := latestP.FindStringSubmatch(r.URL.Path); matches != nil {
		modpath := matches[1]
		h.latest(modpath, w, r)
		return
	}

	// $base/$module/@v/$version.info - get info about a specific version
	if matches := infoP.FindStringSubmatch(r.URL.Path); matches != nil {
		modpath := matches[1]
		modversion := matches[2]
		h.info(modpath, modversion, w, r)
		return
	}

	// if matches := modP.FindStringSubmatch(r.URL.Path); matches != nil {
	// 	modpath := matches[1]
	// 	modversion := matches[2]
	// 	h.modfile(modpath, modversion, w, r)
	// 	return
	// }

	// $base/$module/@v/$version.zip - get the zip bundle of a package version
	if matches := zipP.FindStringSubmatch(r.URL.Path); matches != nil {
		modpath := matches[1]
		modversion := matches[2]
		h.zipfile(modpath, modversion, w, r)
		return
	}

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

// writeError writes a given error to an underlying http responsewriter
func writeError(w http.ResponseWriter, err error) {
	// this sucks and is wrong
	if os.IsNotExist(err) {
		log_info.Printf("404 %v", err)
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "not found")
		return
	}

	var status apiError
	if errors.As(err, &status) {
		w.WriteHeader(int(status))
		log_error.Printf("%d %v", status, err)
		fmt.Fprintf(w, err.Error())
		return
	}

	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, "internal server error")
	log_error.Printf("500 %v", err)
	return
}

// getVersions gets the list of versions available for a module
func (h handler) getVersions(modpath string) ([]string, error) {
	dirpath, _ := filepath.Split(modpath)
	localDir := filepath.Join(h.root, "modules", dirpath)
	files, err := os.ReadDir(localDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, apiError(http.StatusNotFound)
		}
		if errors.Is(err, fs.ErrPermission) {
			return nil, apiError(http.StatusForbidden)
		}
		return nil, joinErrors(err, apiError(http.StatusInternalServerError))
	}

	allVersions := make([]string, 0, len(files))
	for _, f := range files {
		name := f.Name()
		if filepath.Ext(name) != ".zip" {
			continue
		}
		parts := strings.Split(name[:len(name)-4], "@")
		if len(parts) != 2 {
			continue
		}
		if !semver.IsValid(parts[1]) {
			continue
		}
		allVersions = append(allVersions, parts[1])
	}

	if len(allVersions) == 0 {
		return nil, apiError(http.StatusNotFound)
	}
	semver.Sort(allVersions)
	return allVersions, nil
}

func (h handler) stat(modpath, version string) (os.FileInfo, error) {
	log_info.Printf("stat modpath: %s version: %s", modpath, version)
	dirname, basename := filepath.Split(modpath)
	absdir := filepath.Join(h.root, "modules", dirname)
	fname := filepath.Join(absdir, fmt.Sprintf("%s@%s.zip", basename, version))
	return os.Stat(fname)
}

// latest serves the @latest endpoint
func (h handler) latest(modpath string, w http.ResponseWriter, r *http.Request) {
	versions, err := h.getVersions(modpath)
	if err != nil {
		writeError(w, err)
		return
	}
	log_info.Printf("all versions: %v", versions)

	last := versions[len(versions)-1]

	fi, err := h.stat(modpath, last)
	if err != nil {
		writeError(w, err)
		return
	}
	json.NewEncoder(w).Encode(moduleInfo{
		Version: last,
		Time:    fi.ModTime(),
	})
}

// list serves the $base/$module/@v/list endpoint
func (h handler) list(modpath string, w http.ResponseWriter, r *http.Request) {
	versions, err := h.getVersions(modpath)
	if err != nil {
		writeError(w, err)
		return
	}

	for _, version := range versions {
		fmt.Fprintln(w, version)
	}
}

// info serves the $base/$module/@v/$version.info endpoint
func (h handler) info(modpath, modversion string, w http.ResponseWriter, r *http.Request) {
	fi, err := h.stat(modpath, modversion)
	if err != nil {
		writeError(w, err)
		return
	}
	json.NewEncoder(w).Encode(moduleInfo{
		Version: modversion,
		Time:    fi.ModTime(),
	})
}

func (h handler) openZip(modpath, version string) (io.ReadCloser, error) {
	dirname, basename := filepath.Split(modpath)
	absdir := filepath.Join(h.root, "modules", dirname)
	fname := filepath.Join(absdir, fmt.Sprintf("%s@%s.zip", basename, version))
	return os.Open(fname)
}

// modfile serves the $base/$module/@v/$version.mod endpoint
func (h handler) modfile(modpath, modversion string, w http.ResponseWriter, r *http.Request) {
}

// zipfile serves the $base/$module/@v/$version.zip endpoint
func (h handler) zipfile(modpath, modversion string, w http.ResponseWriter, r *http.Request) {
	zf, err := h.openZip(modpath, modversion)
	if err != nil {
		writeError(w, err)
		return
	}
	defer zf.Close()

	w.Header().Add("Content-Type", "application/zip")
	if _, err := io.Copy(w, zf); err != nil {
		log_error.Printf("error writing zip for module %s version %s: %v", modpath, modversion, err)
	}
}

type versionInfo struct {
	Version string    // version string
	Time    time.Time // commit time
}
