// Copyright (c) 2024 Warped Realms. All rights reserved.
// This source code is proprietary and confidential.
// Unauthorized copying or cloning of game mechanics is strictly prohibited.
// See LICENSE file in the project root for full license details.

package content

import "warpedrealms/shared"

func annotateRoomsWithSessionRings(rooms []shared.RoomState) {
	total := len(rooms)
	for i := range rooms {
		zone := shared.RingZoneForRoom(rooms[i].Index, total)
		rooms[i].RingZone = zone
		rooms[i].DeathPenalty = shared.DeathPenaltyForZone(zone)
		rooms[i].IsThrone = zone == shared.RingZoneThrone
	}
}
