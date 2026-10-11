package log

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	logHistorySize    = 200
	logMessageSize    = 4096
	logSubscriberSize = 64
)

// Record is a redacted Navidrome log entry suitable for display in the UI.
type Record struct {
	ID      uint64    `json:"id"`
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

type recordStore struct {
	mu          sync.Mutex
	history     []Record
	nextID      uint64
	subscribers map[chan Record]struct{}
}

var records = &recordStore{subscribers: make(map[chan Record]struct{})}

type recordHook struct{ store *recordStore }

func (h *recordHook) Levels() []logrus.Level { return logrus.AllLevels }

func (h *recordHook) Fire(entry *logrus.Entry) error {
	// Redact a copy even when redaction of the server's normal output is disabled.
	// Never modify the entry passed to the other hooks or the output formatter.
	copyEntry := *entry
	copyEntry.Data = make(logrus.Fields, len(entry.Data))
	for key, value := range entry.Data {
		copyEntry.Data[key] = value
	}
	if err := redacted.Fire(&copyEntry); err != nil {
		return err
	}

	keys := make([]string, 0, len(copyEntry.Data))
	for key := range copyEntry.Data {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	var message strings.Builder
	message.WriteString(copyEntry.Message)
	for _, key := range keys {
		value := copyEntry.Data[key]
		lowerKey := strings.ToLower(key)
		if strings.Contains(lowerKey, "password") || strings.Contains(lowerKey, "secret") ||
			strings.Contains(lowerKey, "token") || strings.Contains(lowerKey, "apikey") ||
			strings.Contains(lowerKey, "api_key") || strings.Contains(lowerKey, "authorization") ||
			strings.Contains(lowerKey, "cookie") || strings.Contains(lowerKey, "credential") ||
			lowerKey == "header" || lowerKey == "headers" || lowerKey == "request" {
			value = "[REDACTED]"
		}
		fmt.Fprintf(&message, " %s=%v", key, value)
	}
	// Redact again after formatting to cover values that were not strings (or
	// contained secrets only once converted to text).
	text := Redact(message.String())
	if len(text) > logMessageSize {
		text = text[:logMessageSize]
	}
	h.store.add(Record{Time: entry.Time, Level: entry.Level.String(), Message: text})
	return nil
}

func (s *recordStore) add(record Record) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	record.ID = s.nextID
	if len(s.history) == logHistorySize {
		copy(s.history, s.history[1:])
		s.history[len(s.history)-1] = record
	} else {
		s.history = append(s.history, record)
	}
	for ch := range s.subscribers {
		select {
		case ch <- record:
		default:
			// A slow browser must never hold up application logging.
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- record:
			default:
			}
		}
	}
}

// Subscribe returns an atomic snapshot and a stream of subsequent entries.
// Passing a nonzero cursor returns only newer entries; a cursor from a previous
// server process falls back to the current history.
func Subscribe(after uint64) ([]Record, <-chan Record, func()) {
	s := records
	s.mu.Lock()
	defer s.mu.Unlock()
	if after > s.nextID {
		after = 0
	}
	history := make([]Record, 0, len(s.history))
	for _, record := range s.history {
		if record.ID > after {
			history = append(history, record)
		}
	}
	ch := make(chan Record, logSubscriberSize)
	s.subscribers[ch] = struct{}{}
	return history, ch, func() {
		s.mu.Lock()
		delete(s.subscribers, ch)
		s.mu.Unlock()
	}
}
