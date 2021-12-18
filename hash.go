package main

import (
	"flag"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// pwhashcmd is just a thing for generating bcrypt hashes for passwords. This
// is like using htpasswd from the apache-utils package but honestly adding
// that whole package to a system to compute a single bcrypt hash is ridiculous
func pwhashcmd(args []string) {
	cost := bcrypt.DefaultCost

	flags := flag.NewFlagSet("pwhash", flag.ExitOnError)
	flags.IntVar(&cost, "cost", cost, "bcrypt cost difficulty")
	flags.Parse(args)

	for _, pw := range flags.Args() {
		hash, err := bcrypt.GenerateFromPassword([]byte(pw), cost)
		if err != nil {
			bail(1, "hash failed: %v", err)
		}
		fmt.Println(string(hash))
	}
}
