package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"

	"github.com/caddyserver/certmagic"
	"github.com/libdns/cloudflare"
	"github.com/navidrome/navidrome/conf"
)

var dnsHostname = regexp.MustCompile(`(?i)^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+$`)

// configureACMETLS obtains a certificate using DNS-01 and keeps it renewed
// while the server runs. No public HTTP or TLS-ALPN challenge port is needed.
func configureACMETLS(ctx context.Context, server *http.Server) (func(), error) {
	host := conf.Server.ACMEHost
	u, err := url.Parse("https://" + host)
	if err != nil || u.Hostname() != host || u.Port() != "" || net.ParseIP(host) != nil || !dnsHostname.MatchString(host) {
		return nil, fmt.Errorf("ACMEHost must be a DNS hostname without a scheme or port: %q", host)
	}
	token := os.Getenv("CF_API_TOKEN")
	if token == "" {
		return nil, errors.New("ACMEHost requires CF_API_TOKEN in the server environment")
	}
	dataDir, err := conf.Server.DataFolder.Path()
	if err != nil {
		return nil, fmt.Errorf("ACME requires a writable DataFolder: %w", err)
	}
	if dataDir == "" {
		return nil, errors.New("ACME requires a writable DataFolder")
	}
	storage := &certmagic.FileStorage{Path: filepath.Join(dataDir, "certmagic")}
	provider := &cloudflare.Provider{APIToken: token, ZoneToken: os.Getenv("CF_ZONE_TOKEN")}
	var cache *certmagic.Cache
	newConfig := func() *certmagic.Config {
		cfg := certmagic.New(cache, certmagic.Config{Storage: storage})
		cfg.Issuers = []certmagic.Issuer{certmagic.NewACMEIssuer(cfg, certmagic.ACMEIssuer{
			CA:     certmagic.LetsEncryptProductionCA,
			Email:  conf.Server.ACMEEmail,
			Agreed: true,
			DNS01Solver: &certmagic.DNS01Solver{DNSManager: certmagic.DNSManager{
				DNSProvider: provider,
			}},
		})}
		return cfg
	}
	cache = certmagic.NewCache(certmagic.CacheOptions{
		GetConfigForCert: func(certmagic.Certificate) (*certmagic.Config, error) {
			return newConfig(), nil
		},
	})
	cfg := newConfig()
	if err := cfg.ManageSync(ctx, []string{host}); err != nil {
		cache.Stop()
		return nil, fmt.Errorf("obtaining ACME certificate for %s: %w", host, err)
	}
	server.TLSConfig = cfg.TLSConfig()
	return cache.Stop, nil
}
