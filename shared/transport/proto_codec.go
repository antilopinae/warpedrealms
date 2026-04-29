package transport

import (
	"fmt"
	"warpedrealms/shared"
	"warpedrealms/shared/pbwire"
)

func EncodeSnapshot(s *shared.SnapshotMessage) []byte {
	w := &pbwire.Writer{}
	w.Double(1, s.ServerTime)
	w.Uint64(2, s.Tick)
	w.String(3, s.LocalPlayerID)
	w.Uint32(4, s.LastProcessedSeq)
	for _, e := range s.Entities {
		w.Message(10, encodeEntity(e))
	}
	for _, l := range s.Loot {
		w.Message(11, encodeLoot(l))
	}
	return w.Bytes()
}
func DecodeSnapshot(b []byte) (shared.SnapshotMessage, error) {
	r := pbwire.NewReader(b)
	out := shared.SnapshotMessage{}
	for {
		f, _, p, err := r.Next()
		if err != nil {
			break
		}
		switch f {
		case 1:
			out.ServerTime = pbwire.AsDouble(p)
		case 2:
			out.Tick = pbwire.AsUint(p)
		case 3:
			out.LocalPlayerID = string(p)
		case 4:
			out.LastProcessedSeq = uint32(pbwire.AsUint(p))
		case 10:
			out.Entities = append(out.Entities, decodeEntity(p))
		case 11:
			out.Loot = append(out.Loot, decodeLoot(p))
		}
	}
	return out, nil
}
func EncodeWelcome(m *shared.WelcomeMessage) []byte {
	w := &pbwire.Writer{}
	w.String(1, m.PlayerID)
	w.String(2, m.PlayerName)
	w.String(3, m.ClassID)
	w.String(4, m.RaidID)
	w.String(5, m.RaidName)
	w.String(6, m.ContentVersion)
	w.Double(7, m.ServerTime)
	w.Double(8, m.TickRate)
	w.Double(9, m.SnapshotRate)
	w.Double(10, m.InterpolationBackTime)
	return w.Bytes()
}
func DecodeWelcome(b []byte) (shared.WelcomeMessage, error) {
	r := pbwire.NewReader(b)
	out := shared.WelcomeMessage{}
	for {
		f, _, p, err := r.Next()
		if err != nil {
			break
		}
		switch f {
		case 1:
			out.PlayerID = string(p)
		case 2:
			out.PlayerName = string(p)
		case 3:
			out.ClassID = string(p)
		case 4:
			out.RaidID = string(p)
		case 5:
			out.RaidName = string(p)
		case 6:
			out.ContentVersion = string(p)
		case 7:
			out.ServerTime = pbwire.AsDouble(p)
		case 8:
			out.TickRate = pbwire.AsDouble(p)
		case 9:
			out.SnapshotRate = pbwire.AsDouble(p)
		case 10:
			out.InterpolationBackTime = pbwire.AsDouble(p)
		}
	}
	return out, nil
}
func EncodePong(m *shared.PongMessage) []byte {
	w := &pbwire.Writer{}
	w.Double(1, m.ClientTime)
	w.Double(2, m.ServerTime)
	return w.Bytes()
}
func DecodePong(b []byte) (shared.PongMessage, error) {
	r := pbwire.NewReader(b)
	out := shared.PongMessage{}
	for {
		f, _, p, err := r.Next()
		if err != nil {
			break
		}
		if f == 1 {
			out.ClientTime = pbwire.AsDouble(p)
		}
		if f == 2 {
			out.ServerTime = pbwire.AsDouble(p)
		}
	}
	return out, nil
}

func encodeEntity(e shared.EntityState) []byte {
	w := &pbwire.Writer{}
	w.String(1, e.ID)
	w.String(2, e.Name)
	w.String(3, string(e.Kind))
	w.String(4, e.RoomID)
	w.Double(5, e.Position.X)
	w.Double(6, e.Position.Y)
	w.Uint32(7, uint32(e.HP))
	w.Uint32(8, uint32(e.MaxHP))
	return w.Bytes()
}
func decodeEntity(b []byte) shared.EntityState {
	r := pbwire.NewReader(b)
	out := shared.EntityState{}
	for {
		f, _, p, err := r.Next()
		if err != nil {
			break
		}
		switch f {
		case 1:
			out.ID = string(p)
		case 2:
			out.Name = string(p)
		case 3:
			out.Kind = shared.EntityKind(string(p))
		case 4:
			out.RoomID = string(p)
		case 5:
			out.Position.X = pbwire.AsDouble(p)
		case 6:
			out.Position.Y = pbwire.AsDouble(p)
		case 7:
			out.HP = int(pbwire.AsUint(p))
		case 8:
			out.MaxHP = int(pbwire.AsUint(p))
		}
	}
	return out
}
func encodeLoot(l shared.LootState) []byte {
	w := &pbwire.Writer{}
	w.String(1, l.ID)
	w.String(2, l.RoomID)
	w.Double(3, l.Position.X)
	w.Double(4, l.Position.Y)
	w.Uint32(5, uint32(l.Value))
	return w.Bytes()
}
func decodeLoot(b []byte) shared.LootState {
	r := pbwire.NewReader(b)
	out := shared.LootState{}
	for {
		f, _, p, err := r.Next()
		if err != nil {
			break
		}
		switch f {
		case 1:
			out.ID = string(p)
		case 2:
			out.RoomID = string(p)
		case 3:
			out.Position.X = pbwire.AsDouble(p)
		case 4:
			out.Position.Y = pbwire.AsDouble(p)
		case 5:
			out.Value = int(pbwire.AsUint(p))
		default:
			fmt.Sprint()
		}
	}
	return out
}
