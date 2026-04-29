package protocolpb

import (
	"fmt"

	"warpedrealms/shared"
	"warpedrealms/warpedrealms/shared/pb"
)

func InputCommandToPB(command shared.InputCommand) *pb.InputCommand {
	return &pb.InputCommand{Seq: command.Seq, MoveX: command.MoveX, MoveY: command.MoveY, Jump: command.Jump, DropDown: command.DropDown, Dash: command.Dash, PrimaryAttack: command.PrimaryAttack, Skill1: command.Skill1, Skill2: command.Skill2, Skill3: command.Skill3, Interact: command.Interact, UseJumpLink: command.UseJumpLink, AimX: command.AimX, AimY: command.AimY, ClientTime: command.ClientTime}
}

func InputCommandFromPB(command *pb.InputCommand) shared.InputCommand {
	if command == nil {
		return shared.InputCommand{}
	}
	return shared.InputCommand{Seq: command.Seq, MoveX: command.MoveX, MoveY: command.MoveY, Jump: command.Jump, DropDown: command.DropDown, Dash: command.Dash, PrimaryAttack: command.PrimaryAttack, Skill1: command.Skill1, Skill2: command.Skill2, Skill3: command.Skill3, Interact: command.Interact, UseJumpLink: command.UseJumpLink, AimX: command.AimX, AimY: command.AimY, ClientTime: command.ClientTime}
}

func InputBatchToPB(batch shared.InputBatch) *pb.InputBatch {
	out := &pb.InputBatch{Commands: make([]*pb.InputCommand, 0, len(batch.Commands))}
	for _, c := range batch.Commands {
		out.Commands = append(out.Commands, InputCommandToPB(c))
	}
	return out
}
func InputBatchFromPB(batch *pb.InputBatch) shared.InputBatch {
	if batch == nil {
		return shared.InputBatch{}
	}
	out := shared.InputBatch{Commands: make([]shared.InputCommand, 0, len(batch.Commands))}
	for _, c := range batch.Commands {
		out.Commands = append(out.Commands, InputCommandFromPB(c))
	}
	return out
}

func ClientMessageToPB(message shared.ClientMessage) (*pb.ClientMessage, error) {
	switch payload := message.Payload.(type) {
	case shared.ClientInputPayload:
		return &pb.ClientMessage{Payload: &pb.ClientMessage_Input{Input: InputBatchToPB(payload.InputBatch)}}, nil
	case *shared.ClientInputPayload:
		if payload == nil {
			return &pb.ClientMessage{}, nil
		}
		return &pb.ClientMessage{Payload: &pb.ClientMessage_Input{Input: InputBatchToPB(payload.InputBatch)}}, nil
	case shared.ClientPingPayload:
		return &pb.ClientMessage{Payload: &pb.ClientMessage_Ping{Ping: &pb.PingMessage{ClientTime: payload.ClientTime}}}, nil
	case *shared.ClientPingPayload:
		if payload == nil {
			return &pb.ClientMessage{}, nil
		}
		return &pb.ClientMessage{Payload: &pb.ClientMessage_Ping{Ping: &pb.PingMessage{ClientTime: payload.ClientTime}}}, nil
	default:
		return nil, fmt.Errorf("unsupported client payload %T", message.Payload)
	}
}
