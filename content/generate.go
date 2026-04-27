// Copyright (c) 2024 Warped Realms. All rights reserved.
// This source code is proprietary and confidential.
// Unauthorized copying or cloning of game mechanics is strictly prohibited.
// See LICENSE file in the project root for full license details.

package content

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"

	"warpedrealms/shared"
	"warpedrealms/world"
)

type GeneratedNPC struct {
	ID                string
	ProfileID         string
	DisguiseProfileID string
	Name              string
	RoomID            string
	Position          shared.Vec2
}

type GeneratedRaid struct {
	Layout       shared.RaidLayoutState
	PlayerSpawn  shared.Vec2   // primary spawn (index 0), kept for back-compat
	PlayerSpawns []shared.Vec2 // one entry per player slot; cycles on overflow
	NPCs         []GeneratedNPC
}

func (b *Bundle) GenerateRaid(seed int64) (*GeneratedRaid, error) {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	if len(b.RoomTemplates) == 0 {
		return nil, fmt.Errorf("no room templates loaded")
	}
	rng := rand.New(rand.NewSource(seed))
	roomCount := shared.GeneratedRoomCountMin + rng.Intn(shared.GeneratedRoomCountMax-shared.GeneratedRoomCountMin+1)

	layout := shared.RaidLayoutState{Seed: seed}
	generated := &GeneratedRaid{Layout: layout}

	type roomBuild struct {
		template RoomTemplate
		state    shared.RoomState
	}
	builds := make([]roomBuild, 0, roomCount)
	npcs := make([]GeneratedNPC, 0, roomCount*8)
	templateBag := make([]RoomTemplate, 0, len(b.RoomTemplates))

	driftX := 0.0
	npcIndex := 0

	for index := 0; index < roomCount; index++ {
		if len(templateBag) == 0 {
			templateBag = append(templateBag[:0], b.RoomTemplates...)
			rng.Shuffle(len(templateBag), func(i int, j int) {
				templateBag[i], templateBag[j] = templateBag[j], templateBag[i]
			})
			if len(builds) > 0 && len(templateBag) > 1 && templateBag[0].ID == builds[len(builds)-1].template.ID {
				templateBag[0], templateBag[1] = templateBag[1], templateBag[0]
			}
		}
		template := templateBag[0]
		templateBag = templateBag[1:]
		roomID := fmt.Sprintf("room-%02d", index+1)
		origin := shared.Vec2{
			X: driftX,
			Y: float64(index) * (shared.DefaultGeneratedRoomHeight - 680),
		}
		driftX = clampDrift(driftX + float64(rng.Intn(1400)-700) + math.Sin(float64(index)*0.9)*240)

		room := shared.RoomState{
			ID:           roomID,
			Name:         fmt.Sprintf("%s %d", template.Name, index+1),
			TemplateID:   template.ID,
			Biome:        template.Biome,
			Index:        index,
			BackgroundID: template.BackgroundID,
			TileStyleID:  template.TileStyleID,
			Bounds: shared.Rect{
				X: origin.X,
				Y: origin.Y,
				W: template.Size.X,
				H: template.Size.Y,
			},
		}

		for _, solid := range template.Solids {
			room.Solids = append(room.Solids, solid.Translate(origin))
		}
		room.Solids = append(room.Solids, proceduralPlatforms(room.Bounds, rng, index)...)
		for _, decoration := range template.Decorations {
			profile, ok := b.Manifest.Profile(decoration.ProfileID)
			if !ok {
				continue
			}
			scale := profile.Scale
			drawOffset := profile.SpriteOffset
			if decoration.Override.Scale != nil {
				scale = *decoration.Override.Scale
			}
			if decoration.Override.DrawOffset != nil {
				drawOffset = *decoration.Override.DrawOffset
			}
			room.Decorations = append(room.Decorations, shared.PlacedAssetState{
				ID:         fmt.Sprintf("%s-%s", roomID, decoration.ID),
				ProfileID:  decoration.ProfileID,
				RoomID:     roomID,
				Position:   origin.Add(decoration.Position),
				Scale:      scale,
				DrawOffset: drawOffset,
				Layer:      decoration.Layer,
				Bounds: shared.Rect{
					X: origin.X + decoration.Position.X + drawOffset.X,
					Y: origin.Y + decoration.Position.Y + drawOffset.Y,
					W: profile.SpriteSize.X * scale,
					H: profile.SpriteSize.Y * scale,
				},
			})
		}
		room.Decorations = append(room.Decorations, proceduralDecorations(b.Manifest, room, rng, index)...)

		for _, zone := range template.PvPZones {
			room.PvPZones = append(room.PvPZones, zone.Translate(origin))
		}
		for _, exit := range template.Exits {
			room.Exits = append(room.Exits, shared.ExitState{
				ID:     fmt.Sprintf("%s-%s", roomID, exit.ID),
				RoomID: roomID,
				Label:  exit.Label,
				Area:   exit.Area.Translate(origin),
			})
		}

		for _, mob := range template.Mobs {
			npcIndex++
			npcs = append(npcs, GeneratedNPC{
				ID:        fmt.Sprintf("npc-%03d", npcIndex),
				ProfileID: mob.ProfileID,
				Name:      fmt.Sprintf("%s %d", mob.ProfileID, npcIndex),
				RoomID:    roomID,
				Position:  origin.Add(mob.Position),
			})
		}
		for _, mob := range proceduralMobSpawns(template, room, rng) {
			npcIndex++
			npcs = append(npcs, GeneratedNPC{
				ID:        fmt.Sprintf("npc-%03d", npcIndex),
				ProfileID: mob.ProfileID,
				Name:      fmt.Sprintf("%s %d", mob.ProfileID, npcIndex),
				RoomID:    roomID,
				Position:  mob.Position,
			})
		}
		for _, mimic := range template.Mimics {
			npcIndex++
			npcs = append(npcs, GeneratedNPC{
				ID:                fmt.Sprintf("mimic-%03d", npcIndex),
				ProfileID:         mimic.CombatProfileID,
				DisguiseProfileID: mimic.DisguiseProfileID,
				Name:              fmt.Sprintf("Mimic %d", npcIndex),
				RoomID:            roomID,
				Position:          origin.Add(mimic.Position),
			})
		}
		for _, mimic := range proceduralMimics(room, rng) {
			npcIndex++
			npcs = append(npcs, GeneratedNPC{
				ID:                fmt.Sprintf("mimic-%03d", npcIndex),
				ProfileID:         mimic.ProfileID,
				DisguiseProfileID: mimic.DisguiseProfileID,
				Name:              fmt.Sprintf("Mimic %d", npcIndex),
				RoomID:            roomID,
				Position:          mimic.Position,
			})
		}
		npcIndex++
		npcs = append(npcs, GeneratedNPC{
			ID:        fmt.Sprintf("boss-%03d", npcIndex),
			ProfileID: template.Boss.ProfileID,
			Name:      fmt.Sprintf("Boss %d", index+1),
			RoomID:    roomID,
			Position:  origin.Add(template.Boss.Position),
		})

		builds = append(builds, roomBuild{
			template: template,
			state:    room,
		})
	}

	for index := range builds {
		current := &builds[index]
		if index > 0 {
			current.state.AboveRoomID = builds[index-1].state.ID
		}
		if index+1 < len(builds) {
			current.state.BelowRoomID = builds[index+1].state.ID
		}

		for _, link := range current.template.JumpLinks {
			targetIndex := index
			switch link.TargetTag {
			case "up":
				targetIndex = max(0, index-1)
			case "down":
				targetIndex = min(len(builds)-1, index+1)
			default:
				if index+1 < len(builds) {
					targetIndex = index + 1
				}
			}
			targetRoom := builds[targetIndex].state
			current.state.JumpLinks = append(current.state.JumpLinks, shared.JumpLinkState{
				ID:           fmt.Sprintf("%s-%s", current.state.ID, link.ID),
				RoomID:       current.state.ID,
				TargetRoomID: targetRoom.ID,
				Label:        link.Label,
				Area:         link.Area.Translate(shared.Vec2{X: current.state.Bounds.X, Y: current.state.Bounds.Y}),
				Arrival: shared.Vec2{
					X: targetRoom.Bounds.X + link.Arrival.X,
					Y: targetRoom.Bounds.Y + link.Arrival.Y,
				},
				PreviewRect: link.PreviewRect.Translate(shared.Vec2{X: current.state.Bounds.X, Y: current.state.Bounds.Y}),
			})
		}
		for _, zone := range current.template.RevealZones {
			targetRoomID := current.state.ID
			switch zone.TargetTag {
			case "up":
				if index > 0 {
					targetRoomID = builds[index-1].state.ID
				}
			case "down":
				if index+1 < len(builds) {
					targetRoomID = builds[index+1].state.ID
				}
			}
			current.state.RevealZones = append(current.state.RevealZones, shared.RevealZoneState{
				ID:           fmt.Sprintf("%s-%s", current.state.ID, zone.ID),
				RoomID:       current.state.ID,
				TargetRoomID: targetRoomID,
				Area:         zone.Area.Translate(shared.Vec2{X: current.state.Bounds.X, Y: current.state.Bounds.Y}),
			})
		}
	}

	for _, build := range builds {
		generated.Layout.Rooms = append(generated.Layout.Rooms, build.state)
	}
	sort.Slice(generated.Layout.Rooms, func(i int, j int) bool {
		return generated.Layout.Rooms[i].Index < generated.Layout.Rooms[j].Index
	})
	annotateRoomsWithSessionRings(generated.Layout.Rooms)
	generated.NPCs = npcs
	generated.PlayerSpawn = shared.Vec2{
		X: builds[0].state.Bounds.X + builds[0].template.PlayerSpawn.X,
		Y: builds[0].state.Bounds.Y + builds[0].template.PlayerSpawn.Y,
	}
	return generated, nil
}

// GenerateRaidFromLDtk builds a raid from a .ldtk project file.
// Each level becomes one room.  Collision comes from IntGrid layers;
// Entities define spawns and portal connections.
func (b *Bundle) GenerateRaidFromLDtk(ldtkPath string) (*GeneratedRaid, error) {
	maps, err := world.LoadLDtk(ldtkPath)
	if err != nil {
		return nil, fmt.Errorf("load ldtk: %w", err)
	}
	if len(maps) == 0 {
		return nil, fmt.Errorf("ldtk file %s has no levels", ldtkPath)
	}
	return b.buildRaidFromMaps(maps)
}

// buildRaidFromMaps converts pre-loaded MapData slices to a GeneratedRaid.
//
// Graph topology: each room occupies its own local coordinate space
// (Bounds always starts at {0,0}).  Jump links use explicit room IDs
// (e.g. "room_02") written by the procgen; relative "above"/"below"
// targets are still supported for hand-authored LDtk files.
func (b *Bundle) buildRaidFromMaps(maps []*world.MapData) (*GeneratedRaid, error) {
	rooms := make([]shared.RoomState, len(maps))
	for i, m := range maps {
		rooms[i] = shared.RoomState{
			ID:           fmt.Sprintf("room-%02d", i+1),
			Name:         fmt.Sprintf("Room %d", i+1),
			TemplateID:   m.ID,
			Biome:        "cave",
			Index:        i,
			Bounds:       shared.Rect{W: float64(m.PixelWidth), H: float64(m.PixelHeight)},
			BackgroundID: "cave",
			TileStyleID:  "cave",
			Solids:       append([]shared.Rect(nil), m.SolidRects...),
			Platforms:    append([]shared.Rect(nil), m.PlatformRects...),
		}
	}

	// Build a lookup: MapData.ID → rooms index.
	mapIDToRoom := make(map[string]int, len(maps))
	for i, m := range maps {
		mapIDToRoom[m.ID] = i
		// Also register the canonical room ID "room-02" form.
		mapIDToRoom[rooms[i].ID] = i
		// Register underscore variant written by procgen ("room_02").
		underscoreID := fmt.Sprintf("room_%02d", i+1)
		mapIDToRoom[underscoreID] = i
	}

	resolveTarget := func(current int, target string) int {
		if idx, ok := mapIDToRoom[target]; ok {
			return idx
		}
		switch target {
		case "above", "up":
			if current > 0 {
				return current - 1
			}
		case "below", "down":
			if current+1 < len(rooms) {
				return current + 1
			}
		}
		return current
	}

	// Build JumpLinks and RevealZones.
	npcIndex := 0
	npcs := make([]GeneratedNPC, 0, len(maps)*4)

	for i, m := range maps {
		for j, link := range m.JumpLinks {
			targetIdx := resolveTarget(i, link.Target)
			targetRoom := rooms[targetIdx]

			// Arrival is in the target room's local coordinate space (origin 0,0).
			var arrival shared.Vec2
			if link.HasArrival {
				arrival = shared.Vec2{X: link.ArrivalX, Y: link.ArrivalY}
			} else if len(maps[targetIdx].PlayerSpawns) > 0 {
				arrival = maps[targetIdx].DefaultPlayerSpawn(0)
			} else {
				arrival = targetRoom.Bounds.Center()
			}

			var previewRect shared.Rect
			if link.HasPreview {
				previewRect = shared.Rect{
					X: link.PreviewX, Y: link.PreviewY,
					W: link.PreviewW, H: link.PreviewH,
				}
			} else {
				previewRect = shared.Rect{
					X: link.Area.X - 160, Y: link.Area.Y - 180,
					W: 320, H: 180,
				}
			}

			rooms[i].JumpLinks = append(rooms[i].JumpLinks, shared.JumpLinkState{
				ID:           fmt.Sprintf("%s-link-%02d", rooms[i].ID, j+1),
				RoomID:       rooms[i].ID,
				TargetRoomID: targetRoom.ID,
				Label:        link.Label,
				Area:         link.Area, // already in local coords
				Arrival:      arrival,
				PreviewRect:  previewRect,
			})
		}

		for j, zone := range m.RevealZones {
			targetIdx := resolveTarget(i, zone.Target)
			rooms[i].RevealZones = append(rooms[i].RevealZones, shared.RevealZoneState{
				ID:           fmt.Sprintf("%s-reveal-%02d", rooms[i].ID, j+1),
				RoomID:       rooms[i].ID,
				TargetRoomID: rooms[targetIdx].ID,
				Area:         zone.Area,
			})
		}

		for j, rift := range m.Rifts {
			targetIdx := resolveTarget(i, rift.Target)
			cap := shared.RiftCapacity(shared.RiftKind(rift.Kind))
			var arrival shared.Vec2
			if rift.HasArrival {
				arrival = shared.Vec2{X: rift.ArrivalX, Y: rift.ArrivalY}
			} else {
				arrival = rooms[targetIdx].Bounds.Center()
			}
			rooms[i].Rifts = append(rooms[i].Rifts, shared.RiftState{
				ID:           fmt.Sprintf("%s-rift-%02d", rooms[i].ID, j+1),
				RoomID:       rooms[i].ID,
				TargetRoomID: rooms[targetIdx].ID,
				Area:         rift.Area,
				Arrival:      arrival,
				Kind:         shared.RiftKind(rift.Kind),
				Capacity:     cap,
				UsedCount:    0,
			})
		}

		// Rat NPCs from entity spawns.
		for _, spawn := range m.RatSpawns {
			npcIndex++
			npcs = append(npcs, GeneratedNPC{
				ID:        fmt.Sprintf("npc-%03d", npcIndex),
				ProfileID: "mob_rat",
				Name:      fmt.Sprintf("Rat %d", npcIndex),
				RoomID:    rooms[i].ID,
				Position:  spawn,
			})
		}
	}

	// Player spawns: collect all Player entities from the first map.
	// Each entry becomes an independent spawn slot; spawnPosition cycles through them.
	playerSpawn := rooms[0].Bounds.Center()
	var playerSpawns []shared.Vec2
	if len(maps[0].PlayerSpawns) > 0 {
		playerSpawn = maps[0].DefaultPlayerSpawn(0)
		playerSpawns = append(playerSpawns, maps[0].PlayerSpawns...)
	}
	if len(playerSpawns) == 0 {
		playerSpawns = []shared.Vec2{playerSpawn}
	}

	sort.Slice(rooms, func(i, j int) bool { return rooms[i].Index < rooms[j].Index })
	annotateRoomsWithSessionRings(rooms)

	return &GeneratedRaid{
		Layout:       shared.RaidLayoutState{Rooms: rooms, PlayerSpawns: playerSpawns},
		PlayerSpawn:  playerSpawn,
		PlayerSpawns: playerSpawns,
		NPCs:         npcs,
	}, nil
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampDrift(value float64) float64 {
	if value < -2600 {
		return -2600
	}
	if value > 2600 {
		return 2600
	}
	return value
}

func proceduralPlatforms(bounds shared.Rect, rng *rand.Rand, roomIndex int) []shared.Rect {
	count := 4 + rng.Intn(5)
	platforms := make([]shared.Rect, 0, count)
	stepX := (bounds.W - 2400) / float64(count+1)
	for index := 0; index < count; index++ {
		width := 760 + float64(rng.Intn(880))
		x := bounds.X + 800 + stepX*float64(index+1) + float64(rng.Intn(380)-190)
		yBand := 980 + float64((index+roomIndex)%4)*560
		y := bounds.Y + yBand + float64(rng.Intn(220)-110)
		platforms = append(platforms, shared.Rect{
			X: x,
			Y: y,
			W: width,
			H: 92 + float64(rng.Intn(32)),
		})
	}
	return platforms
}

func proceduralDecorations(manifest *Manifest, room shared.RoomState, rng *rand.Rand, roomIndex int) []shared.PlacedAssetState {
	assets := make([]shared.PlacedAssetState, 0, 6)
	if profile, ok := manifest.Profile("decor_bridge"); ok {
		for index := 0; index < 2; index++ {
			scale := profile.Scale * (0.9 + rng.Float64()*0.25)
			position := shared.Vec2{
				X: room.Bounds.X + 1100 + float64(index)*((room.Bounds.W-3600)*0.7) + float64(rng.Intn(520)),
				Y: room.Bounds.Y + room.Bounds.H - 940 - float64(index*640) - float64(rng.Intn(220)),
			}
			assets = append(assets, shared.PlacedAssetState{
				ID:         fmt.Sprintf("%s-variant-bridge-%d", room.ID, index),
				ProfileID:  profile.ID,
				RoomID:     room.ID,
				Position:   position,
				Scale:      scale,
				DrawOffset: profile.SpriteOffset,
				Layer:      "foreground",
				Alpha:      0.9,
				Bounds: shared.Rect{
					X: position.X + profile.SpriteOffset.X,
					Y: position.Y + profile.SpriteOffset.Y,
					W: profile.SpriteSize.X * scale,
					H: profile.SpriteSize.Y * scale,
				},
			})
		}
	}
	if profile, ok := manifest.Profile("decor_window"); ok {
		for index := 0; index < 2; index++ {
			scale := profile.Scale * (0.82 + rng.Float64()*0.22)
			position := shared.Vec2{
				X: room.Bounds.X + room.Bounds.W - 2800 + float64(index)*620 + float64(rng.Intn(180)),
				Y: room.Bounds.Y + 720 + float64((roomIndex+index)%3)*420,
			}
			assets = append(assets, shared.PlacedAssetState{
				ID:         fmt.Sprintf("%s-variant-window-%d", room.ID, index),
				ProfileID:  profile.ID,
				RoomID:     room.ID,
				Position:   position,
				Scale:      scale,
				DrawOffset: profile.SpriteOffset,
				Layer:      "midground",
				Alpha:      0.7,
				Bounds: shared.Rect{
					X: position.X + profile.SpriteOffset.X,
					Y: position.Y + profile.SpriteOffset.Y,
					W: profile.SpriteSize.X * scale,
					H: profile.SpriteSize.Y * scale,
				},
			})
		}
	}
	return assets
}

func proceduralMobSpawns(template RoomTemplate, room shared.RoomState, rng *rand.Rand) []GeneratedNPC {
	pools := map[string][]string{
		"forest": {"mob_rat", "mob_frog", "mob_bee"},
		"ruins":  {"mob_rat", "mob_bat", "mob_spider"},
		"vault":  {"mob_bee", "mob_bat", "mob_spider", "mob_frog"},
	}
	pool := pools[template.Biome]
	if len(pool) == 0 {
		pool = []string{"mob_rat", "mob_bat", "mob_spider"}
	}
	count := 2 + rng.Intn(3)
	extra := make([]GeneratedNPC, 0, count)
	for index := 0; index < count; index++ {
		extra = append(extra, GeneratedNPC{
			ProfileID: pool[rng.Intn(len(pool))],
			RoomID:    room.ID,
			Position: shared.Vec2{
				X: room.Bounds.X + 1400 + float64(index)*((room.Bounds.W-3200)/float64(count+1)) + float64(rng.Intn(420)-210),
				Y: room.Bounds.Y + room.Bounds.H - 640 - float64(rng.Intn(1400)),
			},
		})
	}
	return extra
}

func proceduralMimics(room shared.RoomState, rng *rand.Rand) []GeneratedNPC {
	if rng.Float64() < 0.4 {
		return nil
	}
	count := 1 + rng.Intn(2)
	extra := make([]GeneratedNPC, 0, count)
	for index := 0; index < count; index++ {
		extra = append(extra, GeneratedNPC{
			ProfileID:         "mimic_trueform",
			DisguiseProfileID: "mimic_disguise",
			RoomID:            room.ID,
			Position: shared.Vec2{
				X: room.Bounds.X + room.Bounds.W*0.2 + float64(index)*room.Bounds.W*0.28 + float64(rng.Intn(280)-140),
				Y: room.Bounds.Y + room.Bounds.H - 560 - float64(rng.Intn(160)),
			},
		})
	}
	return extra
}
