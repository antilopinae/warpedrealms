// Copyright (c) 2024 Warped Realms. All rights reserved.
// This source code is proprietary and confidential.
// Unauthorized copying or cloning of game mechanics is strictly prohibited.
// See LICENSE file in the project root for full license details.

package clientapp

import "warpedrealms/shared"

type timedState struct {
	serverTime float64
	state      shared.EntityState
}

type Interpolator struct {
	samples []timedState
}

func (i *Interpolator) Push(serverTime float64, state shared.EntityState) {
	i.samples = append(i.samples, timedState{
		serverTime: serverTime,
		state:      state.Clone(),
	})
	if len(i.samples) > 32 {
		i.samples = i.samples[len(i.samples)-32:]
	}
}

func (i *Interpolator) Sample(targetTime float64) (shared.EntityState, bool) {
	if len(i.samples) == 0 {
		return shared.EntityState{}, false
	}
	if len(i.samples) == 1 || targetTime <= i.samples[0].serverTime {
		return i.samples[0].state.Clone(), true
	}

	last := i.samples[len(i.samples)-1]
	if targetTime >= last.serverTime {
		return last.state.Clone(), true
	}

	for index := 1; index < len(i.samples); index++ {
		older := i.samples[index-1]
		newer := i.samples[index]
		if targetTime < older.serverTime || targetTime > newer.serverTime {
			continue
		}
		alpha := (targetTime - older.serverTime) / (newer.serverTime - older.serverTime)
		return lerpState(older.state, newer.state, alpha), true
	}
	return last.state.Clone(), true
}

func lerpState(a shared.EntityState, b shared.EntityState, alpha float64) shared.EntityState {
	result := b.Clone()
	result.Position = lerpVec(a.Position, b.Position, alpha)
	result.Velocity = lerpVec(a.Velocity, b.Velocity, alpha)
	result.Facing = a.Facing + (b.Facing-a.Facing)*alpha
	if alpha < 0.5 {
		result.Grounded = a.Grounded
	}
	return result
}

func lerpVec(a shared.Vec2, b shared.Vec2, alpha float64) shared.Vec2 {
	return shared.Vec2{
		X: a.X + (b.X-a.X)*alpha,
		Y: a.Y + (b.Y-a.Y)*alpha,
	}
}
