// Copyright (c) 2024 Warped Realms. All rights reserved.
// This source code is proprietary and confidential.
// Unauthorized copying or cloning of game mechanics is strictly prohibited.
// See LICENSE file in the project root for full license details.

package serverapp

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"warpedrealms/content"
	"warpedrealms/shared"
)

type SessionManager struct {
	mu       sync.RWMutex
	bundle   *content.Bundle
	raids    map[string]*RaidRoom
	nextRaid int
}

func NewSessionManager(bundle *content.Bundle) *SessionManager {
	manager := &SessionManager{
		bundle:   bundle,
		raids:    make(map[string]*RaidRoom),
		nextRaid: 1,
	}
	manager.CreateRaid()
	return manager
}

func (m *SessionManager) CreateRaid() shared.RaidSummary {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := fmt.Sprintf("raid-%03d", m.nextRaid)
	name := fmt.Sprintf("Raid %02d", m.nextRaid)
	m.nextRaid++

	seed := time.Now().UnixNano() + int64(m.nextRaid*97)
	room, err := NewRaidRoomProcGen(id, name, m.bundle, seed, shared.DefaultRaidMaxPlayers, shared.DefaultRaidDuration)
	if err != nil {
		return shared.RaidSummary{
			ID:            id,
			Name:          name,
			Phase:         shared.RaidPhaseFinished,
			MaxPlayers:    shared.DefaultRaidMaxPlayers,
			TimeRemaining: 0,
			Duration:      shared.DefaultRaidDuration,
		}
	}
	room.Start()
	m.raids[id] = room
	return room.Summary()
}

func (m *SessionManager) ListRaids() []shared.RaidSummary {
	m.mu.RLock()
	rooms := make([]*RaidRoom, 0, len(m.raids))
	for _, room := range m.raids {
		rooms = append(rooms, room)
	}
	m.mu.RUnlock()

	summaries := make([]shared.RaidSummary, 0, len(rooms))
	finishedEmpty := make([]string, 0)
	for _, room := range rooms {
		summary := room.Summary()
		if summary.Phase == shared.RaidPhaseFinished && summary.CurrentPlayers == 0 {
			finishedEmpty = append(finishedEmpty, summary.ID)
			continue
		}
		summaries = append(summaries, summary)
	}
	if len(finishedEmpty) > 0 {
		m.mu.Lock()
		for _, id := range finishedEmpty {
			delete(m.raids, id)
		}
		m.mu.Unlock()
	}
	if len(summaries) == 0 {
		summaries = append(summaries, m.CreateRaid())
	}
	sort.Slice(summaries, func(i int, j int) bool {
		return summaries[i].ID < summaries[j].ID
	})
	return summaries
}

func (m *SessionManager) GetRaid(id string) (*RaidRoom, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	room, ok := m.raids[id]
	return room, ok
}
