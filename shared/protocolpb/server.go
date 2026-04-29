package protocolpb

import (
	"fmt"

	"warpedrealms/shared"
	"warpedrealms/warpedrealms/shared/pb"
)

func vec2ToPB(v shared.Vec2) *pb.Vec2 { return &pb.Vec2{X: v.X, Y: v.Y} }
func vec2FromPB(v *pb.Vec2) shared.Vec2 {
	if v == nil {
		return shared.Vec2{}
	}
	return shared.Vec2{X: v.X, Y: v.Y}
}
func rectToPB(r shared.Rect) *pb.Rect { return &pb.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H} }
func rectFromPB(r *pb.Rect) shared.Rect {
	if r == nil {
		return shared.Rect{}
	}
	return shared.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}
}

func roomToPB(r shared.RoomState) *pb.RoomState {
	out := &pb.RoomState{Id: r.ID, Name: r.Name, Biome: r.Biome, Bounds: rectToPB(r.Bounds), Index: int32(r.Index), TemplateId: r.TemplateID, RingZone: string(r.RingZone), DeathPenalty: string(r.DeathPenalty), IsThrone: r.IsThrone, BackgroundId: r.BackgroundID, TileStyleId: r.TileStyleID, BelowRoomId: r.BelowRoomID, AboveRoomId: r.AboveRoomID}
	for _, v := range r.Solids {
		out.Solids = append(out.Solids, rectToPB(v))
	}
	for _, v := range r.Platforms {
		out.Platforms = append(out.Platforms, rectToPB(v))
	}
	for _, v := range r.Backwalls {
		out.Backwalls = append(out.Backwalls, rectToPB(v))
	}
	for _, v := range r.RevealZones {
		out.RevealZones = append(out.RevealZones, rectToPB(v.Area))
	}
	for _, v := range r.RiftZones {
		out.RiftZones = append(out.RiftZones, rectToPB(v))
	}
	for _, v := range r.PortalZones {
		out.PortalZones = append(out.PortalZones, rectToPB(v))
	}
	for _, v := range r.PvPZones {
		out.PvpZones = append(out.PvpZones, rectToPB(v))
	}
	for _, e := range r.Exits {
		out.Exits = append(out.Exits, &pb.ExitState{Id: e.ID, RoomId: e.RoomID, Label: e.Label, Area: rectToPB(e.Area), AssignedPlayerId: e.AssignedPlayerID})
	}
	for _, j := range r.JumpLinks {
		out.JumpLinks = append(out.JumpLinks, &pb.JumpLinkState{Id: j.ID, RoomId: j.RoomID, TargetRoomId: j.TargetRoomID, Label: j.Label, Area: rectToPB(j.Area), Arrival: vec2ToPB(j.Arrival)})
	}
	for _, a := range r.Decorations {
		out.Decorations = append(out.Decorations, &pb.PlacedAssetState{Id: a.ID, ProfileId: a.ProfileID, RoomId: a.RoomID, Position: vec2ToPB(a.Position), Scale: a.Scale, DrawOffset: vec2ToPB(a.DrawOffset), Layer: a.Layer, Alpha: a.Alpha, Bounds: rectToPB(a.Bounds)})
	}
	for _, b := range r.BossSpawns {
		out.BossSpawns = append(out.BossSpawns, &pb.BossSpawn{X: b.X, Y: b.Y, Level: int32(b.Level), Flying: b.Flying})
	}
	for _, rf := range r.Rifts {
		out.Rifts = append(out.Rifts, &pb.RiftState{Id: rf.ID, RoomId: rf.RoomID, TargetRoomId: rf.TargetRoomID, Area: rectToPB(rf.Area), Arrival: vec2ToPB(rf.Arrival), Kind: string(rf.Kind), Capacity: int32(rf.Capacity), UsedCount: int32(rf.UsedCount)})
	}
	return out
}
func roomFromPB(r *pb.RoomState) shared.RoomState {
	if r == nil {
		return shared.RoomState{}
	}
	out := shared.RoomState{ID: r.Id, Name: r.Name, Biome: r.Biome, Bounds: rectFromPB(r.Bounds), Index: int(r.Index), TemplateID: r.TemplateId, RingZone: shared.RingZone(r.RingZone), DeathPenalty: shared.DeathPenalty(r.DeathPenalty), IsThrone: r.IsThrone, BackgroundID: r.BackgroundId, TileStyleID: r.TileStyleId, BelowRoomID: r.BelowRoomId, AboveRoomID: r.AboveRoomId}
	for _, v := range r.Solids {
		out.Solids = append(out.Solids, rectFromPB(v))
	}
	for _, v := range r.Platforms {
		out.Platforms = append(out.Platforms, rectFromPB(v))
	}
	for _, v := range r.Backwalls {
		out.Backwalls = append(out.Backwalls, rectFromPB(v))
	}
	for _, v := range r.RevealZones {
		out.RevealZones = append(out.RevealZones, shared.RevealZoneState{Area: rectFromPB(v)})
	}
	for _, v := range r.RiftZones {
		out.RiftZones = append(out.RiftZones, rectFromPB(v))
	}
	for _, v := range r.PortalZones {
		out.PortalZones = append(out.PortalZones, rectFromPB(v))
	}
	for _, v := range r.PvpZones {
		out.PvPZones = append(out.PvPZones, rectFromPB(v))
	}
	for _, e := range r.Exits {
		out.Exits = append(out.Exits, shared.ExitState{ID: e.Id, RoomID: e.RoomId, Label: e.Label, Area: rectFromPB(e.Area), AssignedPlayerID: e.AssignedPlayerId})
	}
	for _, j := range r.JumpLinks {
		out.JumpLinks = append(out.JumpLinks, shared.JumpLinkState{ID: j.Id, RoomID: j.RoomId, TargetRoomID: j.TargetRoomId, Label: j.Label, Area: rectFromPB(j.Area), Arrival: vec2FromPB(j.Arrival)})
	}
	for _, a := range r.Decorations {
		out.Decorations = append(out.Decorations, shared.PlacedAssetState{ID: a.Id, ProfileID: a.ProfileId, RoomID: a.RoomId, Position: vec2FromPB(a.Position), Scale: a.Scale, DrawOffset: vec2FromPB(a.DrawOffset), Layer: a.Layer, Alpha: a.Alpha, Bounds: rectFromPB(a.Bounds)})
	}
	for _, b := range r.BossSpawns {
		out.BossSpawns = append(out.BossSpawns, shared.BossSpawn{X: b.X, Y: b.Y, Level: int(b.Level), Flying: b.Flying})
	}
	for _, rf := range r.Rifts {
		out.Rifts = append(out.Rifts, shared.RiftState{ID: rf.Id, RoomID: rf.RoomId, TargetRoomID: rf.TargetRoomId, Area: rectFromPB(rf.Area), Arrival: vec2FromPB(rf.Arrival), Kind: shared.RiftKind(rf.Kind), Capacity: int(rf.Capacity), UsedCount: int(rf.UsedCount)})
	}
	return out
}

func snapshotToPB(s *shared.SnapshotMessage) *pb.SnapshotMessage {
	if s == nil {
		return nil
	}
	out := &pb.SnapshotMessage{ServerTime: s.ServerTime, Tick: s.Tick, LocalPlayerId: s.LocalPlayerID, LastProcessedSeq: s.LastProcessedSeq}
	if s.Layout != nil {
		out.Layout = &pb.RaidLayoutState{Seed: s.Layout.Seed}
		for _, r := range s.Layout.Rooms {
			out.Layout.Rooms = append(out.Layout.Rooms, roomToPB(r))
		}
		for _, sp := range s.Layout.PlayerSpawns {
			out.Layout.PlayerSpawns = append(out.Layout.PlayerSpawns, vec2ToPB(sp))
		}
	}
	for _, e := range s.Entities {
		var travel *pb.TravelState
		if e.Travel != nil {
			travel = &pb.TravelState{
				Active:     e.Travel.Active,
				LinkId:     e.Travel.LinkID,
				FromRoomId: e.Travel.FromRoomID,
				ToRoomId:   e.Travel.ToRoomID,
				Start:      vec2ToPB(e.Travel.Start),
				End:        vec2ToPB(e.Travel.End),
				StartedAt:  e.Travel.StartedAt,
				EndsAt:     e.Travel.EndsAt,
			}
		}
		out.Entities = append(out.Entities, &pb.EntityState{Id: e.ID, Name: e.Name, Kind: string(e.Kind), Faction: string(e.Faction), ClassId: e.ClassID, ProfileId: e.ProfileID, FamilyId: e.FamilyID, RoomId: e.RoomID, Position: vec2ToPB(e.Position), Velocity: vec2ToPB(e.Velocity), Facing: e.Facing, Grounded: e.Grounded, Hp: int32(e.HP), MaxHp: int32(e.MaxHP), Animation: string(e.Animation), AnimationStartedAt: e.AnimationStartedAt, SpriteSize: vec2ToPB(e.SpriteSize), SpriteOffset: vec2ToPB(e.SpriteOffset), Collider: rectToPB(e.Collider), Hurtbox: rectToPB(e.Hurtbox), InteractionBox: rectToPB(e.InteractionBox), Scale: e.Scale, Travel: travel, DashTimer: e.DashTimer, DashCooldown: e.DashCooldown})
	}
	for _, l := range s.Loot {
		out.Loot = append(out.Loot, &pb.LootState{Id: l.ID, ProfileId: l.ProfileID, RoomId: l.RoomID, Position: vec2ToPB(l.Position), Value: int32(l.Value)})
	}
	if s.Raid != nil {
		out.Raid = &pb.RaidState{RaidId: s.Raid.RaidID, Name: s.Raid.Name, Phase: string(s.Raid.Phase), Duration: s.Raid.Duration, TimeRemaining: s.Raid.TimeRemaining, LocalStatus: string(s.Raid.LocalStatus)}
		for _, p := range s.Raid.Players {
			pp := &pb.PlayerRaidState{PlayerId: p.PlayerID, Name: p.Name, Status: string(p.Status), CarriedLoot: int32(p.CarriedLoot), AssignedExitId: p.AssignedExitID, AssignedExitTag: p.AssignedExitTag, CurrentRoomId: p.CurrentRoomID, Hp: int32(p.HP), MaxHp: int32(p.MaxHP)}
			for _, c := range p.Cooldowns {
				pp.Cooldowns = append(pp.Cooldowns, &pb.AbilityCooldown{AbilityId: c.ID, Remaining: c.Remaining})
			}
			out.Raid.Players = append(out.Raid.Players, pp)
		}
	}
	return out
}

func snapshotFromPB(s *pb.SnapshotMessage) *shared.SnapshotMessage {
	if s == nil {
		return nil
	}
	out := &shared.SnapshotMessage{ServerTime: s.ServerTime, Tick: s.Tick, LocalPlayerID: s.LocalPlayerId, LastProcessedSeq: s.LastProcessedSeq}
	if s.Layout != nil {
		out.Layout = &shared.RaidLayoutState{Seed: s.Layout.Seed}
		for _, r := range s.Layout.Rooms {
			out.Layout.Rooms = append(out.Layout.Rooms, roomFromPB(r))
		}
		for _, sp := range s.Layout.PlayerSpawns {
			out.Layout.PlayerSpawns = append(out.Layout.PlayerSpawns, vec2FromPB(sp))
		}
	}
	for _, e := range s.Entities {
		tr := shared.TravelState{}
		if e.Travel != nil {
			tr = shared.TravelState{Active: e.Travel.Active, LinkID: e.Travel.LinkId, FromRoomID: e.Travel.FromRoomId, ToRoomID: e.Travel.ToRoomId, Start: vec2FromPB(e.Travel.Start), End: vec2FromPB(e.Travel.End), StartedAt: e.Travel.StartedAt, EndsAt: e.Travel.EndsAt}
		}
		out.Entities = append(out.Entities, shared.EntityState{ID: e.Id, Name: e.Name, Kind: shared.EntityKind(e.Kind), Faction: shared.Faction(e.Faction), ClassID: e.ClassId, ProfileID: e.ProfileId, FamilyID: e.FamilyId, RoomID: e.RoomId, Position: vec2FromPB(e.Position), Velocity: vec2FromPB(e.Velocity), Facing: e.Facing, Grounded: e.Grounded, HP: int(e.Hp), MaxHP: int(e.MaxHp), Animation: shared.AnimationState(e.Animation), AnimationStartedAt: e.AnimationStartedAt, SpriteSize: vec2FromPB(e.SpriteSize), SpriteOffset: vec2FromPB(e.SpriteOffset), Collider: rectFromPB(e.Collider), Hurtbox: rectFromPB(e.Hurtbox), InteractionBox: rectFromPB(e.InteractionBox), Scale: e.Scale, Travel: &tr, DashTimer: e.DashTimer, DashCooldown: e.DashCooldown})
	}
	for _, l := range s.Loot {
		out.Loot = append(out.Loot, shared.LootState{ID: l.Id, ProfileID: l.ProfileId, RoomID: l.RoomId, Position: vec2FromPB(l.Position), Value: int(l.Value)})
	}
	if s.Raid != nil {
		out.Raid = &shared.RaidState{RaidID: s.Raid.RaidId, Name: s.Raid.Name, Phase: shared.RaidPhase(s.Raid.Phase), Duration: s.Raid.Duration, TimeRemaining: s.Raid.TimeRemaining, LocalStatus: shared.PlayerRaidStatus(s.Raid.LocalStatus)}
		for _, p := range s.Raid.Players {
			sp := shared.PlayerRaidState{PlayerID: p.PlayerId, Name: p.Name, Status: shared.PlayerRaidStatus(p.Status), CarriedLoot: int(p.CarriedLoot), AssignedExitID: p.AssignedExitId, AssignedExitTag: p.AssignedExitTag, CurrentRoomID: p.CurrentRoomId, HP: int(p.Hp), MaxHP: int(p.MaxHp)}
			for _, c := range p.Cooldowns {
				sp.Cooldowns = append(sp.Cooldowns, shared.AbilityCooldown{ID: c.AbilityId, Remaining: c.Remaining})
			}
			out.Raid.Players = append(out.Raid.Players, sp)
		}
	}
	return out
}

func ServerMessageToPB(message shared.ServerMessage) (*pb.ServerMessage, error) {
	switch message.Type {
	case "welcome":
		if message.Welcome == nil {
			return &pb.ServerMessage{}, nil
		}
		return &pb.ServerMessage{Payload: &pb.ServerMessage_Welcome{Welcome: &pb.WelcomeMessage{PlayerId: message.Welcome.PlayerID, PlayerName: message.Welcome.PlayerName, ClassId: message.Welcome.ClassID, RaidId: message.Welcome.RaidID, RaidName: message.Welcome.RaidName, ContentVersion: message.Welcome.ContentVersion, ServerTime: message.Welcome.ServerTime, TickRate: message.Welcome.TickRate, SnapshotRate: message.Welcome.SnapshotRate, InterpolationBackTime: message.Welcome.InterpolationBackTime}}}, nil
	case "snapshot":
		return &pb.ServerMessage{Payload: &pb.ServerMessage_Snapshot{Snapshot: snapshotToPB(message.Snapshot)}}, nil
	case "pong":
		if message.Pong == nil {
			return &pb.ServerMessage{}, nil
		}
		return &pb.ServerMessage{Payload: &pb.ServerMessage_Pong{Pong: &pb.PongMessage{ClientTime: message.Pong.ClientTime, ServerTime: message.Pong.ServerTime}}}, nil
	case "error":
		return &pb.ServerMessage{Payload: &pb.ServerMessage_Error{Error: message.Error}}, nil
	default:
		return nil, fmt.Errorf("unsupported server type %q", message.Type)
	}
}

func ServerMessageFromPB(message *pb.ServerMessage) (shared.ServerMessage, error) {
	if message == nil {
		return shared.ServerMessage{}, nil
	}
	switch payload := message.Payload.(type) {
	case *pb.ServerMessage_Welcome:
		w := payload.Welcome
		if w == nil {
			return shared.ServerMessage{Type: "welcome"}, nil
		}
		return shared.ServerMessage{Type: "welcome", Welcome: &shared.WelcomeMessage{PlayerID: w.PlayerId, PlayerName: w.PlayerName, ClassID: w.ClassId, RaidID: w.RaidId, RaidName: w.RaidName, ContentVersion: w.ContentVersion, ServerTime: w.ServerTime, TickRate: w.TickRate, SnapshotRate: w.SnapshotRate, InterpolationBackTime: w.InterpolationBackTime}}, nil
	case *pb.ServerMessage_Snapshot:
		return shared.ServerMessage{Type: "snapshot", Snapshot: snapshotFromPB(payload.Snapshot)}, nil
	case *pb.ServerMessage_Pong:
		return shared.ServerMessage{Type: "pong", Pong: &shared.PongMessage{ClientTime: payload.Pong.ClientTime, ServerTime: payload.Pong.ServerTime}}, nil
	case *pb.ServerMessage_Error:
		return shared.ServerMessage{Type: "error", Error: payload.Error}, nil
	default:
		return shared.ServerMessage{}, nil
	}
}
