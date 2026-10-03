package main

import (
	"flag"
	"fmt"
	"os"

	"srv.exe.dev/srv"
)

var flagListenAddr = flag.String("listen", ":8000", "address to listen on")
var flagReseed = flag.String("reseed-treasures", "", "re-place the unfound treasures of a session (id or invite code) and exit")

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

func run() error {
	flag.Parse()
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	server, err := srv.New("db.sqlite3", hostname)
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}
	if *flagReseed != "" {
		return server.ReseedTreasures(*flagReseed)
	}
	return server.Serve(*flagListenAddr)
}
