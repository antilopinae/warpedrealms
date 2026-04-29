package serverapp

import (
	"fmt"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"warpedrealms/content"
	"warpedrealms/shared"
)

type raidCreateTask struct {
	jobID      string
	enqueuedAt time.Time
	async      bool
	resultChan chan raidCreateResult
}

type raidCreateResult struct {
	summary shared.RaidSummary
	err     error
}

type raidJobStatus string

const (
	raidJobInProgress raidJobStatus = "in_progress"
	raidJobReady      raidJobStatus = "ready"
	raidJobFailed     raidJobStatus = "failed"
)

type raidJob struct {
	ID                 string
	Status             raidJobStatus
	Raid               *shared.RaidSummary
	Error              string
	QueuedAt           time.Time
	StartedAt          time.Time
	FinishedAt         time.Time
	QueueWait          time.Duration
	GenerationDuration time.Duration
}

type SessionManager struct {
	mu       sync.RWMutex
	bundle   *content.Bundle
	raids    map[string]*RaidRoom
	nextRaid int
	nextJob  uint64

	createQueue chan raidCreateTask
	jobs        map[string]*raidJob

	queueDepth          atomic.Int64
	totalQueueWaitNanos atomic.Int64
	totalGenNanos       atomic.Int64
}

func NewSessionManager(bundle *content.Bundle) *SessionManager {
	workers := runtime.NumCPU() / 2
	if workers < 1 {
		workers = 1
	}
	manager := &SessionManager{
		bundle:      bundle,
		raids:       make(map[string]*RaidRoom),
		nextRaid:    1,
		createQueue: make(chan raidCreateTask, workers*4),
		jobs:        make(map[string]*raidJob),
	}
	for i := 0; i < workers; i++ {
		go manager.raidCreatorWorker()
	}
	manager.createRaidSync(5 * time.Second)
	return manager
}

func (m *SessionManager) raidCreatorWorker() {
	for task := range m.createQueue {
		m.queueDepth.Add(-1)
		startedAt := time.Now()
		m.setJobStarted(task.jobID, startedAt, startedAt.Sub(task.enqueuedAt))
		result := m.createRaidNow(task)
		if task.resultChan != nil {
			task.resultChan <- result
		}
	}
}

func (m *SessionManager) createRaidNow(task raidCreateTask) raidCreateResult {
	m.mu.Lock()
	id := fmt.Sprintf("raid-%03d", m.nextRaid)
	name := fmt.Sprintf("Raid %02d", m.nextRaid)
	m.nextRaid++
	m.mu.Unlock()

	seed := time.Now().UnixNano() + int64(len(id)*97)
	startGen := time.Now()
	room, err := NewRaidRoomProcGen(id, name, m.bundle, seed, shared.DefaultRaidMaxPlayers, shared.DefaultRaidDuration)
	genDuration := time.Since(startGen)
	m.totalGenNanos.Add(genDuration.Nanoseconds())
	if err != nil {
		m.setJobFinished(task.jobID, raidCreateResult{err: err}, genDuration)
		return raidCreateResult{err: err}
	}
	room.Start()
	m.mu.Lock()
	m.raids[id] = room
	summary := room.Summary()
	m.mu.Unlock()
	m.setJobFinished(task.jobID, raidCreateResult{summary: summary}, genDuration)
	return raidCreateResult{summary: summary}
}

func (m *SessionManager) enqueueCreateRaid(async bool) (string, chan raidCreateResult) {
	jobID := fmt.Sprintf("job-%06d", atomic.AddUint64(&m.nextJob, 1))
	queuedAt := time.Now()
	job := &raidJob{ID: jobID, Status: raidJobInProgress, QueuedAt: queuedAt}
	m.mu.Lock()
	m.jobs[jobID] = job
	m.mu.Unlock()
	var resultChan chan raidCreateResult
	if !async {
		resultChan = make(chan raidCreateResult, 1)
	}
	task := raidCreateTask{jobID: jobID, enqueuedAt: queuedAt, async: async, resultChan: resultChan}
	m.queueDepth.Add(1)
	m.createQueue <- task
	return jobID, resultChan
}

func (m *SessionManager) createRaidSync(timeout time.Duration) (shared.RaidSummary, string, error) {
	jobID, resultChan := m.enqueueCreateRaid(false)
	select {
	case res := <-resultChan:
		return res.summary, jobID, res.err
	case <-time.After(timeout):
		return shared.RaidSummary{}, jobID, fmt.Errorf("raid creation timeout")
	}
}

func (m *SessionManager) CreateRaid() shared.RaidSummary {
	summary, _, err := m.createRaidSync(5 * time.Second)
	if err != nil {
		return shared.RaidSummary{Phase: shared.RaidPhaseFinished, MaxPlayers: shared.DefaultRaidMaxPlayers, Duration: shared.DefaultRaidDuration}
	}
	return summary
}

func (m *SessionManager) CreateRaidAsync() string {
	jobID, _ := m.enqueueCreateRaid(true)
	return jobID
}
func (m *SessionManager) GetRaidJob(jobID string) (*raidJob, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	j, ok := m.jobs[jobID]
	return j, ok
}

func (m *SessionManager) setJobStarted(jobID string, startedAt time.Time, wait time.Duration) {
	m.totalQueueWaitNanos.Add(wait.Nanoseconds())
	m.mu.Lock()
	defer m.mu.Unlock()
	if j, ok := m.jobs[jobID]; ok {
		j.StartedAt = startedAt
		j.QueueWait = wait
	}
}
func (m *SessionManager) setJobFinished(jobID string, result raidCreateResult, gen time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j, ok := m.jobs[jobID]; ok {
		j.FinishedAt = time.Now()
		j.GenerationDuration = gen
		if result.err != nil {
			j.Status = raidJobFailed
			j.Error = result.err.Error()
		} else {
			j.Status = raidJobReady
			j.Raid = &result.summary
		}
	}
}

func (m *SessionManager) QueueMetrics() (int64, time.Duration, time.Duration) {
	depth := m.queueDepth.Load()
	m.mu.RLock()
	jobs := len(m.jobs)
	m.mu.RUnlock()
	if jobs == 0 {
		return depth, 0, 0
	}
	avgWait := time.Duration(m.totalQueueWaitNanos.Load() / int64(jobs))
	avgGen := time.Duration(m.totalGenNanos.Load() / int64(jobs))
	return depth, avgWait, avgGen
}

func (m *SessionManager) ListRaids() []shared.RaidSummary { /* unchanged */
	m.mu.RLock()
	rooms := make([]*RaidRoom, 0, len(m.raids))
	for _, room := range m.raids {
		rooms = append(rooms, room)
	}
	m.mu.RUnlock()
	summaries := make([]shared.RaidSummary, 0, len(rooms))
	finishedEmpty := make([]string, 0)
	for _, room := range rooms {
		s := room.Summary()
		if s.Phase == shared.RaidPhaseFinished && s.CurrentPlayers == 0 {
			finishedEmpty = append(finishedEmpty, s.ID)
			continue
		}
		summaries = append(summaries, s)
	}
	if len(finishedEmpty) > 0 {
		m.mu.Lock()
		for _, id := range finishedEmpty {
			delete(m.raids, id)
		}
		m.mu.Unlock()
	}
	if len(summaries) == 0 {
		summaries = append(summaries, m.CreateRaid())
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].ID < summaries[j].ID })
	return summaries
}

func (m *SessionManager) GetRaid(id string) (*RaidRoom, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	room, ok := m.raids[id]
	return room, ok
}
