from datetime import date, datetime, timedelta, timezone

from activity_monitor.db import connect


def test_health_and_token(client):
    health = client.get("/api/health")
    assert health.status_code == 200
    body = health.json()
    assert body["ok"] is True
    assert body["paused"] is False
    assert set(body["collectors"]) == {"hyprland", "metrics", "retention"}

    token_response = client.get("/api/token")
    assert token_response.status_code == 200
    assert token_response.json()["token"]


def test_origin_and_host_protection(client):
    assert client.get("/api/token", headers={"Origin": "https://evil.example"}).status_code == 403
    assert client.get("/api/health", headers={"Host": "evil.example"}).status_code == 400
    assert client.get("/api/token", headers={"Origin": "http://localhost:5173"}).status_code == 200
    assert client.get("/api/token", headers={"Origin": "moz-extension://abc"}).status_code == 200


def test_extension_cors_preflight_is_granted(client):
    """Regression: the extension's JSON POST is CORS-enforced by modern Firefox;
    its preflight must be answered for the per-install moz-extension origin."""
    preflight = client.options(
        "/api/events",
        headers={
            "Origin": "moz-extension://6310faf0-2ab5-4fa0-9d5a-c2a7bb14d8e2",
            "Access-Control-Request-Method": "POST",
            "Access-Control-Request-Headers": "content-type,x-api-token",
        },
    )
    assert preflight.status_code == 200
    assert preflight.headers["access-control-allow-origin"].startswith("moz-extension://")
    assert "POST" in preflight.headers["access-control-allow-methods"]

    rejection = client.options(
        "/api/events",
        headers={
            "Origin": "https://evil.example",
            "Access-Control-Request-Method": "POST",
        },
    )
    # The custom origin middleware (outermost) rejects untrusted origins
    # before CORSMiddleware would see the preflight.
    assert rejection.status_code in (400, 403)
    assert "access-control-allow-origin" not in rejection.headers


def test_writes_require_token(client):
    payload = {"source": "fish", "event_type": "command", "command": "ls"}
    assert client.post("/api/events", json=payload).status_code == 401
    assert client.post("/api/control", json={"paused": True}).status_code == 401
    assert client.post("/api/control/delete-all").status_code == 401
    assert client.post("/api/settings", json={"retention_days": 7}).status_code == 401


def test_non_domain_events_are_persisted(client, auth):
    """Regression: Hyprland/fish events have no domain and must not be dropped."""
    assert client.post("/api/events", headers=auth, json={
        "source": "hyprland", "event_type": "window_focus", "app": "foot", "title": "~"
    }).json()["accepted"] is True
    assert client.post("/api/events", headers=auth, json={
        "source": "fish", "event_type": "command", "command": "ls", "cwd": "/tmp"
    }).json()["accepted"] is True

    events = client.get("/api/events").json()
    assert len(events) == 2
    sources = {event["source"] for event in events}
    assert sources == {"hyprland", "fish"}
    fish = next(event for event in events if event["source"] == "fish")
    assert fish["command"] == "ls"
    assert fish["cwd"] == "/tmp"


def test_url_normalization_and_query_stripping(client, auth):
    response = client.post("/api/events", headers=auth, json={
        "source": "firefox", "event_type": "tab_focus", "app": "Firefox",
        "url": "https://Example.com/path?q=secret#fragment"
    })
    assert response.json()["accepted"] is True
    event = client.get("/api/events").json()[0]
    assert event["domain"] == "example.com"
    assert event["url"] == "https://Example.com/path"


def test_unsupported_url_scheme_rejected(client, auth):
    response = client.post("/api/events", headers=auth, json={
        "source": "firefox", "event_type": "tab_focus", "url": "javascript:alert(1)"
    })
    assert response.status_code == 422


def test_timestamp_is_normalized_to_utc(client, auth):
    client.post("/api/events", headers=auth, json={
        "source": "fish", "event_type": "command", "command": "pwd",
        "ts": "2026-10-26T10:00:00+05:30"
    })
    event = client.get("/api/events").json()[0]
    assert event["ts"] == "2026-10-26T04:30:00+00:00"


def test_pause_blocks_ingestion(client, auth):
    assert client.post("/api/control", headers=auth, json={"paused": True}).json()["paused"] is True
    response = client.post("/api/events", headers=auth, json={
        "source": "fish", "event_type": "command", "command": "whoami"
    })
    assert response.status_code == 200
    assert response.json()["accepted"] is False
    assert client.get("/api/events").json() == []
    assert client.get("/api/summary").json()["event_count"] == 0

    client.post("/api/control", headers=auth, json={"paused": False})
    assert client.post("/api/events", headers=auth, json={
        "source": "fish", "event_type": "command", "command": "whoami"
    }).json()["accepted"] is True


def test_domain_and_app_exclusions(client, auth):
    updated = client.post("/api/settings", headers=auth, json={
        "excluded_domains": ["example.com"],
        "excluded_apps": ["KeePassXC"],
    })
    assert updated.status_code == 200
    assert updated.json()["excluded_domains"] == ["example.com"]

    assert client.post("/api/events", headers=auth, json={
        "source": "firefox", "event_type": "tab_focus", "url": "https://sub.example.com/private"
    }).json()["accepted"] is False
    assert client.post("/api/events", headers=auth, json={
        "source": "hyprland", "event_type": "window_focus", "app": "keepassxc"
    }).json()["accepted"] is False
    assert client.get("/api/events").json() == []


def test_domain_only_and_title_omission(client, auth):
    client.post("/api/settings", headers=auth, json={
        "domain_storage_mode": "domain_only", "exclude_titles": True
    })
    accepted = client.post("/api/events", headers=auth, json={
        "source": "firefox", "event_type": "tab_focus", "app": "Firefox",
        "title": "Sensitive page", "url": "https://example.org/secret?x=1"
    }).json()["accepted"]
    assert accepted is True
    event = client.get("/api/events").json()[0]
    assert event["url"] is None
    assert event["domain"] == "example.org"
    assert event["title"] is None


def test_summary_uses_local_day_boundaries(client, auth):
    today = date.today()
    today_ts = datetime.combine(today, datetime.min.time()).astimezone().replace(hour=12)
    yesterday_ts = today_ts - timedelta(days=1)
    client.post("/api/events", headers=auth, json={
        "source": "hyprland", "event_type": "window_session", "app": "foot",
        "duration_seconds": 60, "ts": today_ts.isoformat()
    })
    client.post("/api/events", headers=auth, json={
        "source": "hyprland", "event_type": "window_session", "app": "old",
        "duration_seconds": 60, "ts": yesterday_ts.isoformat()
    })
    summary = client.get("/api/summary").json()
    assert summary["event_count"] == 1
    assert summary["tracked_seconds"] == 60.0
    assert summary["site_count"] == 0
    assert summary["apps"] == [{"app": "foot", "seconds": 60.0}]


def test_summary_site_count_and_session_totals(client, auth):
    today_ts = datetime.combine(date.today(), datetime.min.time()).astimezone().replace(hour=12)
    for domain in ("one.example", "two.example"):
        client.post("/api/events", headers=auth, json={
            "source": "firefox", "event_type": "tab_focus",
            "url": f"https://{domain}/a",
            "ts": today_ts.isoformat(),
        })
        client.post("/api/events", headers=auth, json={
            "source": "firefox", "event_type": "tab_session",
            "url": f"https://{domain}/a", "duration_seconds": 30,
            "ts": today_ts.isoformat(),
        })
    summary = client.get("/api/summary").json()
    assert summary["site_count"] == 2
    assert summary["sites"][0]["visits"] == 1
    assert sum(site["seconds"] for site in summary["sites"]) == 60


def test_summary_visit_counts_come_from_focus_not_session_samples(client, auth):
    """Regression: website visits must count tab_focus events, not 15s tab_session samples."""
    today_ts = datetime.combine(date.today(), datetime.min.time()).astimezone().replace(hour=12)
    for _ in range(3):
        client.post("/api/events", headers=auth, json={
            "source": "firefox", "event_type": "tab_session",
            "url": "https://example.com/a", "duration_seconds": 10,
            "ts": today_ts.isoformat(),
        })
    client.post("/api/events", headers=auth, json={
        "source": "firefox", "event_type": "tab_focus",
        "url": "https://example.com/",
        "ts": today_ts.isoformat(),
    })
    summary = client.get("/api/summary").json()
    site = summary["sites"][0]
    assert site["domain"] == "example.com"
    assert site["visits"] == 1
    assert site["seconds"] == 30.0


def test_events_and_metrics_date_filters(client, auth):
    """Regression: ?date=YYYY-MM-DD selects one local calendar day for both endpoints."""
    from activity_monitor.main import _day_bounds

    today = date.today()
    noon = datetime.combine(today, datetime.min.time()).astimezone().replace(hour=12)
    yesterday_noon = noon - timedelta(days=1)

    client.post("/api/events", headers=auth, json={
        "source": "fish", "event_type": "command", "command": "today", "ts": noon.isoformat()
    })
    client.post("/api/events", headers=auth, json={
        "source": "fish", "event_type": "command", "command": "old", "ts": yesterday_noon.isoformat()
    })

    events_today = client.get(f"/api/events?date={today}").json()
    assert [event["command"] for event in events_today] == ["today"]
    assert client.get(f"/api/events?date={today - timedelta(days=1)}").json()[0]["command"] == "old"

    with connect() as conn:
        conn.execute(
            "INSERT INTO metrics(ts, cpu_percent) VALUES (?, ?)",
            (noon.astimezone(timezone.utc).isoformat(timespec="seconds"), 1.0),
        )
        conn.execute(
            "INSERT INTO metrics(ts, cpu_percent) VALUES (?, ?)",
            (yesterday_noon.astimezone(timezone.utc).isoformat(timespec="seconds"), 2.0),
        )
    metrics_today = client.get(f"/api/metrics?date={today}").json()
    assert [m["cpu_percent"] for m in metrics_today] == [1.0]
    metrics_yesterday = client.get(f"/api/metrics?date={today - timedelta(days=1)}").json()
    assert [m["cpu_percent"] for m in metrics_yesterday] == [2.0]

    # Sanity: bounds helper covers local noon in UTC terms.
    start, end = _day_bounds(today)
    assert start < noon.astimezone(timezone.utc).isoformat(timespec="seconds") < end


def test_event_and_metric_pagination(client, auth):
    for i in range(5):
        client.post("/api/events", headers=auth, json={
            "source": "fish", "event_type": "command", "command": f"cmd{i}"
        })
    first = client.get("/api/events?limit=2").json()
    assert len(first) == 2
    second = client.get(f"/api/events?limit=2&before_id={first[-1]['id']}").json()
    assert len(second) == 2
    assert {row["id"] for row in first}.isdisjoint({row["id"] for row in second})

    with connect() as conn:
        base = datetime.now(timezone.utc)
        for i in range(5):
            conn.execute(
                "INSERT INTO metrics(ts, cpu_percent) VALUES (?, ?)",
                ((base + timedelta(seconds=i)).isoformat(timespec="seconds"), float(i)),
            )
    metrics_first = client.get("/api/metrics?limit=2").json()
    assert len(metrics_first) == 2
    metrics_second = client.get(f"/api/metrics?limit=2&before_id={metrics_first[0]['id']}").json()
    assert len(metrics_second) == 2
    assert {row["id"] for row in metrics_first}.isdisjoint({row["id"] for row in metrics_second})


def test_body_size_limit(client, auth):
    response = client.post(
        "/api/events",
        headers={**auth, "Content-Type": "application/json"},
        content=b"x" * (1024 * 1024 + 1),
    )
    assert response.status_code == 413


def test_body_without_content_length_is_rejected():
    """Regression: a chunked body with no Content-Length must not bypass the size cap."""
    import asyncio

    from starlette.requests import Request
    from starlette.responses import PlainTextResponse

    from activity_monitor.main import limit_request_size

    async def ok(_):
        return PlainTextResponse("ok")

    def request_for(method, headers):
        return Request({
            "type": "http", "asgi": {"version": "3.0", "spec_version": "2.3"},
            "method": method, "path": "/api/events", "raw_path": b"/api/events",
            "query_string": b"", "root_path": "", "scheme": "http",
            "server": ("127.0.0.1", 8765), "client": ("127.0.0.1", 111),
            "headers": headers,
        })

    chunked = asyncio.run(limit_request_size(
        request_for("POST", [(b"transfer-encoding", b"chunked")]), ok
    ))
    assert chunked.status_code == 411

    length_required = asyncio.run(limit_request_size(
        request_for("PUT", []), ok
    ))
    assert length_required.status_code == 411

    get_passthrough = asyncio.run(limit_request_size(
        request_for("GET", []), ok
    ))
    assert get_passthrough.status_code == 200


def test_settings_validation(client, auth):
    assert client.post("/api/settings", headers=auth, json={"retention_days": 0}).status_code == 422
    assert client.post("/api/settings", headers=auth, json={"domain_storage_mode": "nope"}).status_code == 422
    assert client.post("/api/settings", headers=auth, json={"excluded_domains": ["https://example.com/path"]}).status_code == 422
    assert client.post("/api/settings", headers=auth, json={"metrics_interval_seconds": 1}).status_code == 422
    assert client.post("/api/settings", headers=auth, json={"metrics_interval_seconds": 301}).status_code == 422

    updated = client.post("/api/settings", headers=auth, json={
        "retention_days": 14, "strip_query_fragments": False,
        "domain_storage_mode": "domain_only", "exclude_titles": True,
        "metrics_interval_seconds": 12,
    }).json()
    assert updated["retention_days"] == 14
    assert updated["strip_query_fragments"] is False
    assert updated["metrics_interval_seconds"] == 12


def test_store_cwd_opt_out(client, auth):
    defaults = client.get("/api/settings").json()
    assert defaults["store_cwd"] is True
    assert defaults["metrics_interval_seconds"] == 5

    updated = client.post("/api/settings", headers=auth, json={"store_cwd": False}).json()
    assert updated["store_cwd"] is False

    client.post("/api/events", headers=auth, json={
        "source": "fish", "event_type": "command", "command": "cd",
        "cwd": "/home/user/private",
    })
    event = client.get("/api/events").json()[0]
    assert event["cwd"] is None

    client.post("/api/settings", headers=auth, json={"store_cwd": True})
    client.post("/api/events", headers=auth, json={
        "source": "fish", "event_type": "command", "command": "ls", "cwd": "/tmp",
    })
    events = client.get("/api/events").json()
    assert next(e for e in events if e["command"] == "ls")["cwd"] == "/tmp"


def test_delete_all_data(client, auth):
    client.post("/api/events", headers=auth, json={
        "source": "fish", "event_type": "command", "command": "ls"
    })
    with connect() as conn:
        conn.execute(
            "INSERT INTO metrics(ts, cpu_percent) VALUES (?, ?)",
            (datetime.now(timezone.utc).isoformat(timespec="seconds"), 10.0),
        )
    result = client.post("/api/control/delete-all", headers=auth).json()
    assert result["deleted"] == {"events": 1, "metrics": 1}
    assert client.get("/api/events").json() == []
    assert client.get("/api/metrics").json() == []


def test_retention_cleanup_removes_old_rows(client, auth):
    from activity_monitor.collectors import cleanup_old_data

    old_ts = (datetime.now(timezone.utc) - timedelta(days=45)).isoformat(timespec="seconds")
    new_ts = datetime.now(timezone.utc).isoformat(timespec="seconds")
    with connect() as conn:
        conn.execute(
            "INSERT INTO events(ts, source, event_type) VALUES (?, 'fish', 'command')",
            (old_ts,),
        )
        conn.execute(
            "INSERT INTO events(ts, source, event_type) VALUES (?, 'fish', 'command')",
            (new_ts,),
        )
        conn.execute(
            "INSERT INTO metrics(ts, cpu_percent) VALUES (?, 5.0)", (old_ts,)
        )
        conn.execute(
            "INSERT INTO metrics(ts, cpu_percent) VALUES (?, 5.0)", (new_ts,)
        )

    cleanup_old_data()

    with connect() as conn:
        event_ts = [row["ts"] for row in conn.execute("SELECT ts FROM events")]
        metric_ts = [row["ts"] for row in conn.execute("SELECT ts FROM metrics")]
    assert event_ts == [new_ts]
    assert metric_ts == [new_ts]


def test_network_delta_handles_counter_reset():
    from types import SimpleNamespace

    from activity_monitor.collectors import _network_delta

    previous = SimpleNamespace(bytes_sent=1000, bytes_recv=2000)
    current = SimpleNamespace(bytes_sent=1500, bytes_recv=2600)
    assert _network_delta(current, previous) == (500, 600)

    reset = SimpleNamespace(bytes_sent=10, bytes_recv=20)
    assert _network_delta(reset, previous) == (10, 20)


def test_get_active_window_parses_and_reports_failures(monkeypatch):
    import pytest

    import activity_monitor.collectors as collectors

    monkeypatch.setattr(collectors.shutil, "which", lambda _: "/usr/bin/hyprctl")

    class Ok:
        returncode = 0
        stderr = ""
        stdout = '{"address":"0xabc","class":"foot","title":"~","workspace":{"name":"1"}}'

    monkeypatch.setattr(collectors.subprocess, "run", lambda *a, **k: Ok())
    window = collectors.get_active_window()
    assert window["app"] == "foot"
    assert window["title"] == "~"
    assert window["address"] == "0xabc"
    assert '"address": "0xabc"' in window["details_json"]

    class NoWindow:
        returncode = 0
        stderr = ""
        stdout = "{}"

    monkeypatch.setattr(collectors.subprocess, "run", lambda *a, **k: NoWindow())
    assert collectors.get_active_window() is None

    class Failed:
        returncode = 1
        stderr = "HYPRLAND_INSTANCE_SIGNATURE not set! (is hyprland running?)"
        stdout = ""

    monkeypatch.setattr(collectors.subprocess, "run", lambda *a, **k: Failed())
    with pytest.raises(RuntimeError):
        collectors.get_active_window()


def test_window_key_distinguishes_same_metadata_by_address():
    """Regression: two windows with identical app+title must not be merged."""
    import activity_monitor.collectors as collectors

    first = {"app": "foot", "title": "~", "address": "0xaaa"}
    second = {"app": "foot", "title": "~", "address": "0xbbb"}
    assert collectors._window_key(first) != collectors._window_key(second)
    assert collectors._window_key(first) == ("foot", "~", "0xaaa")
    # Without an address (or empty input) the key stays stable.
    assert collectors._window_key({"app": "foot", "title": "~"}) == ("foot", "~", None)
    assert collectors._window_key(None) == (None, None, None)
    assert collectors._window_key(first) == collectors._window_key(dict(first))


def test_session_state_parsing(monkeypatch):
    import activity_monitor.collectors as collectors

    class Ok:
        returncode = 0
        stderr = ""

        def __init__(self, stdout):
            self.stdout = stdout

    monkeypatch.setattr(collectors.shutil, "which", lambda _: "/usr/bin/loginctl")
    monkeypatch.setenv("XDG_SESSION_ID", "2")

    monkeypatch.setattr(
        collectors.subprocess,
        "run",
        lambda *a, **k: Ok("Active=yes\nIdleHint=yes\nLockedHint=no\n"),
    )
    assert collectors.get_session_state() == {
        "known": True, "active": True, "idle": True, "locked": False,
    }
    assert collectors.session_is_unavailable() is True

    monkeypatch.setattr(
        collectors.subprocess,
        "run",
        lambda *a, **k: Ok("Active=yes\nIdleHint=no\nLockedHint=yes\n"),
    )
    assert collectors.get_session_state()["locked"] is True
    assert collectors.session_is_unavailable() is True

    monkeypatch.setattr(
        collectors.subprocess,
        "run",
        lambda *a, **k: Ok("Active=no\nIdleHint=no\nLockedHint=no\n"),
    )
    assert collectors.get_session_state()["active"] is False
    assert collectors.session_is_unavailable() is True

    monkeypatch.setattr(
        collectors.subprocess,
        "run",
        lambda *a, **k: Ok("Active=yes\nIdleHint=no\nLockedHint=no\n"),
    )
    assert collectors.session_is_unavailable() is False


def test_session_state_falls_back_to_loginctl_display(monkeypatch):
    import activity_monitor.collectors as collectors

    calls = []

    class Ok:
        returncode = 0
        stderr = ""

        def __init__(self, stdout):
            self.stdout = stdout

    def fake_run(args, **kwargs):
        calls.append(args)
        if args[1] == "show-user":
            return Ok("Display=7\n")
        return Ok("Active=yes\nIdleHint=no\nLockedHint=no\n")

    monkeypatch.delenv("XDG_SESSION_ID", raising=False)
    monkeypatch.setattr(collectors.shutil, "which", lambda _: "/usr/bin/loginctl")
    monkeypatch.setattr(collectors.subprocess, "run", fake_run)

    state = collectors.get_session_state()
    assert state == {"known": True, "active": True, "idle": False, "locked": False}
    assert calls[0][1] == "show-user"
    assert calls[1][1] == "show-session"
    assert calls[1][2] == "7"


def test_session_state_unknown_is_treated_active(monkeypatch):
    import activity_monitor.collectors as collectors

    monkeypatch.setattr(collectors.shutil, "which", lambda _: None)
    assert collectors.get_session_state() == {
        "known": False, "active": True, "idle": False, "locked": False,
    }
    assert collectors.session_is_unavailable() is False


def test_metrics_collector_uses_configured_interval(client, monkeypatch):
    import asyncio

    import pytest

    import activity_monitor.collectors as collectors
    from activity_monitor.db import update_settings

    update_settings(metrics_interval_seconds=12)
    sleeps = []

    class StopLoop(Exception):
        pass

    async def fake_sleep(seconds):
        sleeps.append(seconds)
        raise StopLoop()

    monkeypatch.setattr(collectors.asyncio, "sleep", fake_sleep)
    with pytest.raises(StopLoop):
        asyncio.run(collectors.metrics_collector())
    assert sleeps == [12]


def test_window_collector_stops_counting_when_session_unavailable(client, monkeypatch):
    import asyncio

    import pytest

    import activity_monitor.collectors as collectors

    window = {"app": "foot", "title": "~", "details_json": "{}"}
    active_calls = []
    states = iter([False, True])

    class StopLoop(Exception):
        pass

    sleeps = []

    async def fake_sleep(seconds):
        sleeps.append(seconds)
        if len(sleeps) >= 2:
            raise StopLoop()

    monkeypatch.setattr(collectors, "paused", lambda: False)
    monkeypatch.setattr(collectors, "get_active_window", lambda: active_calls.append(True) or window)
    monkeypatch.setattr(collectors, "session_is_unavailable", lambda: next(states))
    monkeypatch.setattr(collectors.asyncio, "sleep", fake_sleep)

    with pytest.raises(StopLoop):
        asyncio.run(collectors.window_collector())

    assert active_calls == [True]
    with connect() as conn:
        rows = conn.execute(
            "SELECT event_type, app, duration_seconds FROM events ORDER BY id"
        ).fetchall()
    assert [row["event_type"] for row in rows] == ["window_focus", "window_session"]
    assert rows[1]["app"] == "foot"
    assert rows[1]["duration_seconds"] >= 0
