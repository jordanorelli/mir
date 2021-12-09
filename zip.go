package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/zip"
)

func modbasename(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func zipcmd(args []string) {
	var (
		version    string
		outputPath string
	)

	flags := flag.NewFlagSet("zip", flag.ExitOnError)
	flags.StringVar(&version, "version", "", "package version")
	flags.StringVar(&outputPath, "o", "", "output file path")
	flags.Parse(args)

	if version == "" {
		bail(1, "target release version is required")
	}

	pkgdir := flags.Arg(0)
	if pkgdir == "" {
		pkgdir = "."
	}

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

	// major version compatibility check
	if err := module.Check(modpath, version); err != nil {
		shutdown(err)
	}

	// default to basename@version.zip
	if outputPath == "" {
		outputPath = fmt.Sprintf("%s@%s.zip", modbasename(modpath), version)
	}

	// check that destination is available
	log_info.Printf("destination: %s", outputPath)
	switch _, err := os.Stat(outputPath); {
	case err == nil:
		bail(1, "a file at %q already exists", outputPath)
	case os.IsNotExist(err):
		break
	default:
		bail(1, "unable to check for file at %q: %v", outputPath, err)
	}

	zf, err := os.CreateTemp("", fmt.Sprintf("modrn_*_%s_%s.zip", modbasename(modpath), version))
	if err != nil {
		bail(1, "creating zip failed to get a temp file: %v", err)
	}
	defer zf.Close()

	fi, err := zf.Stat()
	if err != nil {
		// is this even possible?
		bail(1, "unable to stat resultant tempfile: %v", err)
	}
	abspath := filepath.Join(os.TempDir(), fi.Name())
	log_info.Printf("zipping into temp file at %s", abspath)

	mv := module.Version{Path: modpath, Version: version}
	if err := zip.CreateFromDir(zf, mv, pkgdir); err != nil {
		bail(1, "zip not created: %v", err)
	}

	log_info.Printf("created zip archive at %v", abspath)
}
