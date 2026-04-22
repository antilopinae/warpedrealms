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

func SimulatePlayer(state *EntityState, input InputCommand, colliders []Rect) {
	simulateEntity(state, input, colliders, PlayerMovementConfig())
}

func SimulateEnemy(state *EntityState, moveX float64, jump bool, colliders []Rect) {
	simulateEntity(state, InputCommand{MoveX: moveX, Jump: jump}, colliders, EnemyMovementConfig(state.Kind))
}

func simulateEntity(state *EntityState, input InputCommand, colliders []Rect, cfg MovementConfig) {
	if state.Travel != nil && state.Travel.Active {
		return
	}

	dt := FixedDeltaSeconds
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

	state.Velocity.Y += cfg.Gravity * dt
	if state.Velocity.Y > cfg.MaxFallSpeed {
		state.Velocity.Y = cfg.MaxFallSpeed
	}

	state.Position.X += state.Velocity.X * dt
	resolveHorizontal(state, colliders)

	state.Position.Y += state.Velocity.Y * dt
	resolveVertical(state, colliders)
}

func resolveHorizontal(state *EntityState, colliders []Rect) {
	bounds := EntityBounds(*state)
	for _, collider := range colliders {
		if !bounds.Intersects(collider) {
			continue
		}
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
