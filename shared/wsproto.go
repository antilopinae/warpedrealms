package shared

import (
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	pb "warpedrealms/warpedrealms/shared/pb"
)

type WireProtocol string

const (
	WireProtocolJSON     WireProtocol = "json"
	WireProtocolProtobuf WireProtocol = "protobuf"
)

func ParseWireProtocol(value string) WireProtocol {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "protobuf", "proto", "pb":
		return WireProtocolProtobuf
	default:
		return WireProtocolJSON
	}
}

func EncodeSnapshotBinary(message SnapshotMessage) ([]byte, error) {
	payload := &pb.ServerMessage{Type: "snapshot", Snapshot: ToPBSnapshot(message)}
	return proto.Marshal(payload)
}

func DecodeSnapshotBinary(raw []byte) (*SnapshotMessage, error) {
	var payload pb.ServerMessage
	if err := proto.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal protobuf snapshot: %w", err)
	}
	if payload.Snapshot == nil {
		return nil, fmt.Errorf("protobuf snapshot missing payload")
	}
	result := FromPBSnapshot(payload.Snapshot)
	return &result, nil
}

func ToPBSnapshot(snapshot SnapshotMessage) *pb.SnapshotMessage {
	raw, _ := json.Marshal(snapshot)
	var result pb.SnapshotMessage
	_ = json.Unmarshal(raw, &result)
	return &result
}

func FromPBSnapshot(snapshot *pb.SnapshotMessage) SnapshotMessage {
	if snapshot == nil {
		return SnapshotMessage{}
	}
	raw, _ := json.Marshal(snapshot)
	var result SnapshotMessage
	_ = json.Unmarshal(raw, &result)
	return result
}
