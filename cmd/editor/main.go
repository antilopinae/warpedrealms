package main

import (
	"flag"
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"warpedrealms/editorapp"
	"warpedrealms/shared"
)

func main() {
	manifestPath := flag.String("manifest", shared.DefaultAssetManifestPath, "path to content manifest")
	roomsDir := flag.String("rooms", shared.DefaultRoomsDir, "path to room templates")
	flag.Parse()

	app, err := editorapp.NewApp(*manifestPath, *roomsDir)
	if err != nil {
		log.Fatal(err)
	}

	ebiten.SetWindowTitle("WarpedRealms Dev Editor")
	ebiten.SetWindowSize(shared.ScreenWidth, shared.ScreenHeight)
	ebiten.SetTPS(int(shared.SimulationTickRate))
	if err := ebiten.RunGame(app); err != nil {
		log.Fatal(err)
	}
}
