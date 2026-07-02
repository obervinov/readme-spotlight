// Package logs is a tiny process-wide activity log. Every entry is written to
// stderr (for the terminal / container logs) and kept in a bounded ring buffer
// that the web UI renders, so what the pipeline is doing is visible in both
// places without wiring a logger through every call.
package logs

import (
	"fmt"
	"log"
	"sync"
	"time"
)

const maxEntries = 200

// Entry is a single timestamped log line.
type Entry struct {
	Time time.Time
	Msg  string
}

var (
	mu      sync.Mutex
	entries []Entry
)

// Infof records a formatted line.
func Infof(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Println(msg)

	mu.Lock()
	entries = append(entries, Entry{Time: time.Now(), Msg: msg})
	if len(entries) > maxEntries {
		entries = entries[len(entries)-maxEntries:]
	}
	mu.Unlock()
}

// Recent returns up to n most recent entries, oldest first.
func Recent(n int) []Entry {
	mu.Lock()
	defer mu.Unlock()
	if n <= 0 || n > len(entries) {
		n = len(entries)
	}
	out := make([]Entry, n)
	copy(out, entries[len(entries)-n:])
	return out
}
