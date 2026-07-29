# Production deployment

The app is a single Go binary listening on plain HTTP (`PORT`, default 8080).
[Caddy](https://caddyserver.com) sits in front, terminates TLS for `decay.events`,
and proxies to it on loopback. systemd runs the binary and supplies its config.

Files in this directory:

- `decay-main.service` — the systemd unit (installs to `/etc/systemd/system/`)
- `decay-main.env.example` — production config template (installs to `/etc/decay-main.env`)
- `Caddyfile` — TLS + reverse proxy (installs to `/etc/caddy/Caddyfile`)

## One-time server setup

```bash
# 1. App user and directory
sudo useradd --system --home /opt/decay-main --shell /usr/sbin/nologin decay
sudo mkdir -p /opt/decay-main
sudo git clone https://github.com/vwjake/decay-main /opt/decay-main
sudo chown -R decay:decay /opt/decay-main

# 2. Go toolchain at /usr/local/go (deploy.sh expects it there)
#    https://go.dev/doc/install

# 3. Config — fill in real secrets, then lock it down
sudo cp /opt/decay-main/deploy/decay-main.env.example /etc/decay-main.env
sudo nano /etc/decay-main.env            # set SESSION_SECRET, ADMIN_PASSWORD, etc.
sudo chown root:decay /etc/decay-main.env
sudo chmod 640 /etc/decay-main.env

# 4. systemd service
sudo cp /opt/decay-main/deploy/decay-main.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now decay-main

# 5. Caddy (TLS + proxy) — DNS for decay.events must already point here
sudo apt install caddy                   # or per https://caddyserver.com/docs/install
sudo cp /opt/decay-main/deploy/Caddyfile /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

Caddy obtains and renews the Let's Encrypt certificate on its own — no certbot.

## Redeploying after that

```bash
sudo -u decay /opt/decay-main/deploy.sh
```

Pulls `master`, rebuilds, restarts the service. Config in `/etc/decay-main.env`
is left untouched — edit it and `systemctl restart decay-main` to change config.

## Notes

- The uploads dir (`/opt/decay-main/uploads/`, flyers ~341MB) and `decay.db` are
  **not** in git. They must exist on the host and be writable by the `decay` user.
  A fresh DB self-seeds; migrating real content is a separate step.
- Admin session cookies are marked `Secure` automatically whenever `SITE_URL` is
  `https://…`, so keep it set to the real HTTPS origin.
