package shared

import "math"

type AnimationState string

const (
	AnimationIdle     AnimationState = "idle"
	AnimationRun      AnimationState = "run"
	AnimationJump     AnimationState = "jump"
	AnimationFall     AnimationState = "fall"
	AnimationClimb    AnimationState = "climb"
	AnimationAttack1  AnimationState = "attack1"
	AnimationAttack2  AnimationState = "attack2"
	AnimationAttack3  AnimationState = "attack3"
	AnimationSkill1   AnimationState = "skill1"
	AnimationSkill2   AnimationState = "skill2"
	AnimationSkill3   AnimationState = "skill3"
	AnimationInteract AnimationState = "interact"
	AnimationTravel   AnimationState = "travel"
	AnimationEmerge   AnimationState = "emerge"
	AnimationHit      AnimationState = "hit"
	AnimationDeath    AnimationState = "death"
)

func TriggerAnimation(state *EntityState, animation AnimationState, startedAt float64) {
	state.Animation = animation
	state.AnimationStartedAt = startedAt
}

func RefreshAnimation(state *EntityState, now float64) {
	if state.HP <= 0 {
		if state.Animation != AnimationDeath {
			TriggerAnimation(state, AnimationDeath, now)
		}
		return
	}
	if state.Travel != nil && state.Travel.Active {
		if state.Animation != AnimationTravel {
			TriggerAnimation(state, AnimationTravel, now)
		}
		return
	}
	if AnimationIsOneShot(state.Animation) && now < state.AnimationStartedAt+AnimationDuration(*state, state.Animation) {
		return
	}
	desired := DesiredMovementAnimation(*state)
	if desired != state.Animation {
		TriggerAnimation(state, desired, now)
	}
}

func DesiredMovementAnimation(state EntityState) AnimationState {
	switch {
	case state.HP <= 0:
		return AnimationDeath
	case state.Travel != nil && state.Travel.Active:
		return AnimationTravel
	case !state.Grounded && state.Velocity.Y < -40:
		return AnimationJump
	case !state.Grounded && state.Velocity.Y > 40:
		return AnimationFall
	case math.Abs(state.Velocity.Y) > 40 && math.Abs(state.Velocity.X) < 20 && state.Grounded:
		return AnimationClimb
	case math.Abs(state.Velocity.X) > 18:
		return AnimationRun
	default:
		return AnimationIdle
	}
}

func AnimationIsOneShot(animation AnimationState) bool {
	switch animation {
	case AnimationAttack1, AnimationAttack2, AnimationAttack3,
		AnimationSkill1, AnimationSkill2, AnimationSkill3,
		AnimationInteract, AnimationTravel, AnimationEmerge,
		AnimationHit, AnimationDeath:
		return true
	default:
		return false
	}
}

func AnimationDuration(state EntityState, animation AnimationState) float64 {
	switch animation {
	case AnimationJump, AnimationFall:
		return 0.28
	case AnimationAttack1:
		return 0.30
	case AnimationAttack2:
		return 0.34
	case AnimationAttack3:
		return 0.40
	case AnimationSkill1:
		return 0.42
	case AnimationSkill2:
		return 0.46
	case AnimationSkill3:
		return 0.54
	case AnimationInteract:
		return 0.24
	case AnimationTravel:
		if state.Travel != nil && state.Travel.EndsAt > state.Travel.StartedAt {
			return state.Travel.EndsAt - state.Travel.StartedAt
		}
		return 0.7
	case AnimationEmerge:
		return 0.55
	case AnimationHit:
		return 0.22
	case AnimationDeath:
		switch state.Kind {
		case EntityKindBoss:
			return 1.2
		case EntityKindMimic:
			return 0.85
		default:
			return 0.72
		}
	default:
		return 0
	}
}

func NextAttackAnimation(state EntityState, combo int) AnimationState {
	switch state.ClassID {
	case PlayerClassKnight:
		switch combo % 3 {
		case 1:
			return AnimationAttack2
		case 2:
			return AnimationAttack3
		default:
			return AnimationAttack1
		}
	case PlayerClassArcherAssassin:
		if combo%2 == 1 {
			return AnimationAttack2
		}
		return AnimationAttack1
	case PlayerClassForestCaster:
		if combo%2 == 1 {
			return AnimationAttack3
		}
		return AnimationAttack1
	default:
		if state.Kind == EntityKindRat || state.Kind == EntityKindMob || state.Kind == EntityKindMimic || state.Kind == EntityKindBoss {
			return AnimationAttack1
		}
		switch combo % 3 {
		case 1:
			return AnimationAttack2
		case 2:
			return AnimationAttack3
		default:
			return AnimationAttack1
		}
	}
}
