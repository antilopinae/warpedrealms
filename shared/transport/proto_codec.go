package transport

import (
	"io"
	"warpedrealms/shared"
	"warpedrealms/shared/minipb"
)

func EncodeSnapshot(s *shared.SnapshotMessage, e minipb.Endian) []byte {
	w := minipb.NewWriter(e)
	w.Field(1, minipb.F64(s.ServerTime, e))
	w.Field(2, minipb.U64(s.Tick, e))
	w.Field(3, minipb.Str(s.LocalPlayerID))
	w.Field(4, minipb.U32(s.LastProcessedSeq, e))
	for _, en := range s.Entities {
		w.Field(10, encodeEntity(en, e))
	}
	for _, l := range s.Loot {
		w.Field(11, encodeLoot(l, e))
	}
	return w.Bytes()
}
func DecodeSnapshot(b []byte, e minipb.Endian) (shared.SnapshotMessage, error) {
	r := minipb.NewReader(b, e)
	out := shared.SnapshotMessage{}
	for {
		t, p, err := r.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return out, err
		}
		switch t {
		case 1:
			out.ServerTime = minipb.ReadF64(p, e)
		case 2:
			out.Tick = minipb.ReadU64(p, e)
		case 3:
			out.LocalPlayerID = string(p)
		case 4:
			out.LastProcessedSeq = minipb.ReadU32(p, e)
		case 10:
			out.Entities = append(out.Entities, decodeEntity(p, e))
		case 11:
			out.Loot = append(out.Loot, decodeLoot(p, e))
		}
	}
	return out, nil
}

func EncodeWelcome(wm *shared.WelcomeMessage, e minipb.Endian) []byte {
	w := minipb.NewWriter(e)
	w.Field(1, minipb.Str(wm.PlayerID))
	w.Field(2, minipb.Str(wm.PlayerName))
	w.Field(3, minipb.Str(wm.ClassID))
	w.Field(4, minipb.Str(wm.RaidID))
	w.Field(5, minipb.Str(wm.RaidName))
	w.Field(6, minipb.Str(wm.ContentVersion))
	w.Field(7, minipb.F64(wm.ServerTime, e))
	w.Field(8, minipb.F64(wm.TickRate, e))
	w.Field(9, minipb.F64(wm.SnapshotRate, e))
	w.Field(10, minipb.F64(wm.InterpolationBackTime, e))
	return w.Bytes()
}
func DecodeWelcome(b []byte, e minipb.Endian) (shared.WelcomeMessage, error) {
	r := minipb.NewReader(b, e)
	out := shared.WelcomeMessage{}
	for {
		t, p, err := r.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return out, err
		}
		switch t {
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
			out.ServerTime = minipb.ReadF64(p, e)
		case 8:
			out.TickRate = minipb.ReadF64(p, e)
		case 9:
			out.SnapshotRate = minipb.ReadF64(p, e)
		case 10:
			out.InterpolationBackTime = minipb.ReadF64(p, e)
		}
	}
	return out, nil
}

func EncodePong(p *shared.PongMessage, e minipb.Endian) []byte {
	w := minipb.NewWriter(e)
	w.Field(1, minipb.F64(p.ClientTime, e))
	w.Field(2, minipb.F64(p.ServerTime, e))
	return w.Bytes()
}
func DecodePong(b []byte, e minipb.Endian) (shared.PongMessage, error) {
	r := minipb.NewReader(b, e)
	out := shared.PongMessage{}
	for {
		t, p, err := r.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return out, err
		}
		if t == 1 {
			out.ClientTime = minipb.ReadF64(p, e)
		}
		if t == 2 {
			out.ServerTime = minipb.ReadF64(p, e)
		}
	}
	return out, nil
}

func encodeEntity(en shared.EntityState, e minipb.Endian) []byte {
	w := minipb.NewWriter(e)
	w.Field(1, minipb.Str(en.ID))
	w.Field(2, minipb.Str(en.Name))
	w.Field(3, minipb.Str(string(en.Kind)))
	w.Field(4, minipb.F64(en.Position.X, e))
	w.Field(5, minipb.F64(en.Position.Y, e))
	w.Field(6, minipb.U32(uint32(en.HP), e))
	w.Field(7, minipb.U32(uint32(en.MaxHP), e))
	w.Field(8, minipb.Str(en.RoomID))
	return w.Bytes()
}
func decodeEntity(b []byte, e minipb.Endian) shared.EntityState {
	r := minipb.NewReader(b, e)
	out := shared.EntityState{}
	for {
		t, p, err := r.Next()
		if err != nil {
			break
		}
		switch t {
		case 1:
			out.ID = string(p)
		case 2:
			out.Name = string(p)
		case 3:
			out.Kind = shared.EntityKind(string(p))
		case 4:
			out.Position.X = minipb.ReadF64(p, e)
		case 5:
			out.Position.Y = minipb.ReadF64(p, e)
		case 6:
			out.HP = int(minipb.ReadU32(p, e))
		case 7:
			out.MaxHP = int(minipb.ReadU32(p, e))
		case 8:
			out.RoomID = string(p)
		}
	}
	return out
}
func encodeLoot(l shared.LootState, e minipb.Endian) []byte {
	w := minipb.NewWriter(e)
	w.Field(1, minipb.Str(l.ID))
	w.Field(2, minipb.Str(l.RoomID))
	w.Field(3, minipb.F64(l.Position.X, e))
	w.Field(4, minipb.F64(l.Position.Y, e))
	w.Field(5, minipb.U32(uint32(l.Value), e))
	return w.Bytes()
}
func decodeLoot(b []byte, e minipb.Endian) shared.LootState {
	r := minipb.NewReader(b, e)
	out := shared.LootState{}
	for {
		t, p, err := r.Next()
		if err != nil {
			break
		}
		switch t {
		case 1:
			out.ID = string(p)
		case 2:
			out.RoomID = string(p)
		case 3:
			out.Position.X = minipb.ReadF64(p, e)
		case 4:
			out.Position.Y = minipb.ReadF64(p, e)
		case 5:
			out.Value = int(minipb.ReadU32(p, e))
		}
	}
	return out
}
