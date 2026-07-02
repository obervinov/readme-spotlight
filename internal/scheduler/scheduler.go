// Package scheduler is a thin wrapper around a single reconfigurable cron entry.
// It only handles timing; the callback decides what work to run.
package scheduler

import (
	"sync"

	"github.com/robfig/cron/v3"
)

// Scheduler owns one recurring cron entry.
type Scheduler struct {
	cron *cron.Cron

	mu      sync.Mutex
	entryID cron.EntryID
}

// New creates an empty Scheduler.
func New() *Scheduler {
	return &Scheduler{cron: cron.New()}
}

// Start begins the cron loop.
func (s *Scheduler) Start() { s.cron.Start() }

// Stop halts the cron loop, waiting for a running job to finish.
func (s *Scheduler) Stop() { <-s.cron.Stop().Done() }

// Reschedule replaces the recurring entry so it invokes fn on spec. An empty
// spec disables recurring runs.
func (s *Scheduler) Reschedule(spec string, fn func()) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entryID != 0 {
		s.cron.Remove(s.entryID)
		s.entryID = 0
	}
	if spec == "" {
		return nil
	}
	id, err := s.cron.AddFunc(spec, fn)
	if err != nil {
		return err
	}
	s.entryID = id
	return nil
}
