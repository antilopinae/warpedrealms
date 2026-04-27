// Copyright (c) 2024 Warped Realms. All rights reserved.
// This source code is proprietary and confidential.
// Unauthorized copying or cloning of game mechanics is strictly prohibited.
// See LICENSE file in the project root for full license details.

package clientapp

import (
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"warpedrealms/content"
	"warpedrealms/shared"
	"warpedrealms/world"
)

type Animation struct {
	Frames        []*ebiten.Image
	FrameDuration float64
	Loop          bool
}

func (a Animation) FrameAt(elapsed float64) *ebiten.Image {
	if len(a.Frames) == 0 {
		return nil
	}
	if elapsed < 0 {
		elapsed = 0
	}
	index := int(elapsed / maxf(0.001, a.FrameDuration))
	if a.Loop {
		index %= len(a.Frames)
	} else if index >= len(a.Frames) {
		index = len(a.Frames) - 1
	}
	return a.Frames[index]
}

type LayerSprite struct {
	Image  *ebiten.Image
	Offset shared.Vec2
	Scale  float64
}

type CompositeAsset struct {
	Body     LayerSprite
	LeftArm  LayerSprite
	RightArm LayerSprite
	LeftLeg  LayerSprite
	RightLeg LayerSprite
	Eyes     LayerSprite
	Mouth    LayerSprite
	Detail   LayerSprite
}

type BackgroundLayerAsset struct {
	Image    *ebiten.Image
	Parallax float64
	Alpha    float64
	OffsetY  float64
}

type BackgroundAsset struct {
	Base   *ebiten.Image
	Layers []BackgroundLayerAsset
}

type TileStyleAsset struct {
	Floor       *ebiten.Image
	Platform    *ebiten.Image
	Accent      *ebiten.Image
	JumpPreview *ebiten.Image
}

type UIAssets struct {
	Panel      *ebiten.Image
	Button     *ebiten.Image
	Selection  *ebiten.Image
	Heart      *ebiten.Image
	HeartHalf  *ebiten.Image
	HeartEmpty *ebiten.Image
}

type FXAssets struct {
	Slash       Animation
	Portal      *ebiten.Image
	PortalGlow  *ebiten.Image
	Smoke       *ebiten.Image
	MagicGreen  *ebiten.Image
	MagicBlue   *ebiten.Image
	Lightning   *ebiten.Image
	TravelFlame *ebiten.Image
}

type Assets struct {
	Manifest    *content.Manifest
	Animations  map[string]map[shared.AnimationState]Animation
	Layers      map[string][]LayerSprite
	Composites  map[string]CompositeAsset
	Backgrounds map[string]BackgroundAsset
	TileMaps    map[string]*world.MapData // keyed by TemplateID e.g. "map_1.tmx"
	TileImages  map[string]*ebiten.Image  // keyed by resolved tileset image path
	TileStyles  map[string]TileStyleAsset
	UI          UIAssets
	FX          FXAssets
}

func LoadAssets(manifest *content.Manifest) (*Assets, error) {
	assets := &Assets{
		Manifest:    manifest,
		Animations:  make(map[string]map[shared.AnimationState]Animation),
		Layers:      make(map[string][]LayerSprite),
		Composites:  make(map[string]CompositeAsset),
		Backgrounds: make(map[string]BackgroundAsset),
		TileStyles:  make(map[string]TileStyleAsset),
		TileMaps:    make(map[string]*world.MapData),
		TileImages:  make(map[string]*ebiten.Image),
	}

	// Load LDtk projects — each level inside becomes a separate MapData entry.
	if matches, err := filepath.Glob("gamedata/map/*.ldtk"); err == nil {
		for _, ldtkPath := range matches {
			levels, err := world.LoadLDtk(ldtkPath)
			if err != nil {
				continue // non-fatal
			}
			for _, mapData := range levels {
				assets.TileMaps[mapData.ID] = mapData
				// Load tileset images referenced by LDtk tile layers.
				for _, layer := range mapData.LDtkLayers {
					if _, loaded := assets.TileImages[layer.TilesetPath]; loaded {
						continue
					}
					img, err := loadImageFile(layer.TilesetPath)
					if err != nil {
						continue // non-fatal
					}
					assets.TileImages[layer.TilesetPath] = img
				}
			}
		}
	}

	for _, profileID := range manifest.SortedProfileIDs() {
		profile, _ := manifest.Profile(profileID)
		if len(profile.Animations) > 0 {
			bank := make(map[shared.AnimationState]Animation, len(profile.Animations))
			for _, clip := range profile.Animations {
				animation, err := loadAnimation(clip.Pattern, clip.FrameDuration, clip.Loop)
				if err != nil {
					return nil, err
				}
				bank[clip.State] = animation
			}
			assets.Animations[profileID] = bank
		}
		if len(profile.Layers) > 0 {
			layers := make([]LayerSprite, 0, len(profile.Layers))
			for _, layer := range profile.Layers {
				image, err := loadImageFile(layer.Path)
				if err != nil {
					return nil, err
				}
				scale := layer.Scale
				if scale == 0 {
					scale = 1
				}
				layers = append(layers, LayerSprite{
					Image:  image,
					Offset: layer.Offset,
					Scale:  scale,
				})
			}
			assets.Layers[profileID] = layers
		}
		if profile.Composite != nil {
			composite, err := loadComposite(profile.Composite)
			if err != nil {
				return nil, err
			}
			assets.Composites[profileID] = composite
		}
	}

	for id, background := range manifest.Backgrounds {
		base, err := loadImageFile(background.Base)
		if err != nil {
			return nil, err
		}
		asset := BackgroundAsset{Base: base}
		for _, layer := range background.Layers {
			image, err := loadImageFile(layer.Path)
			if err != nil {
				return nil, err
			}
			asset.Layers = append(asset.Layers, BackgroundLayerAsset{
				Image:    image,
				Parallax: layer.Parallax,
				Alpha:    layer.Alpha,
				OffsetY:  layer.OffsetY,
			})
		}
		assets.Backgrounds[id] = asset
	}

	for id, style := range manifest.TileStyles {
		tile := TileStyleAsset{}
		var err error
		if style.Floor != "" {
			tile.Floor, err = loadImageFile(style.Floor)
			if err != nil {
				return nil, err
			}
		}
		if style.Platform != "" {
			tile.Platform, err = loadImageFile(style.Platform)
			if err != nil {
				return nil, err
			}
		}
		if style.Accent != "" {
			tile.Accent, err = loadImageFile(style.Accent)
			if err != nil {
				return nil, err
			}
		}
		if style.JumpPreview != "" {
			tile.JumpPreview, err = loadImageFile(style.JumpPreview)
			if err != nil {
				return nil, err
			}
		}
		assets.TileStyles[id] = tile
	}

	if manifest.UISkin.Panel != "" {
		image, err := loadImageFile(manifest.UISkin.Panel)
		if err != nil {
			return nil, err
		}
		assets.UI.Panel = image
	}
	if manifest.UISkin.Button != "" {
		image, err := loadImageFile(manifest.UISkin.Button)
		if err != nil {
			return nil, err
		}
		assets.UI.Button = image
	}
	if manifest.UISkin.Selection != "" {
		image, err := loadImageFile(manifest.UISkin.Selection)
		if err != nil {
			return nil, err
		}
		assets.UI.Selection = image
	}
	if manifest.UISkin.Heart != "" {
		image, err := loadImageFile(manifest.UISkin.Heart)
		if err != nil {
			return nil, err
		}
		assets.UI.Heart = image
	}
	if manifest.UISkin.HeartHalf != "" {
		image, err := loadImageFile(manifest.UISkin.HeartHalf)
		if err != nil {
			return nil, err
		}
		assets.UI.HeartHalf = image
	}
	if manifest.UISkin.HeartEmpty != "" {
		image, err := loadImageFile(manifest.UISkin.HeartEmpty)
		if err != nil {
			return nil, err
		}
		assets.UI.HeartEmpty = image
	}

	// FX assets: loaded best-effort, nil is handled gracefully by renderers.

	return assets, nil
}

func (a *Assets) AnimationFor(state shared.EntityState) Animation {
	if bank, ok := a.Animations[state.ProfileID]; ok {
		if animation, ok := bank[state.Animation]; ok {
			return animation
		}
		if animation, ok := bank[shared.DesiredMovementAnimation(state)]; ok {
			return animation
		}
		if animation, ok := bank[shared.AnimationIdle]; ok {
			return animation
		}
	}
	return Animation{}
}

func (a *Assets) IdleFrame(profileID string) *ebiten.Image {
	bank, ok := a.Animations[profileID]
	if !ok {
		return nil
	}
	if idle, ok := bank[shared.AnimationIdle]; ok {
		return idle.FrameAt(0)
	}
	for _, animation := range bank {
		return animation.FrameAt(0)
	}
	return nil
}

func loadComposite(profile *content.CompositeProfile) (CompositeAsset, error) {
	body, err := loadLayer(profile.Body)
	if err != nil {
		return CompositeAsset{}, err
	}
	leftArm, err := loadLayer(profile.LeftArm)
	if err != nil {
		return CompositeAsset{}, err
	}
	rightArm, err := loadLayer(profile.RightArm)
	if err != nil {
		return CompositeAsset{}, err
	}
	leftLeg, err := loadLayer(profile.LeftLeg)
	if err != nil {
		return CompositeAsset{}, err
	}
	rightLeg, err := loadLayer(profile.RightLeg)
	if err != nil {
		return CompositeAsset{}, err
	}
	eyes, err := loadLayer(profile.Eyes)
	if err != nil {
		return CompositeAsset{}, err
	}
	mouth, err := loadLayer(profile.Mouth)
	if err != nil {
		return CompositeAsset{}, err
	}
	detail, err := loadLayer(profile.Detail)
	if err != nil {
		return CompositeAsset{}, err
	}
	return CompositeAsset{
		Body:     body,
		LeftArm:  leftArm,
		RightArm: rightArm,
		LeftLeg:  leftLeg,
		RightLeg: rightLeg,
		Eyes:     eyes,
		Mouth:    mouth,
		Detail:   detail,
	}, nil
}

func loadLayer(part content.CompositePart) (LayerSprite, error) {
	image, err := loadImageFile(part.Path)
	if err != nil {
		return LayerSprite{}, err
	}
	scale := part.Scale
	if scale == 0 {
		scale = 1
	}
	return LayerSprite{
		Image:  image,
		Offset: part.Offset,
		Scale:  scale,
	}, nil
}

func loadAnimation(pattern string, frameDuration float64, loop bool) (Animation, error) {
	paths := []string{pattern}
	if strings.ContainsAny(pattern, "*?[") {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return Animation{}, err
		}
		paths = matches
	}
	if len(paths) == 0 {
		return Animation{}, os.ErrNotExist
	}
	sort.Strings(paths)
	frames := make([]*ebiten.Image, 0, len(paths))
	for _, path := range paths {
		image, err := loadImageFile(path)
		if err != nil {
			return Animation{}, err
		}
		frames = append(frames, image)
	}
	return Animation{Frames: frames, FrameDuration: frameDuration, Loop: loop}, nil
}

func loadImageFile(path string) (*ebiten.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	raw, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}
	return ebiten.NewImageFromImage(raw), nil
}

func maxf(a float64, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
