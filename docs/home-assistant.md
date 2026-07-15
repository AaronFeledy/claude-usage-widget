# Home Assistant REST Sensor

Use Home Assistant's REST sensor to read the Claude Usage Widget server API. This is a plain `configuration.yaml` example; a Home Assistant add-on is future work and does not exist in this repository yet.

The server default `127.0.0.1:7823` only works when Home Assistant runs on the same host namespace as the server. For a Raspberry Pi, NAS, or Docker host, run `usage-server` with a non-loopback bind and a bearer token:

```bash
USAGE_AUTH_TOKEN='replace-with-a-long-random-token' \
  ./usage-server --listen-addr 0.0.0.0:7823
```

Store the Home Assistant header value in `secrets.yaml`, including the `Bearer ` prefix:

```yaml
claude_usage_widget_auth: Bearer replace-with-a-long-random-token
```

## REST Sensors

The server endpoint is `GET /api/v1/usage`. It returns one array entry per enabled provider, sorted by provider name. With all four providers enabled, the order is `Claude`, `Codex`, `Cursor`, then `Grok`.

The `current` and `weekly` sensors below are unchanged and remain valid: keep any existing configuration exactly as it is. `buckets` is a new, additive array field added alongside them, described in [Returned Fields](#returned-fields).

Copy this into `configuration.yaml` and replace `http://usage-server.local:7823` with your server URL:

```yaml
sensor:
  - platform: rest
    name: "Claude Usage Current"
    unique_id: claude_usage_widget_claude_current
    resource: "http://usage-server.local:7823/api/v1/usage"
    scan_interval: 60
    headers:
      Authorization: !secret claude_usage_widget_auth
    value_template: "{{ value_json[0].current.utilization | round(1) }}"
    unit_of_measurement: "%"
    json_attributes_path: "$[0]"
    json_attributes:
      - provider_name
      - primary_label
      - secondary_label
      - show_secondary
      - subtitle
      - primary_status_text
      - secondary_status_text
      - reauth_command
      - current
      - weekly
      - buckets
      - error
      - needs_reauth
      - is_success

  - platform: rest
    name: "Codex Usage Current"
    unique_id: claude_usage_widget_codex_current
    resource: "http://usage-server.local:7823/api/v1/usage"
    scan_interval: 60
    headers:
      Authorization: !secret claude_usage_widget_auth
    value_template: "{{ value_json[1].current.utilization | round(1) }}"
    unit_of_measurement: "%"
    json_attributes_path: "$[1]"
    json_attributes:
      - provider_name
      - primary_label
      - secondary_label
      - show_secondary
      - subtitle
      - primary_status_text
      - secondary_status_text
      - reauth_command
      - current
      - weekly
      - buckets
      - error
      - needs_reauth
      - is_success

  - platform: rest
    name: "Cursor Usage Current"
    unique_id: claude_usage_widget_cursor_current
    resource: "http://usage-server.local:7823/api/v1/usage"
    scan_interval: 60
    headers:
      Authorization: !secret claude_usage_widget_auth
    value_template: "{{ value_json[2].current.utilization | round(1) }}"
    unit_of_measurement: "%"
    json_attributes_path: "$[2]"
    json_attributes:
      - provider_name
      - primary_label
      - secondary_label
      - show_secondary
      - subtitle
      - primary_status_text
      - secondary_status_text
      - reauth_command
      - current
      - weekly
      - buckets
      - error
      - needs_reauth
      - is_success

  - platform: rest
    name: "Grok Usage Current"
    unique_id: claude_usage_widget_grok_current
    resource: "http://usage-server.local:7823/api/v1/usage"
    scan_interval: 60
    headers:
      Authorization: !secret claude_usage_widget_auth
    value_template: "{{ value_json[3].current.utilization | round(1) }}"
    unit_of_measurement: "%"
    json_attributes_path: "$[3]"
    json_attributes:
      - provider_name
      - primary_label
      - secondary_label
      - show_secondary
      - subtitle
      - primary_status_text
      - secondary_status_text
      - reauth_command
      - current
      - weekly
      - buckets
      - error
      - needs_reauth
      - is_success
```

If you enable only some providers, adjust the array indexes to match the returned `/api/v1/usage` order. To avoid index dependence, you can also create one REST sensor per provider endpoint, such as `/api/v1/usage/Claude`; the JSON paths then start at `value_json.current.utilization` instead of `value_json[0].current.utilization`.

## Returned Fields

Each provider entry has this shape:

```json
{
  "provider_name": "Claude",
  "primary_label": "Current Session",
  "secondary_label": "Weekly",
  "show_secondary": true,
  "subtitle": null,
  "primary_status_text": null,
  "secondary_status_text": null,
  "reauth_command": null,
  "current": { "utilization": 42.5, "resets_at": "2026-07-12T18:00:00Z" },
  "weekly": { "utilization": 17.0, "resets_at": null },
  "buckets": [
    { "id": "session", "label": "Current Session", "utilization": 42.5, "resets_at": "2026-07-12T18:00:00Z", "status_text": null },
    { "id": "weekly", "label": "Weekly", "utilization": 17.0, "resets_at": null, "status_text": null },
    { "id": "weekly_fable", "label": "Fable", "utilization": 5.0, "resets_at": null, "status_text": null },
    { "id": "extra", "label": "Extra usage", "utilization": 12.0, "resets_at": null, "status_text": "120 / 1000 credits" }
  ],
  "error": null,
  "needs_reauth": false,
  "is_success": true
}
```

Optional strings and reset timestamps are explicit `null`. `is_success` is true only when `error` is `null`.

`buckets` is always present, is `[]` on error, and lists every usage window a provider reports (typically `session` and `weekly`; model-scoped rows like `weekly_fable`; and credit meters like `extra` / `on_demand` when the account has them enabled or has non-zero spend). Optional `status_text` overrides the reset line (e.g. credit totals). `current` and `weekly` are still populated from the first two header meters for backward compatibility, so existing sensors reading those fields keep working unchanged.
