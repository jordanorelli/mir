package main

import (
	"sync"
	"os"
)

var shutdownHandlers []func() error
var shutdownOnce sync.Once

func shutdown(cause error) {
	shutdownOnce.Do(func() {
		status := 0
		if cause != nil {
			status = 1
			log_error.Printf("shutting down due to error: %v", cause)
		} else {
			log_info.Print("shutting down")
		}
		for _, f := range shutdownHandlers {
			if err := f(); err != nil {
				log_error.Printf("error in shutdown: %v", err)
			}
		}
		os.Exit(status)
	})
}

func onShutdown(f func() error) {
	shutdownHandlers = append(shutdownHandlers, f)
}
