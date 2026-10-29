package server

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/navidrome/navidrome/conf"
)

func TestACMEConfigurationRequiresToken(t *testing.T) {
	old := conf.Server.ACMEHost
	conf.Server.ACMEHost = "music.example.com"
	t.Cleanup(func() { conf.Server.ACMEHost = old })
	t.Setenv("CF_API_TOKEN", "")
	_, err := configureACMETLS(context.Background(), &http.Server{})
	if err == nil || !strings.Contains(err.Error(), "CF_API_TOKEN") {
		t.Fatalf("expected missing token error, got %v", err)
	}
}

func TestACMEConfigurationRejectsInvalidHost(t *testing.T) {
	old := conf.Server.ACMEHost
	conf.Server.ACMEHost = "https://music.example.com:443"
	t.Cleanup(func() { conf.Server.ACMEHost = old })
	_, err := configureACMETLS(context.Background(), &http.Server{})
	if err == nil || !strings.Contains(err.Error(), "ACMEHost") {
		t.Fatalf("expected hostname error, got %v", err)
	}
}
