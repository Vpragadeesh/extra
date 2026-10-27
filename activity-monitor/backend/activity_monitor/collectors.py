import asyncio
import getpass
import json
import os
import shutil
import subprocess
import time
from datetime import datetime, timezone, timedelta
from urllib.parse import urlparse

import psutil

from .db import (
    connect,
    paused,
    normalize_ts,
    get_retention_days,
    get_metrics_interval,
    _should_exclude_domain,
    _should_exclude_app,
    _should_strip_query_fragments,
    _should_exclude_titles,
    _should_store_cwd,
    _domain_storage_mode,
    _strip_query_fragment,
)

# Last-known collector health, exposed by GET /api/health.
# Each entry: {"ok": bool, "last_run": ISO-8601 UTC | None, "last_error": str | None}
COLLECTOR_STATUS = {
    "hyprland": {"ok": False, "last_run": None, "last_error": None},
    "metrics": {"ok": False, "last_run": None, "last_error": None},
    "retention": {"ok": False, "last_run": None, "last_error": None},
}


def _mark_collector(name, ok, error=None):
    status = COLLECTOR_STATUS.setdefault(
        name, {"ok": False, "last_run": None, "last_error": None}
    )
    status["ok"] = ok
    status["last_run"] = datetime.now(timezone.utc).isoformat(timespec="seconds")
    status["last_error"] = None if ok else str(error)[:300] if error else None


def now_iso():
    """Current time in UTC, the canonical format for stored timestamps."""
    return datetime.now(timezone.utc).isoformat(timespec="seconds")


def insert_event(source, event_type, **fields):
    """Persist an event unless paused or excluded.

    Returns True when a row was written, False when the event was dropped by
    the pause state or a privacy rule.
    """
    if paused():
        return False

    domain = fields.get("domain")
    if domain and _should_exclude_domain(domain):
        return False

    app = fields.get("app")
    if app and _should_exclude_app(app):
        return False

    url = fields.get("url")
    if url:
        if _should_strip_query_fragments():
            url = _strip_query_fragment(url)
        if _domain_storage_mode() == "domain_only":
            url = None

    title = None if _should_exclude_titles() else fields.get("title")
    cwd = fields.get("cwd") if _should_store_cwd() else None

    with connect() as conn:
        conn.execute(
            """INSERT INTO events
            (ts, source, event_type, app, title, url, domain, cwd, command, duration_seconds, details_json)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
            (
                normalize_ts(fields.get("ts")),
                source,
                event_type,
                app,
                title,
                url,
                domain,
                cwd,
                fields.get("command"),
                fields.get("duration_seconds"),
                fields.get("details_json"),
            ),
        )
    return True


def _strip_query_fragment(url):
    """Strip query strings and fragments from URL."""
    if not url:
        return url
    parsed = urlparse(url)
    from urllib.parse import urlunparse

    cleaned = urlunparse((parsed.scheme, parsed.netloc, parsed.path, parsed.params, "", ""))
    return cleaned


def get_active_window():
    """Return the active Hyprland window, or None when no window is focused.

    Raises RuntimeError when Hyprland is unavailable so the collector health
    status reflects reality instead of a false positive.
    """
    if not shutil.which("hyprctl"):
        raise RuntimeError("hyprctl not found")
    try:
        proc = subprocess.run(
            ["hyprctl", "activewindow", "-j"],
            capture_output=True,
            text=True,
            timeout=1.5,
            check=False,
        )
    except (subprocess.SubprocessError, OSError) as exc:
        raise RuntimeError(f"hyprctl failed: {exc}") from exc
    if proc.returncode != 0:
        detail = (proc.stderr or proc.stdout or "").strip().splitlines()
        raise RuntimeError(f"hyprctl failed: {detail[0][:200] if detail else proc.returncode}")
    try:
        data = json.loads(proc.stdout)
    except json.JSONDecodeError as exc:
        raise RuntimeError("hyprctl returned invalid JSON") from exc
    if not data or not data.get("address"):
        return None
    workspace = data.get("workspace")
    return {
        "app": data.get("class") or data.get("initialClass") or "unknown",
        "title": data.get("title") or "",
        "address": data.get("address"),
        "details_json": json.dumps(
            {
                "address": data.get("address"),
                "workspace": workspace.get("name") if isinstance(workspace, dict) else None,
            }
        ),
    }


def _window_key(window):
    """Stable identity for an active window, including its address when present."""
    if not window:
        return (None, None, None)
    return (window.get("app"), window.get("title"), window.get("address"))


def _current_session_id():
    """Best-effort lookup of the current logind session ID."""
    session_id = os.environ.get("XDG_SESSION_ID")
    if session_id:
        return session_id

    if not shutil.which("loginctl"):
        return None
    try:
        user = os.environ.get("USER") or getpass.getuser()
        proc = subprocess.run(
            ["loginctl", "show-user", user, "-p", "Display"],
            capture_output=True,
            text=True,
            timeout=1.5,
            check=False,
        )
    except (subprocess.SubprocessError, OSError):
        return None
    if proc.returncode != 0:
        return None
    for line in proc.stdout.splitlines():
        key, sep, value = line.partition("=")
        if sep and key == "Display" and value.strip():
            return value.strip()
    return None


def get_session_state():
    """Return best-effort logind idle/lock state for the graphical session.

    Unknown or unsupported sessions are treated as active so a missing
    logind/DBus setup never silently disables foreground tracking.
    """
    unknown = {"known": False, "active": True, "idle": False, "locked": False}
    if not shutil.which("loginctl"):
        return unknown

    session_id = _current_session_id()
    if not session_id:
        return unknown

    try:
        proc = subprocess.run(
            [
                "loginctl",
                "show-session",
                session_id,
                "-p",
                "Active",
                "-p",
                "IdleHint",
                "-p",
                "LockedHint",
            ],
            capture_output=True,
            text=True,
            timeout=1.5,
            check=False,
        )
    except (subprocess.SubprocessError, OSError):
        return unknown
    if proc.returncode != 0:
        return unknown

    values = {}
    for line in proc.stdout.splitlines():
        key, sep, value = line.partition("=")
        if sep:
            values[key.strip()] = value.strip().lower()

    if "active" not in {key.lower() for key in values}:
        return unknown

    def enabled(name):
        return values.get(name, "no") == "yes"

    return {
        "known": True,
        "active": enabled("Active"),
        "idle": enabled("IdleHint"),
        "locked": enabled("LockedHint"),
    }


def session_is_unavailable():
    """True only when logind reports an inactive, idle, or locked session."""
    state = get_session_state()
    return bool(
        state["known"]
        and (not state["active"] or state["idle"] or state["locked"])
    )


async def window_collector():
    previous_key = None
    previous_start_mono = None
    previous_start_wall = None
    previous = None

    while True:
        try:
            if paused() or session_is_unavailable():
                current = None
            else:
                current = get_active_window()
                _mark_collector("hyprland", True)
        except Exception as exc:
            _mark_collector("hyprland", False, exc)
            current = None

        try:
            key = _window_key(current)

            if current and key != previous_key:
                now_mono = time.monotonic()
                now_wall = datetime.now(timezone.utc)
                if previous and previous_start_mono is not None:
                    insert_event(
                        "hyprland",
                        "window_session",
                        **previous,
                        duration_seconds=max(0.0, now_mono - previous_start_mono),
                        ts=previous_start_wall.isoformat(timespec="seconds"),
                    )
                insert_event("hyprland", "window_focus", **current)
                previous_key = key
                previous_start_mono = now_mono
                previous_start_wall = now_wall
                previous = current
            elif not current and previous and previous_start_mono is not None:
                insert_event(
                    "hyprland",
                    "window_session",
                    **previous,
                    duration_seconds=max(0.0, time.monotonic() - previous_start_mono),
                    ts=previous_start_wall.isoformat(timespec="seconds"),
                )
                previous_key = previous_start_mono = previous_start_wall = previous = None
        except Exception as exc:  # keep the collector alive and surface status
            _mark_collector("hyprland", False, exc)

        await asyncio.sleep(2)


def gpu_metrics():
    if not shutil.which("nvidia-smi"):
        return None, None, None
    try:
        raw = subprocess.run(
            [
                "nvidia-smi",
                "--query-gpu=utilization.gpu,memory.used,temperature.gpu",
                "--format=csv,noheader,nounits",
            ],
            capture_output=True,
            text=True,
            timeout=1.5,
            check=True,
        ).stdout.strip().splitlines()[0]
        parts = [p.strip() for p in raw.split(",")]
        return tuple(
            float(p) if p not in ("N/A", "[N/A]") else None for p in parts[:3]
        )
    except (subprocess.SubprocessError, OSError, ValueError, IndexError):
        return None, None, None


def _network_delta(current, previous):
    """Return per-interval byte deltas, tolerating counter resets."""
    if previous is None:
        return 0, 0
    sent = current.bytes_sent - previous.bytes_sent
    recv = current.bytes_recv - previous.bytes_recv
    if sent < 0:
        sent = current.bytes_sent
    if recv < 0:
        recv = current.bytes_recv
    return max(0, sent), max(0, recv)


async def metrics_collector():
    psutil.cpu_percent(interval=None)
    last_net = None

    while True:
        try:
            interval = get_metrics_interval()
        except Exception:
            interval = 5
        await asyncio.sleep(interval)
        try:
            if paused():
                # Drop the baseline while paused so the next sample is not
                # inflated by traffic that happened during the pause.
                last_net = None
                continue

            cpu = psutil.cpu_percent(interval=None)
            mem = psutil.virtual_memory()
            net = psutil.net_io_counters()
            sent, recv = _network_delta(net, last_net)
            last_net = net
            gpu, gpu_mem, gpu_temp = gpu_metrics()

            with connect() as conn:
                conn.execute(
                    """INSERT INTO metrics
                    (ts, cpu_percent, memory_percent, memory_used_bytes, net_sent_bytes, net_recv_bytes,
                     gpu_percent, gpu_memory_used_mb, gpu_temperature_c)
                    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)""",
                    (
                        now_iso(),
                        cpu,
                        mem.percent,
                        mem.used,
                        sent,
                        recv,
                        gpu,
                        gpu_mem,
                        gpu_temp,
                    ),
                )
            _mark_collector("metrics", True)
        except Exception as exc:
            _mark_collector("metrics", False, exc)


def cleanup_old_data():
    """Delete events and metrics older than the retention period."""
    retention_days = get_retention_days()
    if retention_days <= 0:
        return
    cutoff = (datetime.now(timezone.utc) - timedelta(days=retention_days)).isoformat(
        timespec="seconds"
    )
    with connect() as conn:
        conn.execute("DELETE FROM events WHERE ts < ?", (cutoff,))
        conn.execute("DELETE FROM metrics WHERE ts < ?", (cutoff,))


async def retention_cleanup():
    """Clean up old data at startup and then once per day."""
    while True:
        try:
            cleanup_old_data()
            _mark_collector("retention", True)
        except Exception as exc:
            _mark_collector("retention", False, exc)
        await asyncio.sleep(24 * 60 * 60)
