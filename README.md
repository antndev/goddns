# goddns

Small Go DDNS manager with file-based configuration, Docker packaging, and pluggable IP sources and DNS targets.

## Features

- Multiple named sources for IPv4 and IPv6
- Multiple named targets
- Bindings that connect any source to any target
- Local IP discovery with `outbound`, `interface`, or `static` strategies
- OPNsense source via HTTPS API
- Hetzner DNS target via RRset `set_records`
- Single-run mode and polling mode
- Minimal health endpoint for monitoring

## Config model

1. Define `sources`.
2. Define `targets`.
3. Create `bindings` that connect them.

Example:

```yaml
sources:
  wan-v4:
    type: opnsense
    family: ipv4
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

bindings:
  - source: wan-v4
    target: root-a
```

## Run with Docker

```bash
cp config.example.yaml config.yaml
docker compose up -d --build
```

The container reads `/config/config.yaml`, so mounting a single config file is enough.

## Health

`GET /health` returns status only:

- `200` after the latest reconciliation cycle succeeded
- `503` before the first successful cycle, or after a failed cycle
- `404` for every other path
- `405` for non-`GET` and non-`HEAD` requests to `/health`

## Run once

```bash
docker compose run --rm goddns -config /config/config.yaml -once
```

## Notes

- The default OPNsense endpoint is `/api/diagnostics/interface/getInterfaceConfig`, matching the Python app you shared.
- The Hetzner target uses `POST /zones/{zone}/rrsets/{record}/{type}/actions/set_records` on `https://api.hetzner.cloud/v1`.
- The manager keeps the HTTP surface intentionally tiny: health only.
