package content

import (
	"testing"

	"warpedrealms/shared"
)

func TestManifestClassLookupAndDefaultPlayerClass(t *testing.T) {
	manifest := &Manifest{
		Profiles: map[string]AssetProfile{
			"player_knight": {ID: "player_knight"},
			"player_archer": {ID: "player_archer"},
		},
		Classes: []ClassDefinition{
			{ID: shared.PlayerClassKnight, ProfileID: "player_knight"},
			{ID: shared.PlayerClassArcherAssassin, ProfileID: "player_archer"},
		},
	}
	manifest.Normalize()

	classDef, ok := manifest.Class(shared.PlayerClassArcherAssassin)
	if !ok {
		t.Fatal("expected class lookup by ID to succeed")
	}
	if classDef.ProfileID != "player_archer" {
		t.Fatalf("expected player_archer profile, got %s", classDef.ProfileID)
	}

	byProfile, ok := manifest.ClassByProfileID("player_knight")
	if !ok {
		t.Fatal("expected class lookup by profile ID to succeed")
	}
	if byProfile.ID != shared.PlayerClassKnight {
		t.Fatalf("expected %s, got %s", shared.PlayerClassKnight, byProfile.ID)
	}

	defaultClass, ok := manifest.DefaultPlayerClass()
	if !ok {
		t.Fatal("expected default player class")
	}
	if defaultClass.ID != shared.PlayerClassKnight {
		t.Fatalf("expected default class %s, got %s", shared.PlayerClassKnight, defaultClass.ID)
	}

	profile, ok := manifest.Profile("player_knight")
	if !ok {
		t.Fatal("expected player_knight profile")
	}
	if profile.ClassID != shared.PlayerClassKnight {
		t.Fatalf("expected normalized profile class %s, got %s", shared.PlayerClassKnight, profile.ClassID)
	}
}
