package main

import (
	"flag"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func serve(args []string) {
	// listen on this unix domain socket
	var socketPath string

	// serve modules out of this root directory
	rootDir := "/srv/mir"

	// serve module traffic on this hostname
	var hostname string
	var httpAddr string
	auth := make(authUsers)

	serveFlags := flag.NewFlagSet("serve", flag.ExitOnError)
	serveFlags.StringVar(&socketPath, "unix", socketPath, "path for a unix domain socket to listen on")
	serveFlags.StringVar(&httpAddr, "http", httpAddr, "http address to listen on")
	serveFlags.StringVar(&rootDir, "root", rootDir, "root directory for module storage")
	serveFlags.StringVar(&hostname, "hostname", hostname, "domain name on which mir serves modules")
	serveFlags.Var(&auth, "auth-users", "comma-separated list of usernames and bcrypt password hashes")
	serveFlags.Parse(args)

	h := handler{
		socketPath: socketPath,
		httpAddr:   httpAddr,
		root:       rootDir,
		hostname:   hostname,
		auth:       auth,
	}
	if err := h.run(); err != nil {
		bail(1, err.Error())
	}
}

type authUsers map[string]string

func (a authUsers) String() string {
	if len(a) == 0 {
		return ""
	}

	var b strings.Builder
	for k, v := range a {
		fmt.Fprintf(&b, "%s:%s,", k, v)
	}
	s := b.String()
	return s[:len(s)-1]
}

func (a authUsers) Set(v string) error {
	pairs := strings.Split(v, ",")
	if len(pairs) == 0 {
		return fmt.Errorf("auth users string cannot be empty")
	}

	// Each pair is a colon-delimited username-hash pair
	// username:$2a$10$8KTGhnP8Myh62wjdOqCsiO.zE.i9FQ1Y0PD9lfpvgR7GLtIbbcteG
	for _, pair := range pairs {
		parts := strings.Split(pair, ":")
		if len(parts) != 2 {
			return fmt.Errorf("invalid user/hash pair: %s", pair)
		}

		// check the cost to ensure it's a valid bcrypt hash
		if _, err := bcrypt.Cost([]byte(parts[1])); err != nil {
			return fmt.Errorf("invalid hash %q: %v", parts[1], err)
		}
		a[parts[0]] = parts[1]
	}

	return nil
}
