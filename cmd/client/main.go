// Copyright (c) 2024 Warped Realms. All rights reserved.
// This source code is proprietary and confidential.
// Unauthorized copying or cloning of game mechanics is strictly prohibited.
// See LICENSE file in the project root for full license details.

package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/hajimehoshi/ebiten/v2"

	"warpedrealms/clientapp"
	"warpedrealms/shared"

	_ "net/http"
	_ "net/http/pprof"
)

func main() {
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	serverURL := flag.String("server", "http://127.0.0.1:8080", "server base url")
	flag.Parse()

	var manifestPath = shared.DefaultAssetManifestPath
	roomsDir := shared.DefaultRoomsDir

	game, err := clientapp.NewGame(*serverURL, manifestPath, roomsDir)

	if err != nil {

		log.Fatal(err)
	}

	ebiten.SetWindowTitle("Warped Realms")
	ebiten.SetWindowSize(shared.ScreenWidth, shared.ScreenHeight)
	ebiten.SetTPS(int(shared.SimulationTickRate))

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}
