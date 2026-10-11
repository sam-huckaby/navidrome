package log

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestRecordStoreHistoryAndSubscription(t *testing.T) {
	store := &recordStore{subscribers: make(map[chan Record]struct{})}
	for i := range logHistorySize + 5 {
		store.add(Record{Message: fmt.Sprint(i)})
	}
	if len(store.history) != logHistorySize || store.history[0].ID != 6 {
		t.Fatalf("unexpected bounded history: length=%d first=%d", len(store.history), store.history[0].ID)
	}

	// Subscribe uses the package store so swap it for an isolated test store.
	previous := records
	records = store
	t.Cleanup(func() { records = previous })
	history, live, unsubscribe := Subscribe(204)
	if len(history) != 1 || history[0].ID != 205 {
		t.Fatalf("unexpected replay: %+v", history)
	}
	store.add(Record{Message: "new"})
	if next := <-live; next.ID != 206 || next.Message != "new" {
		t.Fatalf("unexpected live record: %+v", next)
	}
	unsubscribe()
	if len(store.subscribers) != 0 {
		t.Fatal("subscriber was not removed")
	}
}

func TestRecordHookRedactsEvenWhenNormalOutputDoesNot(t *testing.T) {
	store := &recordStore{subscribers: make(map[chan Record]struct{})}
	entry := &logrus.Entry{
		Time:    time.Now(),
		Level:   logrus.InfoLevel,
		Message: "request ?jwt=secret-token",
		Data: logrus.Fields{
			"Authorization": "Bearer secret-token",
			"query":         "?api_key=another-secret",
			"header":        `{"Cookie":["session=third-secret"]}`,
		},
	}
	if err := (&recordHook{store: store}).Fire(entry); err != nil {
		t.Fatal(err)
	}
	got := store.history[0].Message
	if strings.Contains(got, "secret-token") || strings.Contains(got, "another-secret") || strings.Contains(got, "third-secret") {
		t.Fatalf("secrets appeared in UI log: %s", got)
	}
	if entry.Message != "request ?jwt=secret-token" {
		t.Fatal("capture modified the original log entry")
	}
}

func TestSlowSubscriberDoesNotBlockLogging(t *testing.T) {
	store := &recordStore{subscribers: make(map[chan Record]struct{})}
	ch := make(chan Record, logSubscriberSize)
	store.subscribers[ch] = struct{}{}
	for i := range logSubscriberSize + 1 {
		store.add(Record{Message: fmt.Sprint(i)})
	}
	if len(ch) != logSubscriberSize || (<-ch).ID != 2 {
		t.Fatal("oldest queued entry was not evicted for a slow subscriber")
	}
}
