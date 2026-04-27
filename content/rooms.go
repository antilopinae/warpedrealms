// Copyright (c) 2024 Warped Realms. All rights reserved.
// This source code is proprietary and confidential.
// Unauthorized copying or cloning of game mechanics is strictly prohibited.
// See LICENSE file in the project root for full license details.

package content

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"warpedrealms/shared"
)

type InstanceOverride struct {
	Scale          *float64     `json:"scale,omitempty"`
	DrawOffset     *shared.Vec2 `json:"draw_offset,omitempty"`
	Collider       *shared.Rect `json:"collider,omitempty"`
	Hurtbox        *shared.Rect `json:"hurtbox,omitempty"`
	InteractionBox *shared.Rect `json:"interaction_box,omitempty"`
}

type PlacedAsset struct {
	ID        string           `json:"id"`
	ProfileID string           `json:"profile_id"`
	Position  shared.Vec2      `json:"position"`
	Layer     string           `json:"layer,omitempty"`
	Override  InstanceOverride `json:"override,omitempty"`
}

type MobSpawn struct {
	ProfileID string      `json:"profile_id"`
	Position  shared.Vec2 `json:"position"`
}

type MimicSpawn struct {
	DisguiseProfileID string      `json:"disguise_profile_id"`
	CombatProfileID   string      `json:"combat_profile_id"`
	Position          shared.Vec2 `json:"position"`
}

type BossSpawn struct {
	ProfileID string      `json:"profile_id"`
	Position  shared.Vec2 `json:"position"`
}

type LootSpawn struct {
	ProfileID string      `json:"profile_id"`
	Kind      string      `json:"kind"`
	Position  shared.Vec2 `json:"position"`
	Value     int         `json:"value"`
}

type JumpLinkTemplate struct {
	ID          string      `json:"id"`
	TargetTag   string      `json:"target_tag"`
	Label       string      `json:"label"`
	Area        shared.Rect `json:"area"`
	Arrival     shared.Vec2 `json:"arrival"`
	PreviewRect shared.Rect `json:"preview_rect"`
}

type RevealZoneTemplate struct {
	ID        string      `json:"id"`
	TargetTag string      `json:"target_tag"`
	Area      shared.Rect `json:"area"`
}

type ExitTemplate struct {
	ID    string      `json:"id"`
	Label string      `json:"label"`
	Area  shared.Rect `json:"area"`
}

type RoomTemplate struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	Biome        string               `json:"biome"`
	BackgroundID string               `json:"background_id"`
	TileStyleID  string               `json:"tile_style_id"`
	Size         shared.Vec2          `json:"size"`
	PlayerSpawn  shared.Vec2          `json:"player_spawn"`
	Solids       []shared.Rect        `json:"solids"`
	Decorations  []PlacedAsset        `json:"decorations,omitempty"`
	JumpLinks    []JumpLinkTemplate   `json:"jump_links"`
	RevealZones  []RevealZoneTemplate `json:"reveal_zones,omitempty"`
	PvPZones     []shared.Rect        `json:"pvp_zones,omitempty"`
	Exits        []ExitTemplate       `json:"exits,omitempty"`
	Mobs         []MobSpawn           `json:"mobs,omitempty"`
	Mimics       []MimicSpawn         `json:"mimics,omitempty"`
	Boss         BossSpawn            `json:"boss"`
	Loot         []LootSpawn          `json:"loot,omitempty"`
}

func LoadRoomTemplates(dir string) ([]RoomTemplate, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read rooms dir: %w", err)
	}
	templates := make([]RoomTemplate, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read room template %s: %w", entry.Name(), err)
		}
		var template RoomTemplate
		if err := json.Unmarshal(raw, &template); err != nil {
			return nil, fmt.Errorf("decode room template %s: %w", entry.Name(), err)
		}
		if template.ID == "" {
			template.ID = entry.Name()[:len(entry.Name())-len(filepath.Ext(entry.Name()))]
		}
		if template.Size.X == 0 {
			template.Size.X = shared.DefaultGeneratedRoomWidth
		}
		if template.Size.Y == 0 {
			template.Size.Y = shared.DefaultGeneratedRoomHeight
		}
		if len(template.JumpLinks) == 0 {
			return nil, fmt.Errorf("room template %s requires jump links", template.ID)
		}
		if template.Boss.ProfileID == "" {
			return nil, fmt.Errorf("room template %s requires boss spawn", template.ID)
		}
		templates = append(templates, template)
	}
	sort.Slice(templates, func(i int, j int) bool { return templates[i].ID < templates[j].ID })
	if len(templates) == 0 {
		return nil, fmt.Errorf("no room templates found in %s", dir)
	}
	return templates, nil
}

func SaveRoomTemplate(path string, template RoomTemplate) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}
