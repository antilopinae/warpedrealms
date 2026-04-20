package main

import (
	"flag"
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"warpedrealms/clientapp"
	"warpedrealms/shared"
)

func main() {
	serverURL := flag.String("server", "http://127.0.0.1:8080", "server base url")
	manifestPath := flag.String("manifest", shared.DefaultAssetManifestPath, "path to content manifest")
	roomsDir := flag.String("rooms", shared.DefaultRoomsDir, "path to room templates")
	flag.Parse()

	game, err := clientapp.NewGame(*serverURL, *manifestPath, *roomsDir)
	if err != nil {
		log.Fatal(err)
	}

	ebiten.SetWindowTitle("WarpedRealms Go Client")
	ebiten.SetWindowSize(shared.ScreenWidth, shared.ScreenHeight)
	ebiten.SetTPS(int(shared.SimulationTickRate))
	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}
