package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/net/html"

	"orel.li/mir/internal/semver"
)

func nextcmd(args []string) {
	flags := flag.NewFlagSet("next", flag.ExitOnError)
	flags.Parse(args)

	switch flags.Arg(0) {
	case "major", "minor", "patch":
	default:
		bail(1, "major|minor|patch only for now")
	}

	log_debug.Printf("reading module file go.mod")
	b, err := ioutil.ReadFile("go.mod")
	if err != nil {
		bail(1, "unable to read modfile: %v", err)
	}

	log_debug.Printf("parsing module file go.mod")
	f, err := modfile.Parse("go.mod", b, nil)
	if err != nil {
		bail(1, "unable to parse modfile: %v", err)
	}

	modpath := f.Module.Mod.Path
	log_debug.Printf("parsed module path: %s", modpath)

	u, err := url.Parse(f.Module.Mod.Path)
	if err != nil {
		bail(1, "module path %s is not a valid url: %v", err)
	}
	u.Scheme = "https"

	log_debug.Printf("GET %v", u)
	res, err := http.Get(u.String())
	if err != nil {
		bail(1, "unable to fetch module root page: %v", err)
	}
	defer res.Body.Close()

	m, err := parseModPage(res.Body)
	if err != nil {
		bail(1, "unable to parse module meta page: %v", err)
	}

	lines, err := m.fetchVersionList()
	if err != nil {
		bail(1, "unable to fetch version list: %v", err)
	}
	log_debug.Printf("%s", lines)
}

// parseModPage parses the page at a module path, looking for the appropriate
// meta tags defined by the Go module ecosystem and by mir itself
func parseModPage(r io.Reader) (*modmeta, error) {
	root, err := html.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("invalid html: %v", err)
	}

	var meta modmeta
	if err := meta.parseTree(root); err != nil {
		return nil, fmt.Errorf("parse failed: %v", err)
	}
	return &meta, nil
}

type modmeta struct {
	path    string  // module path
	backend string  // git | hg | mod
	dlRoot  url.URL // download URL
}

// parseTree parses an HTML tree, evaluating each node and then descending to
// its children
func (m *modmeta) parseTree(n *html.Node) error {
	m.parseNode(n)

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if err := m.parseTree(c); err != nil {
			return err
		}
	}
	return nil
}

// attr finds the first value for a given attribute in an html attribute list
func attr(key string, attrs []html.Attribute) string {
	for _, a := range attrs {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// parseNode parses a single node in an html tree, without looking at its
// descendants
func (m *modmeta) parseNode(n *html.Node) error {
	if n.Type != html.ElementNode {
		return nil
	}
	if n.Data != "meta" {
		return nil
	}

	if attr("name", n.Attr) == "go-import" {
		content := attr("content", n.Attr)
		if content == "" {
			return fmt.Errorf("go-import meta tag is missing content")
		}

		parts := strings.Fields(content)
		if len(parts) != 3 {
			return fmt.Errorf("go import meta tag has invalid content (not 3 parts): %q", content)
		}

		m.path = parts[0]
		m.backend = parts[1]
		u, err := url.Parse(parts[2])
		if err != nil {
			return fmt.Errorf("go import meta tag has invalid download url: %v", err)
		}
		m.dlRoot = *u
	}

	return nil
}

func (m *modmeta) listEndpoint() (*url.URL, error) {
	var empty url.URL
	if m.dlRoot == empty {
		return nil, fmt.Errorf("dl root is empty")
	}

	u := url.URL{
		Scheme: "https",
		Host:   m.dlRoot.Host,
		Path:   path.Join(m.dlRoot.Path, m.path, "@v", "list"),
	}
	return &u, nil
}

func (m *modmeta) fetchVersionList() ([]string, error) {
	u, err := m.listEndpoint()
	if err != nil {
		return nil, fmt.Errorf("unable to locate version list: %w", err)
	}

	log_debug.Printf("GET %s", u)
	res, err := http.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("unable to fetch version list: %w", err)
	}
	defer res.Body.Close()

	lines, err := parseVersionLines(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse version list: %w", err)
	}
	semver.Sort(lines)
	return lines, nil
}

func parseVersionLines(r io.Reader) ([]string, error) {
	lines := make([]string, 0, 8)
	keep := func(s string) error {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}

		if !semver.IsValid(s) {
			return fmt.Errorf("invalid version string: %s", s)
		}
		lines = append(lines, s)
		return nil
	}

	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return lines, keep(line)
			}
			return nil, fmt.Errorf("error reading version list response: %w", err)
		}
		if err := keep(line); err != nil {
			return nil, fmt.Errorf("bad version list: %w", err)
		}
	}
}

// nextMinor takes a sorted list of version strings and returns a string
// representing the next minor version
// func nextMinor(versions []string) string {
// 	last := "v0.0.0"
// 	semver.MajorMinor
// }
