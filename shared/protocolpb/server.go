package protocolpb

import (
	"encoding/json"
	"fmt"

	"warpedrealms/shared"
	"warpedrealms/warpedrealms/shared/pb"
)

func ServerMessageToPB(message shared.ServerMessage) (*pb.ServerMessage, error) {
	switch message.Type {
	case "welcome":
		if message.Welcome == nil {
			return &pb.ServerMessage{}, nil
		}
		b, _ := json.Marshal(message.Welcome)
		var out pb.WelcomeMessage
		_ = json.Unmarshal(b, &out)
		return &pb.ServerMessage{Payload: &pb.ServerMessage_Welcome{Welcome: &out}}, nil
	case "snapshot":
		if message.Snapshot == nil {
			return &pb.ServerMessage{}, nil
		}
		b, _ := json.Marshal(message.Snapshot)
		var out pb.SnapshotMessage
		_ = json.Unmarshal(b, &out)
		return &pb.ServerMessage{Payload: &pb.ServerMessage_Snapshot{Snapshot: &out}}, nil
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
		var out shared.WelcomeMessage
		b, _ := json.Marshal(payload.Welcome)
		_ = json.Unmarshal(b, &out)
		return shared.ServerMessage{Type: "welcome", Welcome: &out}, nil
	case *pb.ServerMessage_Snapshot:
		var out shared.SnapshotMessage
		b, _ := json.Marshal(payload.Snapshot)
		_ = json.Unmarshal(b, &out)
		return shared.ServerMessage{Type: "snapshot", Snapshot: &out}, nil
	case *pb.ServerMessage_Pong:
		return shared.ServerMessage{Type: "pong", Pong: &shared.PongMessage{ClientTime: payload.Pong.ClientTime, ServerTime: payload.Pong.ServerTime}}, nil
	case *pb.ServerMessage_Error:
		return shared.ServerMessage{Type: "error", Error: payload.Error}, nil
	default:
		return shared.ServerMessage{}, nil
	}
}
