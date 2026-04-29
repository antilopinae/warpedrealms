// Copyright (c) 2024 Warped Realms. All rights reserved.
// This source code is proprietary and confidential.
// Unauthorized copying or cloning of game mechanics is strictly prohibited.
// See LICENSE file in the project root for full license details.

package shared

import "encoding/json"

type AuthRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string `json:"token,omitempty"`
	Error string `json:"error,omitempty"`
}

type RaidListResponse struct {
	Raids []RaidSummary `json:"raids"`
}

type RaidCreateResponse struct {
	Raid RaidSummary `json:"raid"`
}

type RaidCreateAcceptedResponse struct {
	JobID string `json:"job_id"`
}

type RaidCreateJobResponse struct {
	JobID              string       `json:"job_id"`
	Status             string       `json:"status"`
	Raid               *RaidSummary `json:"raid,omitempty"`
	Error              string       `json:"error,omitempty"`
	QueueWaitMs        int64        `json:"queue_wait_ms,omitempty"`
	GenerationTimeMs   int64        `json:"generation_time_ms,omitempty"`
	TotalElapsedTimeMs int64        `json:"total_elapsed_time_ms,omitempty"`
}

type InputCommand struct {
	Seq           uint32  `json:"seq"`
	MoveX         float64 `json:"move_x"`
	MoveY         float64 `json:"move_y"`
	Jump          bool    `json:"jump"`
	DropDown      bool    `json:"drop_down"`
	Dash          bool    `json:"dash"`
	PrimaryAttack bool    `json:"primary_attack"`
	Skill1        bool    `json:"skill1"`
	Skill2        bool    `json:"skill2"`
	Skill3        bool    `json:"skill3"`
	Interact      bool    `json:"interact"`
	UseJumpLink   bool    `json:"use_jump_link"`
	AimX          float64 `json:"aim_x"`
	AimY          float64 `json:"aim_y"`
	ClientTime    float64 `json:"client_time"`
}

type InputBatch struct {
	Commands []InputCommand `json:"commands"`
}

type PingMessage struct {
	ClientTime float64 `json:"client_time"`
}

type PongMessage struct {
	ClientTime float64 `json:"client_time"`
	ServerTime float64 `json:"server_time"`
}

type WelcomeMessage struct {
	PlayerID              string  `json:"player_id"`
	PlayerName            string  `json:"player_name"`
	ClassID               string  `json:"class_id,omitempty"`
	RaidID                string  `json:"raid_id"`
	RaidName              string  `json:"raid_name"`
	ContentVersion        string  `json:"content_version"`
	ServerTime            float64 `json:"server_time"`
	TickRate              float64 `json:"tick_rate"`
	SnapshotRate          float64 `json:"snapshot_rate"`
	InterpolationBackTime float64 `json:"interpolation_back_time"`
}

type SnapshotMessage struct {
	ServerTime       float64          `json:"server_time"`
	Tick             uint64           `json:"tick"`
	LocalPlayerID    string           `json:"local_player_id"`
	LastProcessedSeq uint32           `json:"last_processed_seq"`
	Layout           *RaidLayoutState `json:"layout,omitempty"`
	Entities         []EntityState    `json:"entities"`
	Loot             []LootState      `json:"loot"`
	Raid             *RaidState       `json:"raid,omitempty"`
}

type ClientMessage struct {
	Payload ClientPayload `json:"-"`
}

type ClientPayload interface{ isClientPayload() }

type ClientInputPayload struct{ InputBatch }
type ClientPingPayload struct{ PingMessage }

func (ClientInputPayload) isClientPayload() {}
func (ClientPingPayload) isClientPayload()  {}

func NewClientInputMessage(batch InputBatch) ClientMessage {
	return ClientMessage{Payload: ClientInputPayload{InputBatch: batch}}
}

func NewClientPingMessage(ping PingMessage) ClientMessage {
	return ClientMessage{Payload: ClientPingPayload{PingMessage: ping}}
}

func (m ClientMessage) MarshalJSON() ([]byte, error) {
	encoded := struct {
		Input *InputBatch  `json:"input,omitempty"`
		Ping  *PingMessage `json:"ping,omitempty"`
	}{}
	switch payload := m.Payload.(type) {
	case ClientInputPayload:
		batch := payload.InputBatch
		encoded.Input = &batch
	case *ClientInputPayload:
		if payload != nil {
			batch := payload.InputBatch
			encoded.Input = &batch
		}
	case ClientPingPayload:
		ping := payload.PingMessage
		encoded.Ping = &ping
	case *ClientPingPayload:
		if payload != nil {
			ping := payload.PingMessage
			encoded.Ping = &ping
		}
	}
	return json.Marshal(encoded)
}

func (m *ClientMessage) UnmarshalJSON(data []byte) error {
	decoded := struct {
		Input *InputBatch  `json:"input"`
		Ping  *PingMessage `json:"ping"`
	}{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	m.Payload = nil
	if decoded.Input != nil {
		m.Payload = ClientInputPayload{InputBatch: *decoded.Input}
		return nil
	}
	if decoded.Ping != nil {
		m.Payload = ClientPingPayload{PingMessage: *decoded.Ping}
	}
	return nil
}

type ServerMessage struct {
	Type     string           `json:"type"`
	Welcome  *WelcomeMessage  `json:"welcome,omitempty"`
	Snapshot *SnapshotMessage `json:"snapshot,omitempty"`
	Pong     *PongMessage     `json:"pong,omitempty"`
	Error    string           `json:"error,omitempty"`
}
