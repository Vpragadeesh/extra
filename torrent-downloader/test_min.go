//go:build ignore

// Standalone scratch experiment (not part of the main package build).
// Run it explicitly with:  go run test_min.go
package main

import (
	"log"

	"github.com/anacrolix/torrent"
)

func main() {
	cfg := torrent.NewDefaultClientConfig()
	cfg.ListenPort = 0
	cl, err := torrent.NewClient(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer cl.Close()
	t, _ := cl.AddMagnet("magnet:?xt=urn:btih:3b245504cf5f11bbdbe1201cea6a6bf45aee1bc0&dn=ubuntu-24.04-desktop-amd64.iso")
	<-t.GotInfo()
	log.Println("Got info!")
}
