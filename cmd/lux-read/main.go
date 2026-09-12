package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/legion/display/internal/ambient"
)

func main() {
	port := flag.String("port", "", "serial device (default: auto-detect)")
	baud := flag.Int("baud", 57600, "serial baud rate")
	timeout := flag.Duration("timeout", 5*time.Second, "read timeout")
	flag.Parse()

	portName := *port
	if portName == "" {
		var err error
		portName, err = ambient.DetectPort()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	lux, err := ambient.ReadLux(portName, *baud, *timeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lux-read: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(lux)
}
