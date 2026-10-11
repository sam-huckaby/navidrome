package nativeapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/navidrome/navidrome/log"
)

const (
	logKeepAlive      = 15 * time.Second
	logStreamLifetime = 5 * time.Minute
	logWriteTimeout   = 20 * time.Second
)

func streamLogs(w http.ResponseWriter, r *http.Request) {
	var after uint64
	if cursor := r.URL.Query().Get("since"); cursor != "" {
		var err error
		after, err = strconv.ParseUint(cursor, 10, 64)
		if err != nil {
			http.Error(w, "Invalid log cursor", http.StatusBadRequest)
			return
		}
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	history, live, unsubscribe := log.Subscribe(after)
	defer unsubscribe()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	if !writeLogEvent(w, flusher, nil) {
		return
	}
	for _, record := range history {
		if !writeLogEvent(w, flusher, &record) {
			return
		}
	}

	keepAlive := time.NewTicker(logKeepAlive)
	defer keepAlive.Stop()
	lifetime := time.NewTimer(logStreamLifetime)
	defer lifetime.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-lifetime.C:
			return // Reconnect through authentication, including changes to admin status.
		case <-keepAlive.C:
			if !writeLogEvent(w, flusher, nil) {
				return
			}
		case record := <-live:
			if !writeLogEvent(w, flusher, &record) {
				return
			}
		}
	}
}

func writeLogEvent(w http.ResponseWriter, flusher http.Flusher, record *log.Record) bool {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(logWriteTimeout))
	if record == nil {
		_, err := fmt.Fprint(w, ": keepalive\n\n")
		if err != nil {
			return false
		}
	} else {
		data, err := json.Marshal(record)
		if err != nil {
			return false
		}
		if _, err = fmt.Fprintf(w, "id: %d\nevent: log\ndata: %s\n\n", record.ID, data); err != nil {
			return false
		}
	}
	flusher.Flush()
	return true
}
