// Copyright (c) 2024 Warped Realms. All rights reserved.
// This source code is proprietary and confidential.
// Unauthorized copying or cloning of game mechanics is strictly prohibited.
// See LICENSE file in the project root for full license details.

package shared

import "testing"

func TestRefreshAnimationKeepsOneShotUntilDurationEnds(t *testing.T) {
	state := EntityState{
		Kind:               EntityKindPlayer,
		Grounded:           true,
		Velocity:           Vec2{X: 120},
		HP:                 100,
		Animation:          AnimationAttack1,
		AnimationStartedAt: 5,
	}

	RefreshAnimation(&state, 5.2)
	if state.Animation != AnimationAttack1 {
		t.Fatalf("expected attack animation to stay active, got %s", state.Animation)
	}

	RefreshAnimation(&state, 6.0)
	if state.Animation != AnimationRun {
		t.Fatalf("expected movement animation after attack, got %s", state.Animation)
	}
}

func TestRefreshAnimationPromotesDeath(t *testing.T) {
	state := EntityState{
		Kind:      EntityKindRat,
		Grounded:  true,
		Velocity:  Vec2{},
		HP:        0,
		Animation: AnimationHit,
	}

	RefreshAnimation(&state, 3)
	if state.Animation != AnimationDeath {
		t.Fatalf("expected death animation, got %s", state.Animation)
	}
}
