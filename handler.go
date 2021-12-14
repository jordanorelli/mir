package main

import (
	"archive/zip"
	"context"
	"crypto/md5"
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
	uploadP = regexp.MustCompile(`^/ul/(.+)/@v/(.+)\.zip$`)
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

	// $base/$module/@v/$version.mod - get go.mod file for a specific version
	if matches := modP.FindStringSubmatch(r.URL.Path); matches != nil {
		modpath := matches[1]
		modversion := matches[2]
		h.modfile(modpath, modversion, w, r)
		return
	}

	// $base/$module/@v/$version.zip - get the zip bundle of a package version
	if matches := zipP.FindStringSubmatch(r.URL.Path); matches != nil {
		modpath := matches[1]
		modversion := matches[2]
		h.zipfile(modpath, modversion, w, r)
		return
	}

	if matches := uploadP.FindStringSubmatch(r.URL.Path); matches != nil {
		modpath := matches[1]
		modversion := matches[2]
		h.upload(modpath, modversion, w, r)
		return
	}

	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte("not found"))
	return
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
	fmt.Fprintf(w, "internal server error: %v", err)
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

func (h handler) zipPath(modpath, version string) string {
	dirname, basename := filepath.Split(modpath)
	absdir := filepath.Join(h.root, "modules", dirname)
	return filepath.Join(absdir, fmt.Sprintf("%s@%s.zip", basename, version))
}

func (h handler) uploadPath(modpath, version string) string {
	hash := md5.Sum([]byte(fmt.Sprintf("%s@%s", modpath, version)))
	fname := fmt.Sprintf("%x.zip", hash)
	return filepath.Join(h.root, "uploads", fname)
}

func (h handler) openZip(modpath, version string) (io.ReadCloser, error) {
	return os.Open(h.zipPath(modpath, version))
}

// modfile serves the $base/$module/@v/$version.mod endpoint
func (h handler) modfile(modpath, modversion string, w http.ResponseWriter, r *http.Request) {
	rc, err := zip.OpenReader(h.zipPath(modpath, modversion))
	if err != nil {
		writeError(w, err)
		return
	}
	defer rc.Close()

	mfname := fmt.Sprintf("%s@%s/go.mod", modpath, modversion)
	f, err := rc.Open(mfname)
	if err != nil {
		writeError(w, err)
		return
	}
	defer f.Close()

	if _, err := io.Copy(w, f); err != nil {
		log_error.Printf("error copying modfile contents for %s version %s: %v", modpath, modversion, err)
	}
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

func (h handler) upload(modpath, modversion string, w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, apiError(http.StatusMethodNotAllowed))
		return
	}

	p, err := h.doUpload(modpath, modversion, r)
	if err != nil {
		writeError(w, err)
		return
	}

	if err := h.verifyUpload(modpath, modversion, p); err != nil {
		writeError(w, err)
		return
	}

	if err := os.Rename(p, h.zipPath(modpath, modversion)); err != nil {
		writeError(w, fmt.Errorf("unable to move upload into place: %w", err))
		return
	}
	w.Write([]byte("ok"))
}

func (h handler) doUpload(modpath, modversion string, r *http.Request) (string, error) {
	p := h.uploadPath(modpath, modversion)
	f, err := os.Create(p)
	if err != nil {
		return "", fmt.Errorf("unable to open destination path: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r.Body); err != nil {
		return "", fmt.Errorf("failed to write upload file locally: %w", err)
	}
	return p, nil
}

func (h handler) verifyUpload(modpath, modversion, fpath string) error {
	rc, err := zip.OpenReader(fpath)
	if err != nil {
		return fmt.Errorf("unable to verify upload: %w", err)
	}

	prefix := fmt.Sprintf("%s@%s/", modpath, modversion)
	for _, f := range rc.File {
		if !strings.HasPrefix(f.Name, prefix) {
			return fmt.Errorf("zip contains file with bad name: %w", err)
		}
	}
	return nil
}

type versionInfo struct {
	Version string    // version string
	Time    time.Time // commit time
}
