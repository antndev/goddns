# goddns

Modular Go DDNS manager with file-based configuration, multiple named profiles, and pluggable source and target backends.

## Features

- Multiple named sources for IPv4 and IPv6
- Multiple named targets
- Bindings that connect any source to any target
- Modular source/target design with reusable named profiles
- Local IP discovery via external IP endpoints with fallback URLs
- OPNsense source via HTTPS API
- Hetzner DNS target via RRset `set_records`
- Single-run mode and per-source polling intervals
- Minimal health endpoint for monitoring

## Config model

1. Define `sources`.
2. Define `targets`.
3. Create `bindings` that connect them.

Every source must define `check_interval`, for example `60s` or `300s`.

Health is configured under `health`, with `enabled` and `listen`.

Current built-in presets:

- Sources: `opnsense`, `local`
- Targets: `hetzner`

Example:

```yaml
health:
  enabled: true
  listen: ":8080"

sources:
  wan-v4:
    type: opnsense
    family: ipv4
    check_interval: 60s
    base_url: https://opnsense.example.internal
    api_key: replace-me
    api_secret: replace-me
    interface: wan

targets:
  root-a:
    type: hetzner
    api_token: replace-me
    zone: example.com
    record_name: "@"
    record_type: A
    ttl: 60

bindings:
  - source: wan-v4
    target: root-a
```

## Run with Docker

```bash
# create config.yaml from the example above
docker compose up -d
```

The compose file mounts the config at `/app/config.yaml`.

Local sources query external IP endpoints and fall back across multiple URLs if one is down.

Each source is rechecked on its own `check_interval`, so one profile can run every `60s` while another runs every `300s`.

By default no container ports are published, so the health endpoint is not exposed outside the container unless you explicitly add a port mapping.

## Health

`GET /health` returns status only:

- `200` after the latest reconciliation cycle succeeded
- `503` before the first successful cycle, or after a failed cycle
- `404` for every other path
- `405` for non-`GET` and non-`HEAD` requests to `/health`

Set `health.enabled: false` to disable the HTTP health server entirely.

## Run once

```bash
docker compose run --rm goddns -config /app/config.yaml -once
```

`-once` is a CLI flag for a single reconciliation cycle. It is not part of the YAML config.

## Notes

- The default OPNsense endpoint is `/api/diagnostics/interface/getInterfaceConfig`, matching the Python app you shared.
- The Hetzner target uses `POST /zones/{zone}/rrsets/{record}/{type}/actions/set_records` on `https://api.hetzner.cloud/v1`.
- The manager keeps the HTTP surface intentionally tiny: health only.
- Local sources accept `external_urls` and try them in order until one returns a valid IP.
- Source `check_interval` is required and controls how often that specific source is re-resolved.
