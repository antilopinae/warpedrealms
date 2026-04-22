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
