// Copyright (c) 2024 Warped Realms. All rights reserved.
// This source code is proprietary and confidential.
// Unauthorized copying or cloning of game mechanics is strictly prohibited.
// See LICENSE file in the project root for full license details.

package clientapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

type Action string

const (
	ActionMoveLeft    Action = "move_left"
	ActionMoveRight   Action = "move_right"
	ActionJump        Action = "jump"
	ActionAttack      Action = "attack"
	ActionSkill1      Action = "skill_1"
	ActionSkill2      Action = "skill_2"
	ActionSkill3      Action = "skill_3"
	ActionInteract    Action = "interact"
	ActionUseJumpLink Action = "use_jump_link"
	ActionDropDown    Action = "drop_down"
	ActionDash        Action = "dash"
)

type BindingKind string

const (
	BindingKey   BindingKind = "key"
	BindingMouse BindingKind = "mouse"
)

var actionOrder = []Action{
	ActionMoveLeft,
	ActionMoveRight,
	ActionJump,
	ActionDash,
	ActionAttack,
	ActionSkill1,
	ActionSkill2,
	ActionSkill3,
	ActionInteract,
	ActionUseJumpLink,
	ActionDropDown,
}

var actionLabels = map[Action]string{
	ActionMoveLeft:    "Move Left",
	ActionMoveRight:   "Move Right",
	ActionJump:        "Jump",
	ActionAttack:      "Primary Attack",
	ActionSkill1:      "Skill 1",
	ActionSkill2:      "Skill 2",
	ActionSkill3:      "Skill 3",
	ActionInteract:    "Interact",
	ActionUseJumpLink: "Jump Link",
	ActionDash:        "Dash",
}

type InputBinding struct {
	Kind        BindingKind
	Key         ebiten.Key
	MouseButton ebiten.MouseButton
}

type Controls struct {
	Bindings map[Action]InputBinding
}

type controlsFile struct {
	Bindings map[string]json.RawMessage `json:"bindings"`
}

type controlsBinding struct {
	Kind string `json:"kind"`
	Code int    `json:"code"`
}

var defaultBindings = map[Action]InputBinding{
	ActionMoveLeft:    KeyBinding(ebiten.KeyA),
	ActionMoveRight:   KeyBinding(ebiten.KeyD),
	ActionJump:        KeyBinding(ebiten.KeySpace),
	ActionAttack:      KeyBinding(ebiten.KeyI),
	ActionSkill1:      MouseBinding(ebiten.MouseButtonRight),
	ActionSkill2:      KeyBinding(ebiten.KeyQ),
	ActionSkill3:      KeyBinding(ebiten.KeyR),
	ActionInteract:    KeyBinding(ebiten.KeyF),
	ActionUseJumpLink: KeyBinding(ebiten.KeyE),
	ActionDropDown:    KeyBinding(ebiten.KeyS),
	ActionDash:        KeyBinding(ebiten.KeyShiftLeft),
}

func KeyBinding(key ebiten.Key) InputBinding {
	return InputBinding{Kind: BindingKey, Key: key}
}

func MouseBinding(button ebiten.MouseButton) InputBinding {
	return InputBinding{Kind: BindingMouse, MouseButton: button}
}

func DefaultControls() Controls {
	bindings := make(map[Action]InputBinding, len(defaultBindings))
	for action, binding := range defaultBindings {
		bindings[action] = binding
	}
	return Controls{Bindings: bindings}
}

func LoadControls(path string) (Controls, error) {
	controls := DefaultControls()
	legacyFormat := false

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return controls, nil
		}
		return Controls{}, err
	}

	var file controlsFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return Controls{}, err
	}

	for actionName, payload := range file.Bindings {
		action := Action(actionName)

		var legacyKeyCode int
		if err := json.Unmarshal(payload, &legacyKeyCode); err == nil {
			legacyFormat = true
			controls.Bindings[action] = KeyBinding(ebiten.Key(legacyKeyCode))
			continue
		}

		var binding controlsBinding
		if err := json.Unmarshal(payload, &binding); err != nil {
			return Controls{}, err
		}
		switch BindingKind(binding.Kind) {
		case BindingMouse:
			controls.Bindings[action] = MouseBinding(ebiten.MouseButton(binding.Code))
		default:
			controls.Bindings[action] = KeyBinding(ebiten.Key(binding.Code))
		}
	}
	if legacyFormat {
		migrateLegacyDefaults(&controls)
	}
	return controls, nil
}

func (c Controls) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	file := controlsFile{
		Bindings: make(map[string]json.RawMessage, len(c.Bindings)),
	}
	for _, action := range actionOrder {
		binding := c.Binding(action)
		payload, err := json.Marshal(controlsBinding{
			Kind: string(binding.Kind),
			Code: binding.Code(),
		})
		if err != nil {
			return err
		}
		file.Bindings[string(action)] = payload
	}

	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func (c Controls) Binding(action Action) InputBinding {
	if binding, ok := c.Bindings[action]; ok {
		return binding
	}
	return defaultBindings[action]
}

func (c Controls) Pressed(action Action) bool {
	return c.Binding(action).Pressed()
}

func (c Controls) JustPressed(action Action) bool {
	return c.Binding(action).JustPressed()
}

func (c *Controls) Set(action Action, binding InputBinding) {
	if c.Bindings == nil {
		c.Bindings = make(map[Action]InputBinding, len(defaultBindings))
	}
	c.Bindings[action] = binding
}

func (c *Controls) Reset(action Action) {
	c.Set(action, defaultBindings[action])
}

func (c *Controls) ResetAll() {
	*c = DefaultControls()
}

func (b InputBinding) Code() int {
	switch b.Kind {
	case BindingMouse:
		return int(b.MouseButton)
	default:
		return int(b.Key)
	}
}

func (b InputBinding) Pressed() bool {
	switch b.Kind {
	case BindingMouse:
		return ebiten.IsMouseButtonPressed(b.MouseButton)
	default:
		return ebiten.IsKeyPressed(b.Key)
	}
}

func (b InputBinding) JustPressed() bool {
	switch b.Kind {
	case BindingMouse:
		return inpututil.IsMouseButtonJustPressed(b.MouseButton)
	default:
		return inpututil.IsKeyJustPressed(b.Key)
	}
}

func ActionLabel(action Action) string {
	return actionLabels[action]
}

func KeyLabel(key ebiten.Key) string {
	switch key {
	case ebiten.KeySpace:
		return "Space"
	case ebiten.KeyArrowLeft:
		return "ArrowLeft"
	case ebiten.KeyArrowRight:
		return "ArrowRight"
	case ebiten.KeyArrowUp:
		return "ArrowUp"
	case ebiten.KeyArrowDown:
		return "ArrowDown"
	default:
		return key.String()
	}
}

func MouseButtonLabel(button ebiten.MouseButton) string {
	switch button {
	case ebiten.MouseButtonLeft:
		return "Mouse Left"
	case ebiten.MouseButtonRight:
		return "Mouse Right"
	case ebiten.MouseButtonMiddle:
		return "Mouse Middle"
	case ebiten.MouseButton3:
		return "Mouse Button 4"
	case ebiten.MouseButton4:
		return "Mouse Button 5"
	default:
		return "Mouse " + strconv.Itoa(int(button))
	}
}

func BindingLabel(controls Controls, action Action) string {
	return controls.Binding(action).Label()
}

func (b InputBinding) Label() string {
	switch b.Kind {
	case BindingMouse:
		return MouseButtonLabel(b.MouseButton)
	default:
		return KeyLabel(b.Key)
	}
}

func migrateLegacyDefaults(controls *Controls) {
	legacyDefaults := map[Action]ebiten.Key{
		ActionAttack: ebiten.KeyJ,
		ActionSkill1: ebiten.KeyDigit1,
		ActionSkill2: ebiten.KeyDigit2,
		ActionSkill3: ebiten.KeyDigit3,
	}
	for action, legacyKey := range legacyDefaults {
		current := controls.Binding(action)
		if current.Kind == BindingKey && current.Key == legacyKey {
			controls.Set(action, defaultBindings[action])
		}
	}
}
