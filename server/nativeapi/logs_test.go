package nativeapi

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/conf/configtest"
	"github.com/navidrome/navidrome/core/auth"
	"github.com/navidrome/navidrome/core/playlists"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/server"
	"github.com/navidrome/navidrome/tests"
)

func TestLogStreamAdminAccessAndReplay(t *testing.T) {
	defer configtest.SetupConfig()()
	conf.Server.SessionTimeout = 48 * time.Hour
	previousLevel := log.CurrentLevel()
	log.SetLevel(log.LevelInfo)
	defer log.SetLevel(previousLevel)
	ds := &tests.MockDataStore{}
	admin := &model.User{ID: "logs-admin", UserName: "logs-admin", IsAdmin: true, NewPassword: "pass"}
	regular := &model.User{ID: "logs-regular", UserName: "logs-regular", NewPassword: "pass"}
	for _, user := range []*model.User{admin, regular} {
		if err := ds.User().Put(context.Background(), user); err != nil {
			t.Fatal(err)
		}
	}
	auth.Init(ds)
	api := New(ds, nil, playlists.NewPlaylists(ds, nil), nil, tests.NewMockLibraryService(), tests.NewMockUserService(), nil, nil, nil, nil, nil)
	httpServer := httptest.NewServer(server.JWTVerifier(api))
	defer httpServer.Close()
	get := func(token string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, httpServer.URL+"/logs/stream", nil)
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			req.Header.Set("X-ND-Authorization", "Bearer "+token)
		}
		resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	resp := get("")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous status: %d", resp.StatusCode)
	}
	resp.Body.Close()
	regularToken, err := auth.CreateToken(regular)
	if err != nil {
		t.Fatal(err)
	}
	resp = get(regularToken)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("regular user status: %d", resp.StatusCode)
	}
	resp.Body.Close()

	log.Info("live logs replay marker ?jwt=very-secret")
	adminToken, err := auth.CreateToken(admin)
	if err != nil {
		t.Fatal(err)
	}
	resp = get(adminToken)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("admin response: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	scanner := bufio.NewScanner(resp.Body)
	var replayID uint64
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var record log.Record
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &record); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(record.Message, "live logs replay marker") {
			if strings.Contains(record.Message, "very-secret") || record.ID == 0 {
				t.Fatalf("unredacted or invalid record: %+v", record)
			}
			replayID = record.ID
			break
		}
	}
	if replayID == 0 {
		t.Fatalf("replay marker not found: %v", scanner.Err())
	}
	log.Info("live logs new entry")
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var record log.Record
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &record); err != nil {
			t.Fatal(err)
		}
		if record.Message == "live logs new entry" {
			if record.ID <= replayID {
				t.Fatalf("live record did not follow the replay: %+v", record)
			}
			return
		}
	}
	t.Fatalf("live record not found: %v", scanner.Err())
}
