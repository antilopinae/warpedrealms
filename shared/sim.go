package shared

import "math"

type MovementConfig struct {
	MoveSpeed    float64
	GroundAccel  float64
	AirAccel     float64
	Friction     float64
	Gravity      float64
	JumpSpeed    float64
	MaxFallSpeed float64
}

func PlayerMovementConfig() MovementConfig {
	return MovementConfig{
		MoveSpeed:    192,
		GroundAccel:  1640,
		AirAccel:     1120,
		Friction:     1745,
		Gravity:      2050,
		JumpSpeed:    835,
		MaxFallSpeed: 1220,
	}
}

func EnemyMovementConfig(kind EntityKind) MovementConfig {
	switch kind {
	case EntityKindBoss:
		return MovementConfig{
			MoveSpeed:    120,
			GroundAccel:  900,
			AirAccel:     700,
			Friction:     1000,
			Gravity:      1800,
			JumpSpeed:    480,
			MaxFallSpeed: 950,
		}
	default:
		return MovementConfig{
			MoveSpeed:    120,
			GroundAccel:  1200,
			AirAccel:     830,
			Friction:     1480,
			Gravity:      1900,
			JumpSpeed:    0,
			MaxFallSpeed: 980,
		}
	}
}

// SimulatePlayer runs one physics tick for the local player.
// solids block all movement; platforms are one-way: the player passes through
// them when moving upward and lands on them when falling from above.
func SimulatePlayer(state *EntityState, input InputCommand, solids []Rect, platforms []Rect) {
	simulateEntity(state, input, solids, platforms, PlayerMovementConfig())
}

// SimulateEnemy runs one physics tick for an NPC.
// Enemies are not affected by one-way platforms (they never jump through them),
// so platforms is nil for all NPC calls.
func SimulateEnemy(state *EntityState, moveX float64, jump bool, solids []Rect) {
	simulateEntity(state, InputCommand{MoveX: moveX, Jump: jump, DropDown: false}, solids, nil, EnemyMovementConfig(state.Kind))
}

func simulateEntity(state *EntityState, input InputCommand, solids []Rect, platforms []Rect, cfg MovementConfig) {
	if state.Travel != nil && state.Travel.Active {
		return
	}

	dt := FixedDeltaSeconds

	if state.DashTimer > 0 {
		state.DashTimer -= dt
	}
	if state.DashCooldown > 0 {
		state.DashCooldown -= dt
	}

	if input.Dash && state.DashCooldown <= 0 {
		state.DashTimer = 0.10
		state.DashCooldown = 1.1
		state.Velocity.Y = 0
	}

	if state.DashTimer > 0 {
		// Во время рывка скорость фиксирована, гравитация не действует
		dashSpeed := 720.0
		state.Velocity.X = state.Facing * dashSpeed
		state.Velocity.Y = 0
	} else {
		moveX := clamp(input.MoveX, -1, 1)
		targetVelocityX := moveX * cfg.MoveSpeed
		acceleration := cfg.GroundAccel
		if !state.Grounded {
			acceleration = cfg.AirAccel
		}

		if moveX == 0 {
			state.Velocity.X = moveToward(state.Velocity.X, 0, cfg.Friction*dt)
		} else {
			state.Velocity.X = moveToward(state.Velocity.X, targetVelocityX, acceleration*dt)
			state.Facing = math.Copysign(1, moveX)
		}

		if cfg.JumpSpeed > 0 && input.Jump && state.Grounded {
			state.Velocity.Y = -cfg.JumpSpeed
			state.Grounded = false
		}

		actualGravity := cfg.Gravity

		// Если игрок летит ВВЕРХ (Velocity.Y < 0)
		if state.Velocity.Y < 0 {
			if !input.Jump {
				// Если кнопку ОТПУСТИЛИ раньше времени — увеличиваем гравитацию в 3 раза.
				// Это заставит персонажа быстрее достичь пика и упасть (короткий прыжок).
				actualGravity *= 2.15
			} else {
				// Если кнопку ДЕРЖАТ — можем сделать гравитацию чуть слабее (0.8),
				// чтобы прыжок казался более "парящим" и высоким.
				actualGravity *= 0.85
			}
		}
		state.Velocity.Y += actualGravity * dt
	}

	if state.Velocity.Y > cfg.MaxFallSpeed {
		state.Velocity.Y = cfg.MaxFallSpeed
	}

	state.Position.X += state.Velocity.X * dt
	resolveHorizontal(state, solids)
	// Platforms never block horizontal movement.

	/*
		Это идеальный порядок.
		resolveHorizontal приподнимет игрока на ступеньку, а resolveVertical,
		который идет следом, приземлит его точно на её поверхность в том же самом кадре.
		Игрок даже не заметит микро-телепортации вверх, движение будет выглядеть абсолютно плавным.
	*/

	dy := state.Velocity.Y * dt // vertical displacement this tick (pixels)
	state.Position.Y += dy
	resolveVertical(state, solids)
	resolveOneWayPlatforms(state, platforms, dy, input.DropDown)
}

func resolveHorizontal(state *EntityState, colliders []Rect) {
	bounds := EntityBounds(*state)
	const maxStepHeight = 18.0

	var nearby []Rect
	searchArea := bounds.Inflate(math.Abs(state.Velocity.X)*0.2+10, maxStepHeight)
	for _, c := range colliders {
		if searchArea.Intersects(c) {
			nearby = append(nearby, c)
		}
	}

	for _, collider := range colliders {
		if !bounds.Intersects(collider) {
			continue
		}

		// Мы проверяем подъем только если движемся "в стену"
		if state.Grounded && state.Velocity.Y >= 0 {
			originalY := state.Position.Y
			state.Position.Y -= maxStepHeight
			liftedBounds := EntityBounds(*state)

			// Проверяем столкновение поднятых границ ТОЛЬКО с nearby списком
			canStep := true
			for _, c := range nearby {
				if liftedBounds.Intersects(c) {
					canStep = false
					break
				}
			}

			if canStep {
				// Успешный шаг: оставляем новую высоту Y.
				// Bounds обновляем, чтобы текущий цикл видел новую позицию.
				bounds = liftedBounds
				continue
			} else {
				state.Position.Y = originalY
			}
		}

		// Обычная логика столкновения со стеной
		if state.Velocity.X > 0 {
			state.Position.X = collider.X - state.Collider.X - bounds.W
		} else if state.Velocity.X < 0 {
			state.Position.X = collider.Right() - state.Collider.X
		}

		state.Velocity.X = 0
		bounds = EntityBounds(*state)
	}
}

func resolveVertical(state *EntityState, colliders []Rect) {
	state.Grounded = false
	bounds := EntityBounds(*state)
	for _, collider := range colliders {
		if !bounds.Intersects(collider) {
			continue
		}
		if state.Velocity.Y > 0 {
			state.Position.Y = collider.Y - state.Collider.Y - bounds.H
			state.Grounded = true
		} else if state.Velocity.Y < 0 {
			state.Position.Y = collider.Bottom() - state.Collider.Y
		}
		state.Velocity.Y = 0
		bounds = EntityBounds(*state)
	}
}

// resolveOneWayPlatforms applies one-way platform collision: the player can
// jump upward through a platform and land on top when falling down from above.
//
// dy is the vertical pixel displacement that was applied this tick
// (= Velocity.Y * dt, before resolveVertical). A positive dy means falling.
// The "was above" check: if the player's feet were above the platform top
// before this tick's move, and now they're overlapping, we land.
func resolveOneWayPlatforms(state *EntityState, platforms []Rect, dy float64, dropping bool) {
	if len(platforms) == 0 || state.Grounded || dropping {
		// Already grounded on solid terrain — no need to check platforms.
		return
	}
	bounds := EntityBounds(*state)
	for _, p := range platforms {
		if !bounds.Intersects(p) {
			continue
		}
		if state.Velocity.Y < 0 {
			// Moving upward — pass through platform.
			continue
		}
		// Moving downward (or stationary after impact).
		// Only land if feet were at or above the platform top before this tick.
		prevFeetY := bounds.Y + bounds.H - dy
		if prevFeetY > p.Y {
			// Feet were below platform top before move — came from below, skip.
			continue
		}
		state.Position.Y = p.Y - state.Collider.Y - bounds.H
		state.Velocity.Y = 0
		state.Grounded = true
		bounds = EntityBounds(*state)
	}
}

func moveToward(current float64, target float64, maxDelta float64) float64 {
	if math.Abs(target-current) <= maxDelta {
		return target
	}
	if target > current {
		return current + maxDelta
	}
	return current - maxDelta
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
