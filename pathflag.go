package main

import (
	"fmt"
	"os"
)

type pathArg struct {
	path string
}

func (p *pathArg) Set(s string) error {
	_, err := os.Stat(s)
	if err != nil {
		return fmt.Errorf("bad path arg: %w", err)
	}
	p.path = s
	return nil
}

func (p pathArg) String() string { return p.path }
