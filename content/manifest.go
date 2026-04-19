package content

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"warpedrealms/shared"
)

type Bundle struct {
	Manifest      *Manifest
	RoomTemplates []RoomTemplate
}

type Manifest struct {
	Version     string                          `json:"version"`
	Profiles    map[string]AssetProfile         `json:"profiles"`
	Classes     []ClassDefinition               `json:"classes"`
	Backgrounds map[string]BackgroundDefinition `json:"backgrounds"`
	TileStyles  map[string]TileStyle            `json:"tile_styles"`
	UISkin      UISkin                          `json:"ui"`
}

type AnimationClip struct {
	State         shared.AnimationState `json:"state"`
	Pattern       string                `json:"pattern"`
	FrameDuration float64               `json:"frame_duration"`
	Loop          bool                  `json:"loop"`
}

type HitboxFrame struct {
	Start float64     `json:"start"`
	End   float64     `json:"end"`
	Box   shared.Rect `json:"box"`
}

type AnimationHitbox struct {
	State  shared.AnimationState `json:"state"`
	Frames []HitboxFrame         `json:"frames"`
}

type CompositePart struct {
	Path   string      `json:"path"`
	Offset shared.Vec2 `json:"offset"`
	Scale  float64     `json:"scale,omitempty"`
}

type CompositeProfile struct {
	Body     CompositePart `json:"body"`
	LeftArm  CompositePart `json:"left_arm"`
	RightArm CompositePart `json:"right_arm"`
	LeftLeg  CompositePart `json:"left_leg"`
	RightLeg CompositePart `json:"right_leg"`
	Eyes     CompositePart `json:"eyes"`
	Mouth    CompositePart `json:"mouth"`
	Detail   CompositePart `json:"detail"`
}

type AssetProfile struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Kind           shared.EntityKind  `json:"kind"`
	Faction        shared.Faction     `json:"faction"`
	FamilyID       string             `json:"family_id"`
	ClassID        shared.PlayerClass `json:"class_id,omitempty"`
	Scale          float64            `json:"scale"`
	SpriteSize     shared.Vec2        `json:"sprite_size"`
	SpriteOffset   shared.Vec2        `json:"sprite_offset"`
	Collider       shared.Rect        `json:"collider"`
	Hurtbox        shared.Rect        `json:"hurtbox"`
	InteractionBox shared.Rect        `json:"interaction_box"`
	SortAnchor     shared.Vec2        `json:"sort_anchor"`
	MaxHP          int                `json:"max_hp"`
	Moveset        string             `json:"moveset,omitempty"`
	Animations     []AnimationClip    `json:"animations,omitempty"`
	Hitboxes       []AnimationHitbox  `json:"hitboxes,omitempty"`
	Layers         []CompositePart    `json:"layers,omitempty"`
	Composite      *CompositeProfile  `json:"composite,omitempty"`
}

type AbilityDefinition struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Cooldown    float64 `json:"cooldown"`
}

type ClassDefinition struct {
	ID          shared.PlayerClass  `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	ProfileID   string              `json:"profile_id"`
	WeaponPath  string              `json:"weapon_path,omitempty"`
	Skills      []AbilityDefinition `json:"skills"`
}

type BackgroundLayer struct {
	Path     string  `json:"path"`
	Parallax float64 `json:"parallax"`
	Alpha    float64 `json:"alpha"`
	OffsetY  float64 `json:"offset_y,omitempty"`
}

type BackgroundDefinition struct {
	ID     string            `json:"id"`
	Base   string            `json:"base"`
	Layers []BackgroundLayer `json:"layers,omitempty"`
}

type TileStyle struct {
	ID          string `json:"id"`
	Floor       string `json:"floor"`
	Platform    string `json:"platform"`
	Accent      string `json:"accent,omitempty"`
	JumpPreview string `json:"jump_preview,omitempty"`
}

type UISkin struct {
	Panel      string `json:"panel,omitempty"`
	Button     string `json:"button,omitempty"`
	Selection  string `json:"selection,omitempty"`
	Heart      string `json:"heart,omitempty"`
	HeartHalf  string `json:"heart_half,omitempty"`
	HeartEmpty string `json:"heart_empty,omitempty"`
}

func LoadBundle(manifestPath string, roomsDir string) (*Bundle, error) {
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	rooms, err := LoadRoomTemplates(roomsDir)
	if err != nil {
		return nil, err
	}
	bundle := &Bundle{
		Manifest:      manifest,
		RoomTemplates: rooms,
	}
	if err := bundle.Validate(); err != nil {
		return nil, err
	}
	return bundle, nil
}

func LoadManifest(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	manifest.Normalize()
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func SaveManifest(path string, manifest *Manifest) error {
	if manifest == nil {
		return fmt.Errorf("manifest is nil")
	}
	manifest.Normalize()
	if err := manifest.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func (m *Manifest) Normalize() {
	if m.Profiles == nil {
		m.Profiles = make(map[string]AssetProfile)
	}
	for id, profile := range m.Profiles {
		profile.ID = id
		if profile.Name == "" {
			profile.Name = strings.ReplaceAll(strings.Title(strings.ReplaceAll(id, "_", " ")), "  ", " ")
		}
		if profile.Scale == 0 {
			profile.Scale = 1
		}
		if profile.Hurtbox.W == 0 || profile.Hurtbox.H == 0 {
			profile.Hurtbox = profile.Collider
		}
		if profile.InteractionBox.W == 0 || profile.InteractionBox.H == 0 {
			profile.InteractionBox = profile.Collider.Inflate(14, 12)
		}
		m.Profiles[id] = profile
	}
	for index := range m.Classes {
		if m.Classes[index].ID == "" {
			m.Classes[index].ID = shared.PlayerClass(strings.ToLower(strings.ReplaceAll(m.Classes[index].Name, " ", "_")))
		}
	}
	for id, background := range m.Backgrounds {
		background.ID = id
		m.Backgrounds[id] = background
	}
	for id, style := range m.TileStyles {
		style.ID = id
		m.TileStyles[id] = style
	}
}

func (m *Manifest) Validate() error {
	if len(m.Classes) < 3 {
		return fmt.Errorf("manifest requires at least 3 classes")
	}
	for _, class := range m.Classes {
		if _, ok := m.Profiles[class.ProfileID]; !ok {
			return fmt.Errorf("class %s references missing profile %s", class.ID, class.ProfileID)
		}
		if len(class.Skills) < 3 {
			return fmt.Errorf("class %s requires 3 skills", class.ID)
		}
	}
	if len(m.Profiles) == 0 {
		return fmt.Errorf("manifest requires profiles")
	}
	return nil
}

func (b *Bundle) Validate() error {
	if b.Manifest == nil {
		return fmt.Errorf("bundle manifest is nil")
	}
	if len(b.RoomTemplates) == 0 {
		return fmt.Errorf("bundle requires room templates")
	}
	return nil
}

func (m *Manifest) SortedProfileIDs() []string {
	ids := make([]string, 0, len(m.Profiles))
	for id := range m.Profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (m *Manifest) Class(id shared.PlayerClass) (ClassDefinition, bool) {
	for _, class := range m.Classes {
		if class.ID == id {
			return class, true
		}
	}
	return ClassDefinition{}, false
}

func (m *Manifest) Profile(id string) (AssetProfile, bool) {
	profile, ok := m.Profiles[id]
	return profile, ok
}

func (profile AssetProfile) HitboxFor(animation shared.AnimationState, elapsed float64, facing float64, origin shared.Vec2) (shared.Rect, bool) {
	for _, entry := range profile.Hitboxes {
		if entry.State != animation {
			continue
		}
		for _, frame := range entry.Frames {
			if elapsed < frame.Start || elapsed > frame.End {
				continue
			}
			box := frame.Box
			if facing < 0 {
				box.X = -box.X - box.W + profile.Collider.W
			}
			return shared.Rect{
				X: origin.X + box.X,
				Y: origin.Y + box.Y,
				W: box.W,
				H: box.H,
			}, true
		}
	}
	return shared.Rect{}, false
}

func (profile AssetProfile) DefaultState() shared.EntityState {
	return shared.EntityState{
		Kind:           profile.Kind,
		Faction:        profile.Faction,
		ProfileID:      profile.ID,
		FamilyID:       profile.FamilyID,
		ClassID:        profile.ClassID,
		MaxHP:          profile.MaxHP,
		HP:             profile.MaxHP,
		Facing:         1,
		Animation:      shared.AnimationIdle,
		Scale:          profile.Scale,
		SpriteSize:     profile.SpriteSize,
		SpriteOffset:   profile.SpriteOffset,
		Collider:       profile.Collider,
		Hurtbox:        profile.Hurtbox,
		InteractionBox: profile.InteractionBox,
	}
}
