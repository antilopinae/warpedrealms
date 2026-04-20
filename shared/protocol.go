package shared

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

type InputCommand struct {
	Seq           uint32  `json:"seq"`
	MoveX         float64 `json:"move_x"`
	MoveY         float64 `json:"move_y"`
	Jump          bool    `json:"jump"`
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
	Type  string       `json:"type"`
	Input *InputBatch  `json:"input,omitempty"`
	Ping  *PingMessage `json:"ping,omitempty"`
}

type ServerMessage struct {
	Type     string           `json:"type"`
	Welcome  *WelcomeMessage  `json:"welcome,omitempty"`
	Snapshot *SnapshotMessage `json:"snapshot,omitempty"`
	Pong     *PongMessage     `json:"pong,omitempty"`
	Error    string           `json:"error,omitempty"`
}
