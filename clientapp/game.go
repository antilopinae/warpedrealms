// Copyright (c) 2024 Warped Realms. All rights reserved.
// This source code is proprietary and confidential.
// Unauthorized copying or cloning of game mechanics is strictly prohibited.
// See LICENSE file in the project root for full license details.

package clientapp

import (
	"fmt"
	"image/color"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"warpedrealms/content"
	"warpedrealms/shared"
)

type screenMode int

const (
	screenLogin screenMode = iota
	screenLobby
	screenConnecting
	screenGame
	screenSettings
)

type inputKeyOverlay struct {
	action Action
	rect   shared.Rect
	label  string
	img    *ebiten.Image // Предварительно отрисованный текст
}

type Game struct {
	baseURL  string
	bundle   *content.Bundle
	assets   *Assets
	renderer *RaidRenderer
	controls Controls
	network  *NetworkClient
	uiEvents chan func()

	screen               screenMode
	previousScreen       screenMode
	authMode             string
	email                string
	password             string
	activeField          int
	status               string
	settingsPath         string
	settingsSelection    int
	awaitingBinding      bool
	bindingCapturePrimed bool
	settingsStatus       string

	token           string
	raids           []shared.RaidSummary
	selectedRaid    int
	selectedClass   int
	currentRaidID   string
	currentRaid     shared.RaidState
	hasRaidState    bool
	currentLayout   shared.RaidLayoutState
	hasLayout       bool
	layoutSolids    map[string][]shared.Rect // roomID → solids; never cross-room
	layoutPlatforms map[string][]shared.Rect // roomID → one-way platforms

	localPlayer      shared.EntityState
	localReady       bool
	localID          string
	pendingInput     []shared.InputCommand
	nextSeq          uint32
	lastAckSeq       uint32
	localAttackCombo int
	predictedAnim    shared.AnimationState
	predictedAnimAt  float64
	predictedSeq     uint32

	loot          []shared.LootState
	interpolators map[string]*Interpolator
	lastServerAt  time.Time
	lastServerTS  float64
	lastPingAt    time.Time
	pingMS        float64

	camera              shared.Vec2
	viewOffset          shared.Vec2
	worldScale          float64
	startedAt           time.Time
	debugPhysics        bool
	useSpriteRectRender bool

	inputOverlayKeys []inputKeyOverlay
	whitePixelImg    *ebiten.Image

	// bots[0] roams the background room (BelowRoomID of the active room).
	// bots[1] roams the same room as the local player.
	bots [2]*LocalBot
}

func NewGame(baseURL string, manifestPath string, roomsDir string) (*Game, error) {
	bundle, err := content.LoadBundle(manifestPath, roomsDir)
	if err != nil {
		return nil, err
	}

	assets, err := LoadAssets(bundle.Manifest)
	if err != nil {
		return nil, err
	}

	settingsPath := filepath.Join("data", "client_settings.json")
	controls, err := LoadControls(settingsPath)
	settingsStatus := "Enter: rebinding  Backspace: reset action  R: reset all  Esc: back"
	if err != nil {
		controls = DefaultControls()
		settingsStatus = fmt.Sprintf("Controls reset to defaults: %v", err)
	}

	game := &Game{
		baseURL:             baseURL,
		bundle:              bundle,
		assets:              assets,
		controls:            controls,
		network:             NewNetworkClient(baseURL),
		uiEvents:            make(chan func(), 64),
		screen:              screenLogin,
		authMode:            "sign-up",
		email:               "player@example.com",
		password:            "1234",
		status:              "F2 to sign-up/sign-in. Tab to switch. Enter to continue.",
		settingsPath:        settingsPath,
		settingsStatus:      settingsStatus,
		interpolators:       make(map[string]*Interpolator),
		worldScale:          float64(shared.ScreenHeight) / 480.0,
		startedAt:           time.Now(),
		useSpriteRectRender: true,
		bots: [2]*LocalBot{
			NewLocalBot("__bot_bg__", "Bot-BG"), // [0] background room
			NewLocalBot("__bot_fg__", "Bot-FG"), // [1] same room as player
		},
	}
	game.syncSelectedClass()
	game.updateViewOffset()

	// 1. Создаем белый пиксель (замена пакету vector)
	game.whitePixelImg = ebiten.NewImage(1, 1)
	game.whitePixelImg.Fill(color.White)
	game.renderer = NewRaidRenderer(assets, game.whitePixelImg)

	// 2. Настраиваем кнопки
	baseX, baseY := float64(shared.ScreenWidth)-220.0, float64(shared.ScreenHeight)-150.0

	// Вспомогательный список для инициализации
	tempKeys := []struct {
		act        Action
		x, y, w, h float64
		lbl        string
	}{
		{ActionJump, 60, 0, 50, 40, "JMP"},
		{ActionMoveLeft, 0, 45, 50, 40, "A"},
		{ActionDropDown, 60, 45, 50, 40, "S"},
		{ActionMoveRight, 120, 45, 50, 40, "D"},
		{ActionAttack, 0, 95, 80, 35, "I"},
		{ActionSkill1, 90, 95, 80, 35, "SKL"},
		{ActionDash, 150, 95, 70, 35, "SHIFT"},
	}

	game.inputOverlayKeys = make([]inputKeyOverlay, len(tempKeys))
	for i, k := range tempKeys {
		// Создаем картинку для текста (размером с кнопку)
		txtImg := ebiten.NewImage(int(k.w), int(k.h))
		ebitenutil.DebugPrintAt(txtImg, k.lbl, 10, 12) // Рисуем текст ОДИН РАЗ

		game.inputOverlayKeys[i] = inputKeyOverlay{
			action: k.act,
			rect:   shared.Rect{X: baseX + k.x, Y: baseY + k.y, W: k.w, H: k.h},
			label:  k.lbl,
			img:    txtImg,
		}
	}

	return game, nil
}

func (g *Game) Layout(_, _ int) (int, int) {
	return shared.ScreenWidth, shared.ScreenHeight
}

func (g *Game) Update() error {
	g.drainUIEvents()
	g.drainNetwork()

	if g.screen != screenConnecting && g.screen != screenSettings && inpututil.IsKeyJustPressed(ebiten.KeyF1) {
		g.openSettings()
		return nil
	}

	switch g.screen {
	case screenLogin:
		g.updateLogin()
	case screenConnecting:
	case screenLobby:
		g.updateLobby()
	case screenGame:
		g.updateGame()
	case screenSettings:
		g.updateSettings()
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{11, 16, 22, 255})
	switch g.screen {
	case screenLobby:
		g.drawLobby(screen)
	case screenGame:
		g.drawGame(screen)
	case screenSettings:
		g.drawSettings(screen)
	default:
		g.drawLogin(screen)
	}
}

func (g *Game) updateLogin() {
	signUpRect := shared.Rect{X: 160, Y: 176, W: 160, H: 34}
	signInRect := shared.Rect{X: 332, Y: 176, W: 160, H: 34}
	emailRect := shared.Rect{X: 160, Y: 286, W: 960, H: 54}
	passwordRect := shared.Rect{X: 160, Y: 368, W: 960, H: 54}
	submitRect := shared.Rect{X: 160, Y: 452, W: 220, H: 42}

	if uiClick(signUpRect) {
		g.authMode = "sign-up"
	}
	if uiClick(signInRect) {
		g.authMode = "sign-in"
	}
	if uiClick(emailRect) {
		g.activeField = 0
	}
	if uiClick(passwordRect) {
		g.activeField = 1
	}
	if uiClick(submitRect) {
		g.beginAuth()
		return
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		g.activeField = (g.activeField + 1) % 2
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF2) {
		if g.authMode == "sign-up" {
			g.authMode = "sign-in"
		} else {
			g.authMode = "sign-up"
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		g.beginAuth()
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		if g.activeField == 0 {
			g.email = deleteLastRune(g.email)
		} else {
			g.password = deleteLastRune(g.password)
		}
	}
	for _, char := range ebiten.AppendInputChars(nil) {
		if char < 32 || char == 127 {
			continue
		}
		if g.activeField == 0 {
			g.email += string(char)
		} else {
			g.password += string(char)
		}
	}
}

func (g *Game) updateLobby() {
	joinRect := shared.Rect{X: 884, Y: 92, W: 132, H: 40}
	createRect := shared.Rect{X: 1028, Y: 92, W: 160, H: 40}
	refreshRect := shared.Rect{X: 884, Y: 142, W: 132, H: 40}
	settingsRect := shared.Rect{X: 1028, Y: 142, W: 160, H: 40}
	logoutRect := shared.Rect{X: 884, Y: 192, W: 304, H: 40}

	for index := range g.raids {
		rowY := float64(360 + index*66)
		rowRect := shared.Rect{X: 88, Y: rowY, W: 1100, H: 54}
		if uiClick(rowRect) {
			if g.selectedRaid == index {
				g.connectToRaid(g.raids[index].ID)
				return
			}
			g.selectedRaid = index
		}
	}
	if uiClick(joinRect) && len(g.raids) > 0 {
		g.connectToRaid(g.raids[g.selectedRaid].ID)
		return
	}
	if uiClick(createRect) {
		g.createRaidAndJoin()
		return
	}
	if uiClick(refreshRect) {
		g.refreshRaids()
	}
	if uiClick(settingsRect) {
		g.openSettings()
		return
	}
	if uiClick(logoutRect) {
		g.token = ""
		g.screen = screenLogin
		g.status = "Сессия сброшена."
		return
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.token = ""
		g.screen = screenLogin
		g.status = "Сессия сброшена."
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		g.refreshRaids()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyN) {
		g.createRaidAndJoin()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) && len(g.raids) > 0 {
		g.selectedRaid--
		if g.selectedRaid < 0 {
			g.selectedRaid = len(g.raids) - 1
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) && len(g.raids) > 0 {
		g.selectedRaid = (g.selectedRaid + 1) % len(g.raids)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		if len(g.raids) == 0 {
			g.createRaidAndJoin()
			return
		}
		g.connectToRaid(g.raids[g.selectedRaid].ID)
	}
}

func (g *Game) syncSelectedClass() {
	if g.bundle == nil || g.bundle.Manifest == nil || len(g.bundle.Manifest.Classes) == 0 {
		g.selectedClass = 0
		return
	}
	classDef, ok := g.bundle.Manifest.DefaultPlayerClass()
	if !ok {
		g.selectedClass = 0
		return
	}
	for i, class := range g.bundle.Manifest.Classes {
		if class.ID == classDef.ID {
			g.selectedClass = i
			return
		}
	}
	g.selectedClass = 0
}

func (g *Game) selectedClassDefinition() (content.ClassDefinition, bool) {
	if g.bundle == nil || g.bundle.Manifest == nil || len(g.bundle.Manifest.Classes) == 0 {
		return content.ClassDefinition{}, false
	}
	if g.selectedClass < 0 || g.selectedClass >= len(g.bundle.Manifest.Classes) {
		g.syncSelectedClass()
	}
	if g.selectedClass < 0 || g.selectedClass >= len(g.bundle.Manifest.Classes) {
		return content.ClassDefinition{}, false
	}
	return g.bundle.Manifest.Classes[g.selectedClass], true
}

func (g *Game) selectedClassID() string {
	classDef, ok := g.selectedClassDefinition()
	if !ok {
		return ""
	}
	return classDef.ID
}

func (g *Game) syncSelectedClassByID(classID string) {
	if classID == "" || g.bundle == nil || g.bundle.Manifest == nil {
		return
	}
	for i, class := range g.bundle.Manifest.Classes {
		if class.ID == classID {
			g.selectedClass = i
			return
		}
	}
}

func (g *Game) updateGame() {
	if inpututil.IsKeyJustPressed(ebiten.KeyF3) {
		g.debugPhysics = !g.debugPhysics
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF4) {
		g.useSpriteRectRender = !g.useSpriteRectRender
		g.renderer.SetUseSpriteRectOps(g.useSpriteRectRender)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.network.Close()
		g.resetRaidState()
		g.screen = screenLobby
		g.status = "Вернулся в lobby."
		g.refreshRaids()
		return
	}
	if !g.localReady || !g.hasRaidState {
		return
	}
	if g.currentRaid.LocalStatus != shared.PlayerRaidStatusActive {
		return
	}

	command := g.captureInput()
	g.pendingInput = append(g.pendingInput, command)
	if len(g.pendingInput) > 120 {
		g.pendingInput = g.pendingInput[len(g.pendingInput)-120:]
	}
	now := g.estimatedServerTime()

	if command.PrimaryAttack {
		g.predictedAnim = shared.NextAttackAnimation(g.localPlayer, g.localAttackCombo)
		g.predictedAnimAt = now
		g.predictedSeq = command.Seq
		g.localAttackCombo++
		shared.TriggerAnimation(&g.localPlayer, g.predictedAnim, now)
	}
	if command.Skill1 {
		g.predictedAnim = shared.AnimationSkill1
		g.predictedAnimAt = now
		g.predictedSeq = command.Seq
		shared.TriggerAnimation(&g.localPlayer, shared.AnimationSkill1, now)
	}
	if command.Skill2 {
		g.predictedAnim = shared.AnimationSkill2
		g.predictedAnimAt = now
		g.predictedSeq = command.Seq
		shared.TriggerAnimation(&g.localPlayer, shared.AnimationSkill2, now)
	}
	if command.Skill3 {
		g.predictedAnim = shared.AnimationSkill3
		g.predictedAnimAt = now
		g.predictedSeq = command.Seq
		shared.TriggerAnimation(&g.localPlayer, shared.AnimationSkill3, now)
	}

	if g.localPlayer.Travel == nil || !g.localPlayer.Travel.Active {
		shared.SimulatePlayer(&g.localPlayer, command, g.solidsForPlayer(), g.platformsForPlayer())
	}
	shared.RefreshAnimation(&g.localPlayer, now)

	// ── Bot updates ────────────────────────────────────────────────────────────
	g.updateBots(now)

	if err := g.network.SendInputs(g.pendingInput); err != nil {
		g.status = err.Error()
	}
	if time.Since(g.lastPingAt) > time.Second {
		_ = g.network.SendPing(g.clientTime())
		g.lastPingAt = time.Now()
	}
	g.updateCamera()
}

// realPlayersInRoom returns the number of real (non-local) players currently
// in roomID according to the latest interpolated snapshot data.
// Bots are excluded by their ID prefix.
func (g *Game) realPlayersInRoom(roomID string, targetTime float64) int {
	count := 0
	for id, interp := range g.interpolators {
		// Skip the local player and client-side bots.
		if id == g.localID || id == "__bot_bg__" || id == "__bot_fg__" {
			continue
		}
		state, ok := interp.Sample(targetTime)
		if !ok {
			continue
		}
		if state.Kind == shared.EntityKindPlayer && state.RoomID == roomID {
			count++
		}
	}
	return count
}

// updateBots assigns rooms to the two bots and ticks their AI + physics.
//
//   - bots[0]: background room (BelowRoomID of the active room).
//   - bots[1]: same room as the local player.
//
// A bot is active only when NO other real player is in its assigned room.
// Each client independently checks the interpolated entity list it received
// from the server, so all clients agree on who is alone.
func (g *Game) updateBots(now float64) {
	if !g.localReady || g.layoutSolids == nil {
		return
	}
	activeRoomID := g.localPlayer.RoomID
	if activeRoomID == "" {
		return
	}
	activeRoom, ok := g.currentLayout.RoomByID(activeRoomID)
	if !ok {
		return
	}
	targetTime := now - shared.InterpolationBackTime

	// Bot 0 → background room: RevealZone target (when near a portal) or BelowRoomID.
	// This matches exactly which room DrawScene renders in the background layer.
	bgRoomID := g.revealBgRoomID(activeRoomID)
	if bgRoomID == "" {
		bgRoomID = activeRoom.BelowRoomID
	}
	if bgRoomID != "" {
		if bgRoom, ok := g.currentLayout.RoomByID(bgRoomID); ok {
			bgSolids := g.layoutSolids[bgRoom.ID]
			bgPlatforms := g.layoutPlatforms[bgRoom.ID]
			if g.realPlayersInRoom(bgRoom.ID, targetTime) == 0 {
				g.bots[0].AssignRoom(bgRoom.ID, bgRoom, g.localPlayer, now, bgSolids, bgPlatforms)
				g.bots[0].Update(now, bgRoom, bgSolids, bgPlatforms)
			} else {
				g.bots[0].Deactivate()
			}
		}
	} else {
		g.bots[0].Deactivate()
	}

	// Bot 1 → same room as local player (only when no other real players here).
	fgSolids := g.layoutSolids[activeRoom.ID]
	fgPlatforms := g.layoutPlatforms[activeRoom.ID]
	if g.realPlayersInRoom(activeRoom.ID, targetTime) == 0 {
		g.bots[1].AssignRoom(activeRoom.ID, activeRoom, g.localPlayer, now, fgSolids, fgPlatforms)
		g.bots[1].Update(now, activeRoom, fgSolids, fgPlatforms)
	} else {
		g.bots[1].Deactivate()
	}
}

func (g *Game) beginAuth() {
	if g.screen == screenConnecting {
		return
	}
	g.network.Close()
	g.screen = screenConnecting
	g.status = "Связываюсь с сервером..."

	email := g.email
	password := g.password
	mode := g.authMode

	go func() {
		var (
			token string
			err   error
		)
		if mode == "sign-up" {
			token, err = g.network.SignUp(email, password)
		} else {
			token, err = g.network.SignIn(email, password)
		}
		if err != nil {
			g.post(func() {
				g.screen = screenLogin
				g.status = err.Error()
			})
			return
		}

		raids, err := g.network.ListRaids(token)
		if err != nil {
			g.post(func() {
				g.screen = screenLogin
				g.status = err.Error()
			})
			return
		}

		g.post(func() {
			g.token = token
			g.raids = raids
			g.selectedRaid = 0
			g.syncSelectedClass()
			g.screen = screenLobby
			if classDef, ok := g.selectedClassDefinition(); ok {
				g.status = fmt.Sprintf("Авторизация успешна. Класс по умолчанию: %s. Выбери рейд.", classDef.Name)
			} else {
				g.status = "Авторизация успешна. Выбери рейд."
			}
		})
	}()
}

func (g *Game) refreshRaids() {
	if g.token == "" {
		return
	}
	token := g.token
	go func() {
		raids, err := g.network.ListRaids(token)
		if err != nil {
			g.post(func() {
				g.status = err.Error()
			})
			return
		}
		g.post(func() {
			g.raids = raids
			if g.selectedRaid >= len(g.raids) {
				g.selectedRaid = 0
			}
			g.status = "Список рейдов обновлён."
		})
	}()
}

func (g *Game) createRaidAndJoin() {
	if g.token == "" {
		return
	}
	token := g.token
	g.screen = screenConnecting
	g.status = "Создаю новый рейд..."
	go func() {
		raid, err := g.network.CreateRaid(token)
		if err != nil {
			g.post(func() {
				g.screen = screenLobby
				g.status = err.Error()
			})
			return
		}
		g.post(func() {
			g.raids = append(g.raids, raid)
			sort.Slice(g.raids, func(i int, j int) bool { return g.raids[i].ID < g.raids[j].ID })
			g.connectToRaid(raid.ID)
		})
	}()
}

func (g *Game) connectToRaid(raidID string) {
	g.screen = screenConnecting
	g.status = "Подключаюсь к рейду..."
	g.connectToRaidAsync(g.token, raidID)
}

func (g *Game) connectToRaidAsync(token string, raidID string) {
	g.resetRaidState()
	g.currentRaidID = raidID
	classID := g.selectedClassID()
	go func() {
		if err := g.network.Connect(token, raidID, classID); err != nil {
			g.post(func() {
				g.screen = screenLobby
				g.status = err.Error()
			})
			return
		}
		g.post(func() {
			g.status = "Сокет открыт. Жду welcome и snapshot..."
		})
	}()
}

func (g *Game) resetRaidState() {
	g.pendingInput = nil
	g.interpolators = make(map[string]*Interpolator)
	g.localReady = false
	g.localID = ""
	g.nextSeq = 0
	g.lastAckSeq = 0
	g.localAttackCombo = 0
	g.predictedAnim = ""
	g.predictedAnimAt = 0
	g.predictedSeq = 0
	g.loot = nil
	g.currentRaid = shared.RaidState{}
	g.hasRaidState = false
	g.currentLayout = shared.RaidLayoutState{}
	g.hasLayout = false
	g.layoutSolids = nil
	g.layoutPlatforms = nil

}

func (g *Game) captureInput() shared.InputCommand {
	moveX := 0.0
	if g.controls.Pressed(ActionMoveLeft) {
		moveX -= 1
	}
	if g.controls.Pressed(ActionMoveRight) {
		moveX += 1
	}
	cursorX, cursorY := ebiten.CursorPosition()
	worldAim := g.screenToWorld(shared.Vec2{X: float64(cursorX), Y: float64(cursorY)})
	g.nextSeq++
	return shared.InputCommand{
		Seq:           g.nextSeq,
		MoveX:         moveX,
		Jump:          g.controls.Pressed(ActionJump),
		Dash:          g.controls.Pressed(ActionDash),
		PrimaryAttack: g.controls.JustPressed(ActionAttack),
		Skill1:        g.controls.JustPressed(ActionSkill1),
		Skill2:        g.controls.JustPressed(ActionSkill2),
		Skill3:        g.controls.JustPressed(ActionSkill3),
		Interact:      g.controls.JustPressed(ActionInteract),
		UseJumpLink:   g.controls.JustPressed(ActionUseJumpLink),
		DropDown:      g.controls.JustPressed(ActionDropDown),
		AimX:          worldAim.X,
		AimY:          worldAim.Y,
		ClientTime:    g.clientTime(),
	}
}

func (g *Game) drawLogin(screen *ebiten.Image) {
	screen.Fill(color.RGBA{12, 17, 25, 255})
	drawPanel(screen, shared.Rect{X: 120, Y: 84, W: 1040, H: 548}, color.RGBA{18, 28, 40, 230}, color.RGBA{70, 120, 170, 255}, 2)

	ebitenutil.DebugPrintAt(screen, "WarpedRealms Content-Driven Build", 160, 126)
	ebitenutil.DebugPrintAt(screen, "Mouse-first lobby/settings/editor pass. Click fields, tabs and buttons.", 160, 148)

	signUpRect := shared.Rect{X: 160, Y: 176, W: 160, H: 34}
	signInRect := shared.Rect{X: 332, Y: 176, W: 160, H: 34}
	drawButton(screen, signUpRect, g.authMode == "sign-up", uiHover(signUpRect))
	drawButton(screen, signInRect, g.authMode == "sign-in", uiHover(signInRect))
	ebitenutil.DebugPrintAt(screen, "SIGN UP", 206, 186)
	ebitenutil.DebugPrintAt(screen, "SIGN IN", 380, 186)

	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Mode: %s", strings.ToUpper(g.authMode)), 160, 224)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Server: %s", g.baseURL), 160, 232)

	g.drawTextField(screen, 160, 286, 960, 54, "Email", g.email, g.activeField == 0)
	g.drawTextField(screen, 160, 368, 960, 54, "Password", maskPassword(g.password), g.activeField == 1)

	submitRect := shared.Rect{X: 160, Y: 452, W: 220, H: 42}
	drawButton(screen, submitRect, false, uiHover(submitRect))
	ebitenutil.DebugPrintAt(screen, "Continue", 230, 464)
	ebitenutil.DebugPrintAt(screen, "Tab: switch field", 400, 454)
	ebitenutil.DebugPrintAt(screen, "F2: toggle sign-up/sign-in", 400, 476)
	ebitenutil.DebugPrintAt(screen, "F1: settings", 400, 498)
	ebitenutil.DebugPrintAt(screen, "Mouse: click tabs, fields and Continue", 400, 520)
	ebitenutil.DebugPrintAt(screen, g.status, 160, 562)
}

func (g *Game) drawLobby(screen *ebiten.Image) {
	screen.Fill(color.RGBA{10, 16, 24, 255})
	drawPanel(screen, shared.Rect{X: 56, Y: 54, W: 1168, H: 612}, color.RGBA{17, 27, 38, 235}, color.RGBA{70, 120, 170, 255}, 2)

	classLabel := "unknown"
	if classDef, ok := g.selectedClassDefinition(); ok {
		classLabel = classDef.Name
	}

	ebitenutil.DebugPrintAt(screen, "Raid Lobby", 88, 80)
	ebitenutil.DebugPrintAt(screen, "Click cards and raids. Double-click raid or use Join. Keyboard still works.", 88, 102)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Signed as %s", g.email), 88, 124)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Class: %s   Up/Down: change raid   F1: settings", classLabel), 88, 146)

	buttons := []struct {
		label  string
		rect   shared.Rect
		active bool
	}{
		{label: "Join", rect: shared.Rect{X: 884, Y: 92, W: 132, H: 40}, active: false},
		{label: "Create", rect: shared.Rect{X: 1028, Y: 92, W: 160, H: 40}, active: false},
		{label: "Refresh", rect: shared.Rect{X: 884, Y: 142, W: 132, H: 40}, active: false},
		{label: "Settings", rect: shared.Rect{X: 1028, Y: 142, W: 160, H: 40}, active: false},
		{label: "Logout", rect: shared.Rect{X: 884, Y: 192, W: 304, H: 40}, active: false},
	}
	for _, button := range buttons {
		drawButton(screen, button.rect, button.active, uiHover(button.rect))
		ebitenutil.DebugPrintAt(screen, button.label, int(button.rect.X)+18, int(button.rect.Y)+12)
	}

	y := 360
	if len(g.raids) == 0 {
		ebitenutil.DebugPrintAt(screen, "Нет доступных рейдов. Нажми N, чтобы создать новый.", 88, y)
	} else {
		for index, raid := range g.raids {
			fill := color.RGBA{20, 30, 42, 220}
			stroke := color.RGBA{60, 90, 120, 255}
			if index == g.selectedRaid {
				fill = color.RGBA{32, 48, 68, 240}
				stroke = color.RGBA{120, 190, 235, 255}
			}
			vector.DrawFilledRect(screen, 88, float32(y), 1100, 54, fill, false)
			vector.StrokeRect(screen, 88, float32(y), 1100, 54, 1, stroke, false)
			ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%s | %s", raid.ID, raid.Name), 108, y+10)
			ebitenutil.DebugPrintAt(screen, fmt.Sprintf("phase=%s  players=%d/%d  time=%.0fs / %.0fs  seed=%d", raid.Phase, raid.CurrentPlayers, raid.MaxPlayers, raid.TimeRemaining, raid.Duration, raid.Seed), 108, y+28)
			y += 66
		}
	}

	ebitenutil.DebugPrintAt(screen, g.status, 88, 638)
}

func (g *Game) drawGame(screen *ebiten.Image) {
	screen.Fill(color.RGBA{14, 20, 28, 255})
	activeRoomID := g.localPlayer.RoomID
	if activeRoomID == "" && len(g.currentLayout.Rooms) > 0 {
		activeRoomID = g.currentLayout.Rooms[0].ID
	}
	g.renderer.DrawScene(screen, g.currentLayout, activeRoomID, g.camera, g.viewOffset, g.worldScale, g.currentPreview(), g.revealBgRoomID(activeRoomID))
	g.drawLoot(screen)

	entities := make([]shared.EntityState, 0, len(g.interpolators)+3)
	if g.localReady {
		entities = append(entities, g.localPlayer.Clone())
	}
	// Add all active bots to the entity list.
	// DrawLowerEntities will pick up background-room bots (it filters by belowRoom.ID).
	// The foreground draw loop below skips entities not in the active room.
	for _, bot := range g.bots {
		if bot.IsActive() {
			entities = append(entities, bot.Entity())
		}
	}
	targetTime := g.estimatedServerTime() - shared.InterpolationBackTime
	for id, interpolator := range g.interpolators {
		if id == g.localID {
			continue
		}
		state, ok := interpolator.Sample(targetTime)
		if !ok {
			continue
		}
		entities = append(entities, state)
	}
	// Draw below-room entities (players + rats) as semi-transparent ghosts.
	// Use the same bgRoomID that DrawScene renders: RevealZone target or BelowRoomID.
	bgRoomID := g.revealBgRoomID(activeRoomID)
	if bgRoomID == "" {
		if ar, ok := g.currentLayout.RoomByID(activeRoomID); ok {
			bgRoomID = ar.BelowRoomID
		}
	}
	g.renderer.DrawLowerEntities(screen, bgRoomID, entities, g.camera, g.viewOffset, g.worldScale)

	sort.Slice(entities, func(i int, j int) bool { return entities[i].Position.Y < entities[j].Position.Y })
	for _, entity := range entities {
		if entity.RoomID != activeRoomID {
			continue // background-room entities rendered only via DrawLowerEntities
		}
		g.drawEntity(screen, entity)
	}
	if g.debugPhysics {
		g.drawPhysicsDebug(screen, activeRoomID, entities)
		if activeRoom, ok := g.currentLayout.RoomByID(activeRoomID); ok {
			g.renderer.DrawDebugOverlays(screen, activeRoom, g.camera, g.viewOffset, g.worldScale)
		}
	}
	g.drawRaidHUD(screen, len(entities))
	g.drawInteractionPrompt(screen, entities)
	g.drawInputOverlay(screen)
}

func (g *Game) drawInputOverlay(screen *ebiten.Image) {
	op := &ebiten.DrawImageOptions{}

	for _, k := range g.inputOverlayKeys {
		// 1. Рисуем фон кнопки (используя белый пиксель и масштабирование)
		op.GeoM.Reset()
		op.GeoM.Scale(k.rect.W, k.rect.H) // Растягиваем 1x1 до размеров кнопки
		op.GeoM.Translate(k.rect.X, k.rect.Y)

		// Выбираем цвет
		if g.controls.Pressed(k.action) {
			op.ColorScale.Reset()
			op.ColorScale.ScaleWithColor(color.RGBA{80, 150, 255, 200}) // Нажата
		} else {
			op.ColorScale.Reset()
			op.ColorScale.ScaleWithColor(color.RGBA{20, 20, 25, 150}) // Не нажата
		}
		screen.DrawImage(g.whitePixelImg, op)

		// 2. Рисуем обводку (чуть сложнее без vector, но можно просто нарисовать
		//    текст поверх. Если очень нужна обводка, лучше сделать её спрайтом.
		//    Для скорости просто рисуем пред-отрисованный текст)
		op.GeoM.Reset()
		op.GeoM.Translate(k.rect.X, k.rect.Y)
		op.ColorScale.Reset() // Текст всегда белый или серый
		screen.DrawImage(k.img, op)
	}
}

// revealBgRoomID returns the TargetRoomID of the first RevealZone that
// contains the local player's centre, or "" if none is found.
// This overrides the BelowRoomID background when the player approaches a portal.
func (g *Game) revealBgRoomID(activeRoomID string) string {
	if !g.localReady {
		return ""
	}
	room, ok := g.currentLayout.RoomByID(activeRoomID)
	if !ok {
		return ""
	}
	center := shared.EntityCenter(g.localPlayer)
	for _, zone := range room.RevealZones {
		if zone.Area.ContainsPoint(center) {
			return zone.TargetRoomID
		}
	}
	return ""
}

func (g *Game) drawPhysicsDebug(screen *ebiten.Image, activeRoomID string, entities []shared.EntityState) {
	room, ok := g.currentLayout.RoomByID(activeRoomID)
	if ok {
		// Draw solid collision rects (cyan fill + border).
		for _, solid := range room.Solids {
			sx := float32(g.viewOffset.X + (solid.X-g.camera.X)*g.worldScale)
			sy := float32(g.viewOffset.Y + (solid.Y-g.camera.Y)*g.worldScale)
			sw := float32(solid.W * g.worldScale)
			sh := float32(solid.H * g.worldScale)
			vector.DrawFilledRect(screen, sx, sy, sw, sh, color.RGBA{0, 200, 255, 40}, false)
			vector.StrokeRect(screen, sx, sy, sw, sh, 1, color.RGBA{0, 200, 255, 200}, false)
		}

		// Draw jump link areas (yellow).
		for _, link := range room.JumpLinks {
			sx := float32(g.viewOffset.X + (link.Area.X-g.camera.X)*g.worldScale)
			sy := float32(g.viewOffset.Y + (link.Area.Y-g.camera.Y)*g.worldScale)
			vector.StrokeRect(screen, sx, sy, float32(link.Area.W*g.worldScale), float32(link.Area.H*g.worldScale), 2, color.RGBA{255, 220, 0, 220}, false)
		}

		// Draw reveal zones (translucent white outline).
		for _, zone := range room.RevealZones {
			sx := float32(g.viewOffset.X + (zone.Area.X-g.camera.X)*g.worldScale)
			sy := float32(g.viewOffset.Y + (zone.Area.Y-g.camera.Y)*g.worldScale)
			vector.StrokeRect(screen, sx, sy, float32(zone.Area.W*g.worldScale), float32(zone.Area.H*g.worldScale), 1, color.RGBA{220, 220, 255, 140}, false)
		}

		// Draw rifts: color-coded, show capacity / used count.
		for _, rift := range room.Rifts {
			sx := float32(g.viewOffset.X + (rift.Area.X-g.camera.X)*g.worldScale)
			sy := float32(g.viewOffset.Y + (rift.Area.Y-g.camera.Y)*g.worldScale)
			sw := float32(rift.Area.W * g.worldScale)
			sh := float32(rift.Area.H * g.worldScale)
			clr := riftDebugColor(rift.Kind, rift.IsOpen())
			vector.StrokeRect(screen, sx, sy, sw, sh, 2, clr, false)
			ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%s %d/%d", rift.Kind, rift.UsedCount, rift.Capacity), int(sx), int(sy)-14)
		}

		// Draw spawn points as orange circles (stored in layout, generated for the first room).
		for i, spawn := range g.currentLayout.PlayerSpawns {
			sx := float32(g.viewOffset.X + (spawn.X-g.camera.X)*g.worldScale)
			sy := float32(g.viewOffset.Y + (spawn.Y-g.camera.Y)*g.worldScale)
			vector.DrawFilledCircle(screen, sx, sy, 8, color.RGBA{255, 140, 0, 180}, false)
			vector.StrokeCircle(screen, sx, sy, 8, 2, color.RGBA{255, 200, 80, 255}, false)
			ebitenutil.DebugPrintAt(screen, fmt.Sprintf("S%d", i+1), int(sx)+10, int(sy)-8)
		}

		// Draw entity physics capsules: green if grounded, red if airborne.
		for _, entity := range entities {
			bounds := shared.EntityBounds(entity)
			bx := g.viewOffset.X + (bounds.X-g.camera.X)*g.worldScale
			by := g.viewOffset.Y + (bounds.Y-g.camera.Y)*g.worldScale
			bw := bounds.W * g.worldScale
			bh := bounds.H * g.worldScale

			clr := color.RGBA{255, 60, 60, 220} // red = airborne
			fill := color.RGBA{255, 60, 60, 40}
			if entity.Grounded {
				clr = color.RGBA{60, 255, 80, 220} // green = grounded
				fill = color.RGBA{60, 255, 80, 40}
			}

			// Capsule: radius = half the width.
			r := float32(bw * 0.5)
			cx := float32(bx + bw*0.5)
			topY := float32(by + float64(r))
			botY := float32(by + bh - float64(r))

			// Fill: middle rect + two circles.
			vector.DrawFilledRect(screen, float32(bx), topY, float32(bw), botY-topY, fill, false)
			vector.DrawFilledCircle(screen, cx, topY, r, fill, true)
			vector.DrawFilledCircle(screen, cx, botY, r, fill, true)

			// Stroke: sides of the middle rect + two circle outlines.
			strokeW := float32(2)
			vector.StrokeCircle(screen, cx, topY, r, strokeW, clr, true)
			vector.StrokeCircle(screen, cx, botY, r, strokeW, clr, true)
			// Cover the flat edges where circles meet the rect (no stroke line in the middle).
			vector.StrokeLine(screen, float32(bx), topY, float32(bx), botY, strokeW, clr, false)
			vector.StrokeLine(screen, float32(bx+bw), topY, float32(bx+bw), botY, strokeW, clr, false)

			// Yellow dot = Position origin.
			px := float32(g.viewOffset.X + (entity.Position.X-g.camera.X)*g.worldScale)
			py := float32(g.viewOffset.Y + (entity.Position.Y-g.camera.Y)*g.worldScale)
			vector.DrawFilledCircle(screen, px, py, 4, color.RGBA{255, 255, 0, 255}, false)

			// Hurtbox — magenta rect (takes damage zone).
			hb := entity.Hurtbox
			if hb.W > 0 && hb.H > 0 {
				hx := float32(g.viewOffset.X + (entity.Position.X+hb.X-g.camera.X)*g.worldScale)
				hy := float32(g.viewOffset.Y + (entity.Position.Y+hb.Y-g.camera.Y)*g.worldScale)
				hw := float32(hb.W * g.worldScale)
				hh := float32(hb.H * g.worldScale)
				vector.StrokeRect(screen, hx, hy, hw, hh, 1, color.RGBA{220, 60, 255, 200}, false)
			}

			// Attack hitbox — orange rect when entity is actively attacking.
			if profile, ok := g.assets.Manifest.Profile(entity.ProfileID); ok {
				elapsed := g.animationElapsed(entity)
				if atk, active := profile.HitboxFor(entity.Animation, elapsed, entity.Facing, entity.Position); active {
					ax := float32(g.viewOffset.X + (atk.X-g.camera.X)*g.worldScale)
					ay := float32(g.viewOffset.Y + (atk.Y-g.camera.Y)*g.worldScale)
					aw := float32(atk.W * g.worldScale)
					ah := float32(atk.H * g.worldScale)
					vector.DrawFilledRect(screen, ax, ay, aw, ah, color.RGBA{255, 140, 0, 60}, false)
					vector.StrokeRect(screen, ax, ay, aw, ah, 2, color.RGBA{255, 140, 0, 240}, false)
					ebitenutil.DebugPrintAt(screen, "HIT", int(ax), int(ay)-14)
				}
			}

			// Label: show Y position and velocity.
			ebitenutil.DebugPrintAt(screen,
				fmt.Sprintf("y=%.0f vy=%.0f %s", entity.Position.Y, entity.Velocity.Y, map[bool]string{true: "GND", false: "AIR"}[entity.Grounded]),
				int(bx), int(by)-16)
		}
	}

	roomType := "unknown"
	if ok {
		roomType = formatRoomType(room, len(g.currentLayout.Rooms))
	}
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Type: %s", roomType), 16, shared.ScreenHeight-40)

	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("[F3] debug  [F4] sprite-rect=%t  cyan=portal  white=reveal-zone  rift=R/B/G  orange●=spawn", g.useSpriteRectRender), 16, shared.ScreenHeight-22)
}

func (g *Game) drawLoot(screen *ebiten.Image) {
	for _, loot := range g.loot {
		frame := g.assets.IdleFrame(loot.ProfileID)
		screenPos := g.worldToScreen(loot.Position)
		if frame != nil {
			op := &ebiten.DrawImageOptions{}
			scale := 28.0 * g.worldScale / float64(frame.Bounds().Dy())
			op.GeoM.Scale(scale, scale)
			op.GeoM.Translate(screenPos.X, screenPos.Y)
			screen.DrawImage(frame, op)
		} else {
			vector.DrawFilledRect(screen, float32(screenPos.X), float32(screenPos.Y), float32(18*g.worldScale), float32(18*g.worldScale), color.RGBA{220, 190, 70, 255}, false)
		}
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%d", loot.Value), int(screenPos.X), int(screenPos.Y)-14)
	}
}

func (g *Game) drawRaidHUD(screen *ebiten.Image, entityCount int) {
	vector.DrawFilledRect(screen, 16, 16, 520, 178, color.RGBA{0, 0, 0, 160}, false)
	vector.StrokeRect(screen, 16, 16, 520, 178, 1, color.RGBA{80, 140, 190, 255}, false)

	localStatus := shared.PlayerRaidStatusWaiting
	loot := 0
	exitTag := ""
	hp := 0
	maxHP := 1
	roomID := g.localPlayer.RoomID
	cooldowns := []shared.AbilityCooldown{}
	for _, player := range g.currentRaid.Players {
		if player.PlayerID == g.localID {
			localStatus = player.Status
			loot = player.CarriedLoot
			exitTag = player.AssignedExitTag
			hp = player.HP
			maxHP = player.MaxHP
			roomID = player.CurrentRoomID
			cooldowns = player.Cooldowns
			break
		}
	}

	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Raid: %s (%s)", g.currentRaid.Name, g.currentRaid.RaidID), 28, 28)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Phase: %s  Time: %.0fs / %.0fs  Seed: %d", g.currentRaid.Phase, g.currentRaid.TimeRemaining, g.currentRaid.Duration, g.currentRaid.Seed), 28, 46)
	roomLine := fmt.Sprintf("Room: %s  Status: %s", roomID, localStatus)
	if g.debugPhysics {
		roomType := "unknown"
		if room, ok := g.currentLayout.RoomByID(roomID); ok {
			roomType = formatRoomType(room, len(g.currentLayout.Rooms))
		}
		roomLine = fmt.Sprintf("Room: %s  Type: %s  Status: %s", roomID, roomType, localStatus)
	}
	ebitenutil.DebugPrintAt(screen, roomLine, 28, 64)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("HP: %d/%d  Loot: %d  Exit: %s", hp, maxHP, loot, exitTag), 28, 82)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Ping: %.0f ms  Pending: %d  Ack: %d  Entities: %d", g.pingMS, len(g.pendingInput), g.lastAckSeq, entityCount), 28, 100)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Attack %s  Skills %s/%s/%s  Interact %s  JumpLink %s", BindingLabel(g.controls, ActionAttack), BindingLabel(g.controls, ActionSkill1), BindingLabel(g.controls, ActionSkill2), BindingLabel(g.controls, ActionSkill3), BindingLabel(g.controls, ActionInteract), BindingLabel(g.controls, ActionUseJumpLink)), 28, 118)

	y := 138
	for _, cooldown := range cooldowns {
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%s: %.1fs", cooldown.Name, cooldown.Remaining), 28, y)
		y += 16
	}

	if localStatus != shared.PlayerRaidStatusActive && g.hasRaidState {
		vector.DrawFilledRect(screen, 296, 288, 688, 116, color.RGBA{0, 0, 0, 190}, false)
		vector.StrokeRect(screen, 296, 288, 688, 116, 2, color.RGBA{140, 190, 230, 255}, false)
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Рейд завершён для тебя: %s", strings.ToUpper(string(localStatus))), 328, 320)
		ebitenutil.DebugPrintAt(screen, "Нажми Esc, чтобы вернуться в lobby и запустить новый seed.", 328, 352)
	}
}

func (g *Game) drawInteractionPrompt(screen *ebiten.Image, entities []shared.EntityState) {
	if !g.localReady || !g.hasLayout {
		return
	}
	room, ok := g.currentLayout.RoomByID(g.localPlayer.RoomID)
	if !ok {
		return
	}

	localCenter := shared.EntityCenter(g.localPlayer)
	localBounds := shared.InteractionBounds(g.localPlayer).Inflate(18, 18)
	var prompt string

	for _, exit := range room.Exits {
		if exit.Area.ContainsPoint(localCenter) || exit.Area.Intersects(localBounds) {
			prompt = fmt.Sprintf("%s Extract at %s", BindingLabel(g.controls, ActionInteract), exit.Label)
			break
		}
	}
	if prompt == "" {
		for _, loot := range g.loot {
			if loot.RoomID != g.localPlayer.RoomID {
				continue
			}
			if localCenter.Sub(loot.Position).Length() > 84 {
				continue
			}
			prompt = fmt.Sprintf("%s Pick up loot", BindingLabel(g.controls, ActionInteract))
			break
		}
	}
	if prompt == "" {
		for _, entity := range entities {
			if entity.ID == g.localID || entity.RoomID != g.localPlayer.RoomID {
				continue
			}
			if !shared.InteractionBounds(entity).Inflate(18, 18).ContainsPoint(localCenter) {
				continue
			}
			prompt = fmt.Sprintf("%s Open chest", BindingLabel(g.controls, ActionInteract))
			break
		}
	}
	if prompt == "" {
		for _, link := range room.JumpLinks {
			if link.Area.ContainsPoint(localCenter) || link.Area.Intersects(localBounds) {
				prompt = fmt.Sprintf("%s Portal: %s", BindingLabel(g.controls, ActionUseJumpLink), link.Label)
				break
			}
		}
	}
	if prompt == "" {
		for _, rift := range room.Rifts {
			if !rift.IsOpen() {
				continue
			}
			if rift.Area.ContainsPoint(localCenter) || rift.Area.Intersects(localBounds) {
				remaining := rift.Capacity - rift.UsedCount
				prompt = fmt.Sprintf("%s Rift [%s] — %d use(s) left", BindingLabel(g.controls, ActionUseJumpLink), rift.Kind, remaining)
				break
			}
		}
	}
	if prompt == "" {
		return
	}

	rect := shared.Rect{X: 430, Y: 642, W: 420, H: 40}
	drawPanel(screen, rect, color.RGBA{8, 12, 18, 220}, color.RGBA{94, 146, 188, 255}, 1)
	ebitenutil.DebugPrintAt(screen, prompt, int(rect.X)+18, int(rect.Y)+12)
}

func (g *Game) drawEntity(screen *ebiten.Image, entity shared.EntityState) {
	if composite, ok := g.assets.Composites[entity.ProfileID]; ok {
		g.drawCompositeEntity(screen, entity, composite)
		return
	}
	animation := g.assets.AnimationFor(entity)
	frame := animation.FrameAt(g.animationElapsed(entity))
	if frame == nil {
		return
	}
	g.drawEntityFrame(screen, entity, frame, g.assets.Layers[entity.ProfileID])
}

func (g *Game) drawCompositeEntity(screen *ebiten.Image, entity shared.EntityState, composite CompositeAsset) {
	spritePos := entity.Position.Add(entity.SpriteOffset)
	screenPos := g.worldToScreen(spritePos)
	baseScale := g.worldScale * maxf(0.6, entity.Scale)
	elapsed := g.animationElapsed(entity)
	bob := math.Sin(elapsed*4.0) * 4 * baseScale
	if entity.Animation == shared.AnimationTravel {
		bob -= math.Sin(elapsed*7.0) * 12 * baseScale
	}

	parts := []LayerSprite{
		composite.LeftLeg,
		composite.RightLeg,
		composite.LeftArm,
		composite.RightArm,
		composite.Body,
		composite.Detail,
		composite.Eyes,
		composite.Mouth,
	}
	for _, part := range parts {
		if part.Image == nil {
			continue
		}
		op := &ebiten.DrawImageOptions{}
		partScale := baseScale * maxf(0.4, part.Scale)
		if entity.Facing < 0 {
			op.GeoM.Scale(-partScale, partScale)
			op.GeoM.Translate(float64(part.Image.Bounds().Dx())*partScale, 0)
		} else {
			op.GeoM.Scale(partScale, partScale)
		}
		op.GeoM.Translate(screenPos.X+part.Offset.X*baseScale, screenPos.Y+part.Offset.Y*baseScale+bob)
		screen.DrawImage(part.Image, op)
	}
	g.drawEntityEffects(screen, entity, spritePos, shared.Vec2{X: 220 * entity.Scale, Y: 260 * entity.Scale})
	g.drawEntityLabel(screen, entity, screenPos)
}

func (g *Game) drawEntityFrame(screen *ebiten.Image, entity shared.EntityState, frame *ebiten.Image, layers []LayerSprite) {
	spritePos := entity.Position.Add(entity.SpriteOffset)
	screenPos := g.worldToScreen(spritePos)
	spriteSize := entity.SpriteSize.Mul(g.worldScale * maxf(0.6, entity.Scale))

	op := &ebiten.DrawImageOptions{}
	scaleX := spriteSize.X / float64(frame.Bounds().Dx())
	scaleY := spriteSize.Y / float64(frame.Bounds().Dy())
	if entity.Facing < 0 {
		op.GeoM.Scale(-scaleX, scaleY)
		op.GeoM.Translate(spriteSize.X, 0)
	} else {
		op.GeoM.Scale(scaleX, scaleY)
	}
	op.GeoM.Translate(screenPos.X, screenPos.Y)
	screen.DrawImage(frame, op)

	for _, layer := range layers {
		if layer.Image == nil {
			continue
		}
		layerOp := &ebiten.DrawImageOptions{}
		layerScale := g.worldScale * maxf(0.3, layer.Scale)
		if entity.Facing < 0 {
			layerOp.GeoM.Scale(-layerScale, layerScale)
			layerOp.GeoM.Translate(float64(layer.Image.Bounds().Dx())*layerScale, 0)
			layerOp.GeoM.Translate(screenPos.X+spriteSize.X-layer.Offset.X*g.worldScale-float64(layer.Image.Bounds().Dx())*layerScale*0.7, screenPos.Y+layer.Offset.Y*g.worldScale)
		} else {
			layerOp.GeoM.Scale(layerScale, layerScale)
			layerOp.GeoM.Translate(screenPos.X+layer.Offset.X*g.worldScale, screenPos.Y+layer.Offset.Y*g.worldScale)
		}
		screen.DrawImage(layer.Image, layerOp)
	}

	g.drawEntityEffects(screen, entity, spritePos, entity.SpriteSize)
	g.drawEntityLabel(screen, entity, screenPos)
}

func (g *Game) drawEntityLabel(screen *ebiten.Image, entity shared.EntityState, screenPos shared.Vec2) {
	if entity.Name == "" {
		return
	}
	label := entity.Name
	if entity.HP > 0 {
		label = fmt.Sprintf("%s [%d]", entity.Name, entity.HP)
	}
	ebitenutil.DebugPrintAt(screen, label, int(screenPos.X), int(screenPos.Y)-14)
}

func (g *Game) drawEntityEffects(screen *ebiten.Image, entity shared.EntityState, spritePos shared.Vec2, spriteSize shared.Vec2) {
	center := g.worldToScreen(shared.EntityCenter(entity))
	elapsed := g.animationElapsed(entity)

	switch entity.Animation {
	case shared.AnimationAttack1, shared.AnimationAttack2, shared.AnimationAttack3:
		if frame := g.assets.FX.Slash.FrameAt(elapsed); frame != nil {
			g.drawEffectFrame(screen, frame, center.X+entity.Facing*52*g.worldScale, center.Y-36*g.worldScale, 0.3*g.worldScale, elapsed, entity.Facing, 0.9)
		}
	case shared.AnimationSkill1, shared.AnimationSkill2, shared.AnimationSkill3:
		if frame := g.assets.FX.Slash.FrameAt(elapsed); frame != nil {
			g.drawEffectFrame(screen, frame, center.X+entity.Facing*44*g.worldScale, center.Y-40*g.worldScale, 0.36*g.worldScale, elapsed, entity.Facing, 0.95)
		}
	}

	if entity.Travel != nil && entity.Travel.Active {
		travelAlpha := 0.55 + 0.35*math.Sin(elapsed*11)
		g.drawEffectImage(screen, g.assets.FX.Portal, center.X-18*g.worldScale, center.Y+12*g.worldScale, 0.55*g.worldScale, elapsed, entity.Facing, travelAlpha)
		g.drawEffectImage(screen, g.assets.FX.PortalGlow, center.X-8*g.worldScale, center.Y-8*g.worldScale, 0.72*g.worldScale, elapsed, entity.Facing, 0.65)
		g.drawEffectImage(screen, g.assets.FX.Smoke, center.X-20*g.worldScale, center.Y+28*g.worldScale, 0.52*g.worldScale, elapsed, entity.Facing, 0.48)
		g.drawEffectImage(screen, g.assets.FX.TravelFlame, center.X+entity.Facing*14*g.worldScale, center.Y-18*g.worldScale, 0.42*g.worldScale, elapsed, entity.Facing, 0.45)
	}

	_ = spritePos
	_ = spriteSize
}

func (g *Game) drawEffectImage(screen *ebiten.Image, image *ebiten.Image, x float64, y float64, scale float64, elapsed float64, facing float64, alpha float64) {
	if image == nil {
		return
	}
	g.drawEffectFrame(screen, image, x, y, scale, elapsed, facing, alpha)
}

func (g *Game) drawEffectFrame(screen *ebiten.Image, image *ebiten.Image, x float64, y float64, scale float64, elapsed float64, facing float64, alpha float64) {
	if image == nil {
		return
	}
	scale = maxf(0.08, scale)
	op := &ebiten.DrawImageOptions{}
	if facing < 0 {
		op.GeoM.Scale(-scale, scale)
		op.GeoM.Translate(float64(image.Bounds().Dx())*scale, 0)
	} else {
		op.GeoM.Scale(scale, scale)
	}
	op.GeoM.Translate(x, y+math.Sin(elapsed*12)*4*g.worldScale)
	op.ColorScale.Scale(1, 1, 1, float32(alpha))
	screen.DrawImage(image, op)
}

func (g *Game) drawTextField(screen *ebiten.Image, x float32, y float32, width float32, height float32, label string, value string, active bool) {
	fill := color.RGBA{20, 30, 42, 255}
	stroke := color.RGBA{60, 90, 120, 255}
	if active {
		fill = color.RGBA{28, 44, 62, 255}
		stroke = color.RGBA{110, 180, 220, 255}
	}
	vector.DrawFilledRect(screen, x, y, width, height, fill, false)
	vector.StrokeRect(screen, x, y, width, height, 2, stroke, false)
	ebitenutil.DebugPrintAt(screen, label, int(x)+12, int(y)+10)
	ebitenutil.DebugPrintAt(screen, value, int(x)+12, int(y)+30)
}

func (g *Game) drainUIEvents() {
	for {
		select {
		case event := <-g.uiEvents:
			event()
		default:
			return
		}
	}
}

func (g *Game) drainNetwork() {
	for {
		select {
		case welcome := <-g.network.WelcomeCh:
			g.localID = welcome.PlayerID
			g.currentRaidID = welcome.RaidID
			g.syncSelectedClassByID(welcome.ClassID)
			if classDef, ok := g.bundle.Manifest.Class(welcome.ClassID); ok {
				g.status = fmt.Sprintf("Подключён к %s как %s. Жду состояние рейда...", welcome.RaidName, classDef.Name)
			} else {
				g.status = fmt.Sprintf("Подключён к %s. Жду состояние рейда...", welcome.RaidName)
			}
			g.lastServerTS = welcome.ServerTime
			g.lastServerAt = time.Now()
		case snapshot := <-g.network.SnapshotCh:
			g.applySnapshot(snapshot)
		case pong := <-g.network.PongCh:
			now := g.clientTime()
			g.pingMS = (now - pong.ClientTime) * 500
		case err := <-g.network.ErrCh:
			g.network.Close()
			g.resetRaidState()
			if g.token == "" {
				g.screen = screenLogin
			} else {
				g.screen = screenLobby
			}
			g.status = fmt.Sprintf("Сеть: %v", err)
		default:
			return
		}
	}
}

func (g *Game) applySnapshot(snapshot shared.SnapshotMessage) {
	g.lastServerTS = snapshot.ServerTime
	g.lastServerAt = time.Now()
	g.localID = snapshot.LocalPlayerID
	g.lastAckSeq = snapshot.LastProcessedSeq
	g.loot = snapshot.Loot
	if snapshot.Raid != nil {
		g.currentRaid = *snapshot.Raid
		g.hasRaidState = true
	}
	if snapshot.Layout != nil {
		g.currentLayout = *snapshot.Layout
		g.hasLayout = true
		g.layoutSolids = g.buildLayoutSolids(g.currentLayout)
		g.layoutPlatforms = g.buildLayoutPlatforms(g.currentLayout)
	}

	seen := make(map[string]bool, len(snapshot.Entities))
	for _, entity := range snapshot.Entities {
		seen[entity.ID] = true
		if entity.ID == snapshot.LocalPlayerID {
			g.reconcileLocal(entity, snapshot.LastProcessedSeq)
			continue
		}
		interpolator := g.interpolators[entity.ID]
		if interpolator == nil {
			interpolator = &Interpolator{}
			g.interpolators[entity.ID] = interpolator
		}
		interpolator.Push(snapshot.ServerTime, entity)
	}

	for id := range g.interpolators {
		if !seen[id] {
			delete(g.interpolators, id)
		}
	}

	g.screen = screenGame
	g.updateCamera()
	if g.hasRaidState {
		switch g.currentRaid.LocalStatus {
		case shared.PlayerRaidStatusActive:
			g.status = "Рейд активен. Сражайся, лутай и уходи через свой exit."
		case shared.PlayerRaidStatusExtracted:
			g.status = "Ты успешно покинул рейд."
		case shared.PlayerRaidStatusEliminated:
			g.status = "Ты погиб и потерял добычу."
		case shared.PlayerRaidStatusExpired:
			g.status = "Таймер истёк. Добыча потеряна."
		}
	}
}

func (g *Game) reconcileLocal(authoritative shared.EntityState, ackSeq uint32) {
	g.localPlayer = authoritative.Clone()
	g.localReady = true

	keepIndex := 0
	for keepIndex < len(g.pendingInput) && g.pendingInput[keepIndex].Seq <= ackSeq {
		keepIndex++
	}
	if keepIndex > 0 {
		g.pendingInput = append([]shared.InputCommand(nil), g.pendingInput[keepIndex:]...)
	}
	now := g.estimatedServerTime()
	if g.predictedSeq > ackSeq && g.predictedAnim != "" && now < g.predictedAnimAt+shared.AnimationDuration(g.localPlayer, g.predictedAnim) {
		shared.TriggerAnimation(&g.localPlayer, g.predictedAnim, g.predictedAnimAt)
	}
	for _, command := range g.pendingInput {
		if g.localPlayer.Travel == nil || !g.localPlayer.Travel.Active {
			shared.SimulatePlayer(&g.localPlayer, command, g.solidsForPlayer(), g.platformsForPlayer())
		}
	}
	shared.RefreshAnimation(&g.localPlayer, now)
}

func (g *Game) updateCamera() {
	if !g.localReady || !g.hasLayout {
		return
	}
	room, ok := g.currentLayout.RoomByID(g.localPlayer.RoomID)
	if !ok {
		if len(g.currentLayout.Rooms) == 0 {
			return
		}
		room = g.currentLayout.Rooms[0]
	}
	center := shared.EntityCenter(g.localPlayer)
	visibleWidth := float64(shared.ScreenWidth) / g.worldScale
	visibleHeight := float64(shared.ScreenHeight) / g.worldScale

	// Allow the camera to travel slightly beyond the room's stated bounds so
	// it can follow the player even if they clip past the level edge before the
	// floor/wall boundary stops them.
	const cameraPad = 512.0
	minX := room.Bounds.X - cameraPad
	minY := room.Bounds.Y - cameraPad
	maxX := math.Max(minX, room.Bounds.Right()+cameraPad-visibleWidth)
	maxY := math.Max(minY, room.Bounds.Bottom()+cameraPad-visibleHeight)

	g.camera.X = clamp(center.X-visibleWidth*0.5, minX, maxX)
	g.camera.Y = clamp(center.Y-visibleHeight*0.5, minY, maxY)
	g.updateViewOffset()
}

func (g *Game) updateViewOffset() {
	g.viewOffset = shared.Vec2{}
}

func (g *Game) currentPreview() *roomPreview {
	if !g.localReady || !g.hasLayout {
		return nil
	}
	room, ok := g.currentLayout.RoomByID(g.localPlayer.RoomID)
	if !ok {
		return nil
	}
	playerCenter := shared.EntityCenter(g.localPlayer)
	for _, zone := range room.RevealZones {
		if !zone.Area.Inflate(120, 120).ContainsPoint(playerCenter) {
			continue
		}
		targetRoom, ok := g.currentLayout.RoomByID(zone.TargetRoomID)
		if !ok {
			continue
		}
		alpha := 1 - clamp(playerCenter.Sub(zone.Area.Center()).Length()/math.Max(zone.Area.W, zone.Area.H), 0, 1)
		alpha = 0.18 + alpha*0.72
		previewRect := zone.Area
		targetPos := targetRoom.Bounds.Center()
		for _, link := range room.JumpLinks {
			if link.TargetRoomID == zone.TargetRoomID {
				previewRect = link.PreviewRect
				targetPos = link.Arrival
				break
			}
		}
		return &roomPreview{
			room:   targetRoom,
			rect:   previewRect,
			alpha:  alpha,
			target: targetPos,
		}
	}
	return nil
}

// buildLayoutSolids builds a per-room solid map so physics never crosses
// room boundaries.  Each room's entities only collide against their own terrain.
func (g *Game) buildLayoutSolids(layout shared.RaidLayoutState) map[string][]shared.Rect {
	m := make(map[string][]shared.Rect, len(layout.Rooms))
	for _, room := range layout.Rooms {
		m[room.ID] = room.Solids
	}
	return m
}

// buildLayoutPlatforms builds a per-room one-way platform map.
func (g *Game) buildLayoutPlatforms(layout shared.RaidLayoutState) map[string][]shared.Rect {
	m := make(map[string][]shared.Rect, len(layout.Rooms))
	for _, room := range layout.Rooms {
		m[room.ID] = room.Platforms
	}
	return m
}

// solidsForPlayer returns the collision slice for the room the local player
// is currently in, or nil if not yet known (safe — SimulatePlayer handles nil).
func (g *Game) solidsForPlayer() []shared.Rect {
	if g.layoutSolids == nil {
		return nil
	}
	return g.layoutSolids[g.localPlayer.RoomID]
}

// platformsForPlayer returns the one-way platform slice for the current room.
func (g *Game) platformsForPlayer() []shared.Rect {
	if g.layoutPlatforms == nil {
		return nil
	}
	return g.layoutPlatforms[g.localPlayer.RoomID]
}

func (g *Game) worldToScreen(position shared.Vec2) shared.Vec2 {
	return shared.Vec2{
		X: g.viewOffset.X + (position.X-g.camera.X)*g.worldScale,
		Y: g.viewOffset.Y + (position.Y-g.camera.Y)*g.worldScale,
	}
}

func (g *Game) screenToWorld(position shared.Vec2) shared.Vec2 {
	return shared.Vec2{
		X: g.camera.X + (position.X-g.viewOffset.X)/g.worldScale,
		Y: g.camera.Y + (position.Y-g.viewOffset.Y)/g.worldScale,
	}
}

func (g *Game) estimatedServerTime() float64 {
	if g.lastServerAt.IsZero() {
		return g.clientTime()
	}
	return g.lastServerTS + time.Since(g.lastServerAt).Seconds()
}

func (g *Game) animationElapsed(entity shared.EntityState) float64 {
	return g.estimatedServerTime() - entity.AnimationStartedAt
}

func (g *Game) clientTime() float64 {
	return time.Since(g.startedAt).Seconds()
}

func (g *Game) post(event func()) {
	select {
	case g.uiEvents <- event:
	default:
	}
}

func deleteLastRune(value string) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	return string(runes[:len(runes)-1])
}

func maskPassword(value string) string {
	if value == "" {
		return ""
	}
	return strings.Repeat("*", len([]rune(value)))
}

func clamp(value float64, minValue float64, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func formatRoomType(room shared.RoomState, totalRooms int) string {
	ring := string(room.EffectiveRingZone(totalRooms))
	if ring == "" {
		ring = "unknown"
	}
	if room.IsThrone || room.EffectiveRingZone(totalRooms) == shared.RingZoneThrone {
		ring += " (throne)"
	}
	biome := strings.TrimSpace(room.Biome)
	if biome == "" {
		biome = "unknown"
	}
	return fmt.Sprintf("ring=%s biome=%s", ring, biome)
}

// riftDebugColor returns a debug outline colour for a rift based on kind and open state.
func riftDebugColor(kind shared.RiftKind, isOpen bool) color.RGBA {
	if !isOpen {
		return color.RGBA{80, 80, 80, 160} // closed — grey
	}
	switch kind {
	case shared.RiftKindRed:
		return color.RGBA{255, 60, 60, 220}
	case shared.RiftKindBlue:
		return color.RGBA{80, 140, 255, 220}
	case shared.RiftKindGreen:
		return color.RGBA{60, 210, 90, 220}
	}
	return color.RGBA{200, 200, 200, 200}
}
