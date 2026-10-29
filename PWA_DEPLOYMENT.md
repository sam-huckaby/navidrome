# Private HTTPS and Music PWA deployment

Navidrome embeds the deployable build of the separate Music client in `ui/pwa/` and serves it at `https://music.example.com/pwa/`. Navidrome's `/rest/` API remains on the same origin. To update the PWA, replace **all** files in `ui/pwa/` with a fresh build from `Music/pwa/dist/`, then rebuild and deploy Navidrome. Do not deploy `pwa/src/` or `node_modules/`. The PWA build uses absolute `/pwa/` and `/rest/` paths, so use a root `BaseURL` (no path prefix).

## DNS and certificates

1. Use local DNS (or VPN DNS) to resolve your chosen hostname (for example, `music.example.com`) to Navidrome's private IP on each iPhone. ACME DNS-01 does **not** require a public A/AAAA record or inbound Internet access. Cloudflare must remain authoritative for the public domain's DNS zone, and Navidrome needs outbound HTTPS access to Cloudflare and Let's Encrypt.
2. In Cloudflare, create a `CF_API_TOKEN` with **Zone DNS: Edit** for your domain's zone to manage temporary `_acme-challenge` TXT records. Create a separate `CF_ZONE_TOKEN` with **Zone: Read** for all zones for zone discovery. The Cloudflare DNS library's single-token mode instead requires Zone: Read and Zone DNS: Edit across all zones; the two-token setup avoids granting DNS-edit access to other zones. Never use a global API key.
3. Copy `.env.example` to a private `.env` **on the production server**, fill in the values, and provide them as environment variables to the Navidrome process. `ND_ACMEHOST` enables DNS-01 issuance and automatic renewal. `CF_API_TOKEN` and `CF_ZONE_TOKEN` must be present at runtime for the recommended scoped-token setup; they are not compiled into the binary. Keep the file readable only by the service account. `ND_ACMEEMAIL` is recommended for expiration notices.
4. Keep Navidrome's `DataFolder` persistent and writable. ACME account data, certificates and keys are saved in `<DataFolder>/certmagic/`; back this up or retain it across deployments. Do not set `TLSCert` or `TLSKey` when `ACMEHost` is set. Without `ACMEHost`, the existing manual certificate option still works.
5. Make the server available to iPhones on TCP **443**. Either set `ND_PORT=443` (requires permission to bind a privileged port) or map host port 443 to the service's default port 4533. For Docker Compose, for example:

   ```yaml
   services:
     navidrome:
       ports:
         - "443:4533"
       env_file:
         - .env
       volumes:
         - navidrome_data:/data
   ```

   For systemd, reference the private file with `EnvironmentFile=/path/to/.env` in the service unit. Compose's `.env` substitution alone does not send variables to the container; `env_file` (or explicit `environment` entries) does. Set `ND_DATAFOLDER` appropriately for your deployment. No reverse proxy or public port 80 is required.

## Check

With an iPhone on the private network, open your configured HTTPS hostname's `/pwa/` path in Safari, verify the certificate is trusted, and use **Share → Add to Home Screen**. Check that `/pwa/albums/123` loads the PWA, missing `/pwa/assets/` paths return 404, and API requests including `/rest/stream.view` and cover art remain on the same HTTPS hostname. Media streams retain their query authentication and Range/206 behavior because PWA routes do not intercept `/rest/`.
