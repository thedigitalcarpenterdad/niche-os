# Bot Tool Activity in ClickClack

## Overview

When a Hermes bot agent is actively using a tool (calling an API, reading files, running a search, etc.), it can reflect this in the ClickClack typing indicator bar. Instead of showing:

> **Estimating Agent is typing…**

the UI will show:

> **Estimating Agent is searching Gmail…**
> **Estimating Agent is querying Notion…**
> **Estimating Agent is searching the web…**

## How it works

The feature extends the existing `typing.started` ephemeral event with an optional `tool_name` payload field. No new endpoint is needed — bots use the same `/api/realtime/ephemeral` endpoint they already call for typing signals.

The UI decays the indicator after **6.5 seconds** without a re-ping, so bots should re-send `typing.started` every ~4 seconds while the tool is still running (same cadence as the existing typing heartbeat). Send `typing.stopped` when the tool call completes.

## API Call

**Endpoint:** `POST /api/realtime/ephemeral`  
**Auth:** Bot token (`Authorization: Bearer <token>`)  
**Scope required:** `messages:write`

### Start tool activity

```json
{
  "workspace_id": "<workspace_id>",
  "channel_id": "<channel_id>",
  "type": "typing.started",
  "payload": {
    "tool_name": "gmail_search"
  }
}
```

Or for a direct conversation:

```json
{
  "workspace_id": "<workspace_id>",
  "direct_conversation_id": "<dm_id>",
  "type": "typing.started",
  "payload": {
    "tool_name": "notion_query"
  }
}
```

### Stop tool activity (on completion or error)

```json
{
  "workspace_id": "<workspace_id>",
  "channel_id": "<channel_id>",
  "type": "typing.stopped",
  "payload": {}
}
```

## Recognised `tool_name` values

The frontend maps these to human-readable phrases. Unknown values are shown prettified (underscores → spaces).

| `tool_name`        | Displayed as                     |
|--------------------|----------------------------------|
| `web_search`       | searching the web                |
| `search_web`       | searching the web                |
| `gmail_search`     | searching Gmail                  |
| `gmail`            | searching Gmail                  |
| `search_gmail`     | searching Gmail                  |
| `notion_query`     | querying Notion                  |
| `notion`           | querying Notion                  |
| `read_file`        | reading a file                   |
| `write_file`       | writing a file                   |
| `list_files`       | listing files                    |
| `search_files`     | searching files                  |
| `terminal`         | running a command                |
| `bash`             | running a command                |
| `shell`            | running a command                |
| `github`           | checking GitHub                  |
| `google_calendar`  | checking Calendar                |
| `calendar`         | checking Calendar                |
| *(anything else)*  | *(raw name, underscores→spaces)* |

## Node.js / Hermes Bot Integration Pattern

```typescript
const CLICKCLACK_URL = process.env.CLICKCLACK_URL;
const BOT_TOKEN = process.env.CLICKCLACK_BOT_TOKEN;
const WORKSPACE_ID = process.env.CLICKCLACK_WORKSPACE_ID;
const CHANNEL_ID = process.env.CLICKCLACK_CHANNEL_ID;

const PING_INTERVAL_MS = 4000;

async function signalToolActivity(toolName: string): Promise<() => void> {
  const body = {
    workspace_id: WORKSPACE_ID,
    channel_id: CHANNEL_ID,
    type: "typing.started",
    payload: { tool_name: toolName },
  };

  const ping = () =>
    fetch(`${CLICKCLACK_URL}/api/realtime/ephemeral`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${BOT_TOKEN}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify(body),
    }).catch(() => {}); // best-effort, don't block the tool

  await ping();
  const interval = setInterval(ping, PING_INTERVAL_MS);

  // Returns a stop function
  return async () => {
    clearInterval(interval);
    await fetch(`${CLICKCLACK_URL}/api/realtime/ephemeral`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${BOT_TOKEN}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        workspace_id: WORKSPACE_ID,
        channel_id: CHANNEL_ID,
        type: "typing.stopped",
        payload: {},
      }),
    }).catch(() => {});
  };
}

// Usage example:
const stopSignal = await signalToolActivity("gmail_search");
try {
  const results = await searchGmail(query);
  // ... process results
} finally {
  await stopSignal();
}
```

## Python / Hermes Gateway Pattern

For bots using the Hermes tool call hooks, wrap the tool invocation:

```python
import os, requests, threading

CLICKCLACK_URL = os.environ["CLICKCLACK_URL"]
BOT_TOKEN = os.environ["CLICKCLACK_BOT_TOKEN"]
WORKSPACE_ID = os.environ["CLICKCLACK_WORKSPACE_ID"]
CHANNEL_ID = os.environ["CLICKCLACK_CHANNEL_ID"]

def signal_tool_activity(tool_name: str):
    """Context manager that shows tool activity in ClickClack."""
    import contextlib, time

    @contextlib.contextmanager
    def _ctx():
        stop_event = threading.Event()

        def _ping():
            while not stop_event.wait(4):
                try:
                    requests.post(
                        f"{CLICKCLACK_URL}/api/realtime/ephemeral",
                        headers={"Authorization": f"Bearer {BOT_TOKEN}"},
                        json={
                            "workspace_id": WORKSPACE_ID,
                            "channel_id": CHANNEL_ID,
                            "type": "typing.started",
                            "payload": {"tool_name": tool_name},
                        },
                        timeout=3,
                    )
                except Exception:
                    pass

        # Send first ping immediately
        _ping_once(tool_name)
        t = threading.Thread(target=_ping, daemon=True)
        t.start()
        try:
            yield
        finally:
            stop_event.set()
            _stop()

    def _ping_once(tn):
        try:
            requests.post(
                f"{CLICKCLACK_URL}/api/realtime/ephemeral",
                headers={"Authorization": f"Bearer {BOT_TOKEN}"},
                json={"workspace_id": WORKSPACE_ID, "channel_id": CHANNEL_ID,
                      "type": "typing.started", "payload": {"tool_name": tn}},
                timeout=3,
            )
        except Exception:
            pass

    def _stop():
        try:
            requests.post(
                f"{CLICKCLACK_URL}/api/realtime/ephemeral",
                headers={"Authorization": f"Bearer {BOT_TOKEN}"},
                json={"workspace_id": WORKSPACE_ID, "channel_id": CHANNEL_ID,
                      "type": "typing.stopped", "payload": {}},
                timeout=3,
            )
        except Exception:
            pass

    return _ctx()


# Usage:
with signal_tool_activity("notion_query"):
    results = notion.databases.query(database_id=DB_ID)
```

## Files Changed

| File | Change |
|------|--------|
| `apps/web/src/components/messages/TypingIndicator.svelte` | Added `toolName?: string` to `TypingEntry` type; added `toolLabel()` mapping function; updated `activityOf()` to render tool-specific label when present |
| `apps/web/src/ChatApp.svelte` | `handleTypingEvent()` now reads `tool_name` from event payload and passes it into the `TypingEntry` |

Backend (`mutations.go`) required **no changes** — the payload map is passed through as-is to all WebSocket subscribers.
