// Copyright (c) 2024 Warped Realms. All rights reserved.
// This source code is proprietary and confidential.
// Unauthorized copying or cloning of game mechanics is strictly prohibited.
// See LICENSE file in the project root for full license details.

package main

import (
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"

	"warpedrealms/serverapp"
	"warpedrealms/shared"
)

func main() {
	var (
		addr     = flag.String("addr", ":8080", "http listen address")
		authDB   = flag.String("auth", "data/users.json", "path to user database")
		manifest = flag.String("manifest", shared.DefaultAssetManifestPath, "path to content manifest")
		roomsDir = flag.String("rooms", shared.DefaultRoomsDir, "path to room templates")
	)
	flag.Parse()

	go func() {
		log.Println(http.ListenAndServe("localhost:6061", nil))
	}()

	server, err := serverapp.NewServer(*addr, *authDB, *manifest, *roomsDir)
	if err != nil {
		log.Fatal(err)
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
