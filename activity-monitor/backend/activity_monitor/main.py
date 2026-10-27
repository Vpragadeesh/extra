import asyncio
import json
from contextlib import asynccontextmanager
from datetime import date, datetime, timezone, timedelta
from typing import Literal
from urllib.parse import urlparse

from fastapi import FastAPI, HTTPException, Query, Depends, Request
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from fastapi.security import APIKeyHeader
from pydantic import AliasChoices, BaseModel, Field, field_validator

from .collectors import (
    COLLECTOR_STATUS,
    insert_event,
    metrics_collector,
    window_collector,
    retention_cleanup,
)
from .db import (
    connect,
    init_db,
    paused,
    get_api_token,
    get_settings,
    update_settings,
)

VERSION = "0.1.0"
API_HOST = "127.0.0.1"
API_PORT = 8765

# Browser origins explicitly allowed to call the API. The Firefox extension
# origin (moz-extension://<uuid>) is allowed separately because its UUID is
# generated per install.
ALLOWED_ORIGINS = {"http://localhost:5173", "http://127.0.0.1:5173"}
ALLOWED_HOSTS = {
    f"{API_HOST}:{API_PORT}",
    f"localhost:{API_PORT}",
    f"[::1]:{API_PORT}",
    "testserver",  # FastAPI's TestClient
}


@asynccontextmanager
async def lifespan(app):
    init_db()
    tasks = [
        asyncio.create_task(window_collector()),
        asyncio.create_task(metrics_collector()),
        asyncio.create_task(retention_cleanup()),
    ]
    app.state.tasks = tasks
    try:
        yield
    finally:
        for task in tasks:
            task.cancel()
        await asyncio.gather(*tasks, return_exceptions=True)


app = FastAPI(title="Local Activity Monitor API", version=VERSION, lifespan=lifespan)

app.add_middleware(
    CORSMiddleware,
    allow_origins=list(ALLOWED_ORIGINS),
    # The extension's origin carries a per-install/moz-specific UUID that the
    # dashboard allowlist cannot name, so grant it by prefix regex. Without
    # this, the extension's JSON POST is rejected at its CORS preflight
    # (OPTIONS) and the event never reaches the API.
    # Starlette full-matches this regex, so it needs an explicit tail.
    allow_origin_regex=r"^moz-extension://.*",
    allow_credentials=False,
    allow_methods=["GET", "POST"],
    allow_headers=["*"],
)


def _origin_allowed(origin):
    if not origin:
        return True
    if origin in ALLOWED_ORIGINS:
        return True
    return origin.startswith("moz-extension://")


@app.middleware("http")
async def enforce_loopback_host(request: Request, call_next):
    """Reject DNS-rebinding requests whose Host is not this loopback service."""
    host = (request.headers.get("host") or "").lower()
    if host and host not in ALLOWED_HOSTS:
        return JSONResponse(status_code=400, content={"detail": "Invalid Host header"})
    return await call_next(request)


@app.middleware("http")
async def enforce_local_origin(request: Request, call_next):
    """Reject cross-origin browser requests that are not the dashboard or extension."""
    origin = request.headers.get("origin")
    if origin and not _origin_allowed(origin):
        return JSONResponse(status_code=403, content={"detail": "Origin not allowed"})
    return await call_next(request)


@app.middleware("http")
async def limit_request_size(request: Request, call_next):
    content_length = request.headers.get("content-length")
    if content_length is None:
        # A chunked body has no Content-Length and would bypass the size cap
        # below. All local clients (dashboard fetch, curl -d, the extension,
        # the test client) declare Content-Length, so require it instead of
        # buffering a stream of unknown size.
        if request.method in {"POST", "PUT", "PATCH"}:
            return JSONResponse(status_code=411, content={"detail": "Content-Length required"})
        return await call_next(request)
    try:
        too_large = int(content_length) > 1024 * 1024
    except ValueError:
        return JSONResponse(status_code=400, content={"detail": "Invalid Content-Length"})
    if too_large:
        return JSONResponse(
            status_code=413,
            content={"detail": "Request body too large (max 1MB)"},
        )
    return await call_next(request)


api_key_header = APIKeyHeader(name="X-API-Token", auto_error=False)


def require_api_token(api_token: str = Depends(api_key_header)):
    """Dependency to validate the per-install API token on write endpoints."""
    stored = get_api_token()
    import secrets

    if not api_token or not secrets.compare_digest(api_token, stored):
        raise HTTPException(status_code=401, detail="Invalid or missing API token")


# =============================================
# Pydantic models
# =============================================

ALLOWED_SOURCES = {"hyprland", "firefox", "fish"}
ALLOWED_EVENT_TYPES = {
    "window_focus",
    "window_session",
    "tab_focus",
    "tab_session",
    "command",
    "metrics",
}
ALLOWED_DETAIL_KEYS = {"tab_id", "address", "workspace", "sample"}


class EventIn(BaseModel):
    model_config = {"extra": "ignore"}

    source: str = Field(min_length=1, max_length=20)
    event_type: str = Field(min_length=1, max_length=40)
    ts: str | None = Field(
        default=None,
        validation_alias=AliasChoices("ts", "timestamp"),
        description="ISO-8601 timestamp with timezone; normalized to UTC",
    )
    app: str | None = Field(default=None, max_length=100)
    title: str | None = Field(default=None, max_length=500)
    url: str | None = Field(default=None, max_length=2000)
    cwd: str | None = Field(default=None, max_length=500)
    command: str | None = Field(default=None, max_length=200)
    duration_seconds: float | None = Field(default=None, ge=0, le=86400)
    details: dict | None = None

    @field_validator("source")
    @classmethod
    def validate_source(cls, v):
        if v not in ALLOWED_SOURCES:
            raise ValueError(f"source must be one of: {sorted(ALLOWED_SOURCES)}")
        return v

    @field_validator("event_type")
    @classmethod
    def validate_event_type(cls, v):
        if v not in ALLOWED_EVENT_TYPES:
            raise ValueError(f"event_type must be one of: {sorted(ALLOWED_EVENT_TYPES)}")
        return v

    @field_validator("ts")
    @classmethod
    def validate_ts(cls, v):
        if v is None:
            return v
        try:
            parsed = datetime.fromisoformat(v.replace("Z", "+00:00"))
        except (ValueError, TypeError) as exc:
            raise ValueError("ts must be a valid ISO-8601 timestamp") from exc
        if parsed.tzinfo is None:
            raise ValueError("ts must include a timezone offset")
        return parsed.astimezone(timezone.utc).isoformat(timespec="seconds")

    @field_validator("url")
    @classmethod
    def validate_url(cls, v):
        if v is not None:
            parsed = urlparse(v)
            if parsed.scheme not in ("http", "https") or not parsed.netloc:
                raise ValueError("url must be an absolute http(s) URL")
        return v

    @field_validator("details")
    @classmethod
    def validate_details(cls, v):
        if v is None:
            return v
        cleaned = {}
        for key, value in v.items():
            if key not in ALLOWED_DETAIL_KEYS or isinstance(value, (dict, list)):
                continue
            if isinstance(value, str):
                value = value[:200]
            cleaned[key] = value
        return cleaned or None


class ControlIn(BaseModel):
    paused: bool


class SettingsIn(BaseModel):
    excluded_domains: list[str] | None = None
    excluded_apps: list[str] | None = None
    retention_days: int | None = Field(default=None, ge=1, le=3650)
    strip_query_fragments: bool | None = None
    domain_storage_mode: Literal["full", "domain_only"] | None = None
    exclude_titles: bool | None = None
    store_cwd: bool | None = None
    metrics_interval_seconds: int | None = Field(default=None, ge=2, le=300)

    @field_validator("excluded_domains", "excluded_apps")
    @classmethod
    def clean_list(cls, v):
        if v is None:
            return v
        if len(v) > 200:
            raise ValueError("at most 200 entries are allowed")
        cleaned = []
        for item in v:
            item = str(item).strip().lower().lstrip(".")
            if not item:
                continue
            if len(item) > 253 or "/" in item or ":" in item:
                raise ValueError(f"invalid exclusion entry: {item!r}")
            cleaned.append(item)
        return cleaned


class HealthOut(BaseModel):
    ok: bool
    paused: bool
    events: int
    version: str
    collectors: dict


class EventOut(BaseModel):
    id: int
    ts: str
    source: str
    event_type: str
    app: str | None = None
    title: str | None = None
    url: str | None = None
    domain: str | None = None
    cwd: str | None = None
    command: str | None = None
    duration_seconds: float | None = None
    details_json: str | None = None


class MetricOut(BaseModel):
    id: int
    ts: str
    cpu_percent: float | None = None
    memory_percent: float | None = None
    memory_used_bytes: int | None = None
    net_sent_bytes: int | None = None
    net_recv_bytes: int | None = None
    gpu_percent: float | None = None
    gpu_memory_used_mb: float | None = None
    gpu_temperature_c: float | None = None


class SummaryAppOut(BaseModel):
    app: str | None = None
    seconds: float


class SummarySiteOut(BaseModel):
    domain: str
    seconds: float
    visits: int


class SummaryOut(BaseModel):
    date: str
    event_count: int
    commands: int
    tracked_seconds: float
    site_count: int
    apps: list[SummaryAppOut]
    sites: list[SummarySiteOut]
    latest_metrics: MetricOut | None = None
    paused: bool


class SettingsOut(BaseModel):
    excluded_domains: list[str]
    excluded_apps: list[str]
    retention_days: int
    strip_query_fragments: bool
    domain_storage_mode: str
    exclude_titles: bool
    store_cwd: bool
    metrics_interval_seconds: int


class TokenOut(BaseModel):
    token: str


class AcceptedOut(BaseModel):
    ok: bool
    accepted: bool


class ControlOut(BaseModel):
    ok: bool
    paused: bool


class DeleteOut(BaseModel):
    ok: bool
    deleted: dict


# =============================================
# API endpoints
# =============================================

def _day_bounds(day: date):
    """Return UTC start/end ISO timestamps for a local calendar day."""
    local_midnight = datetime.combine(day, datetime.min.time()).astimezone()
    return (
        local_midnight.astimezone(timezone.utc).isoformat(timespec="seconds"),
        (local_midnight + timedelta(days=1))
        .astimezone(timezone.utc)
        .isoformat(timespec="seconds"),
    )


@app.get("/api/health", response_model=HealthOut)
def health():
    with connect() as conn:
        count = conn.execute("SELECT COUNT(*) AS n FROM events").fetchone()["n"]
    collectors = {name: dict(status) for name, status in COLLECTOR_STATUS.items()}
    return {
        "ok": True,
        "paused": paused(),
        "events": count,
        "version": VERSION,
        "collectors": collectors,
    }


@app.get("/api/token", response_model=TokenOut)
def get_token():
    """Return the local API token for the dashboard/extension bootstrap.

    Browser requests are restricted by the origin middleware above, so an
    arbitrary web page cannot read the token even though the endpoint itself
    has no token requirement.
    """
    return {"token": get_api_token()}


@app.post("/api/events", dependencies=[Depends(require_api_token)], response_model=AcceptedOut)
def create_event(event: EventIn):
    """Ingest one validated event."""
    url = event.url
    domain = urlparse(url).hostname if url else None
    if domain:
        domain = domain.lower()

    details_json = json.dumps(event.details, ensure_ascii=False) if event.details else None

    stored = insert_event(
        event.source,
        event.event_type,
        ts=event.ts,
        app=event.app,
        title=event.title,
        url=url,
        domain=domain,
        cwd=event.cwd,
        command=event.command,
        duration_seconds=event.duration_seconds,
        details_json=details_json,
    )
    return {"ok": True, "accepted": stored}


@app.get("/api/events", response_model=list[EventOut])
def list_events(
    limit: int = Query(default=100, ge=1, le=500),
    before_id: int | None = Query(default=None, ge=1),
    source: str | None = Query(default=None),
    day: date | None = Query(default=None, alias="date"),
):
    """List events newest-first with optional day/source filters and cursor pagination."""
    if source and source not in ALLOWED_SOURCES:
        raise HTTPException(
            status_code=400,
            detail="Invalid source. Must be one of: hyprland, firefox, fish",
        )
    query = "SELECT * FROM events"
    clauses = []
    params = []
    if source:
        clauses.append("source=?")
        params.append(source)
    if day:
        start, end = _day_bounds(day)
        clauses.extend(["ts >= ?", "ts < ?"])
        params.extend([start, end])
    if before_id:
        clauses.append("id < ?")
        params.append(before_id)
    if clauses:
        query += " WHERE " + " AND ".join(clauses)
    query += " ORDER BY ts DESC, id DESC LIMIT ?"
    params.append(limit)

    with connect() as conn:
        rows = conn.execute(query, tuple(params)).fetchall()
    return [dict(row) for row in rows]


@app.get("/api/metrics", response_model=list[MetricOut])
def list_metrics(
    limit: int = Query(default=120, ge=1, le=500),
    before_id: int | None = Query(default=None, ge=1),
    day: date | None = Query(default=None, alias="date"),
):
    """List recent metric samples oldest-first (ready for charting)."""
    clauses = []
    params = []
    if day:
        start, end = _day_bounds(day)
        clauses.extend(["ts >= ?", "ts < ?"])
        params.extend([start, end])
    if before_id:
        clauses.append("id < ?")
        params.append(before_id)
    sql = "SELECT * FROM metrics"
    if clauses:
        sql += " WHERE " + " AND ".join(clauses)
    sql += " ORDER BY ts DESC, id DESC LIMIT ?"
    params.append(limit)
    with connect() as conn:
        rows = conn.execute(sql, tuple(params)).fetchall()
    return [dict(row) for row in reversed(rows)]


@app.get("/api/summary", response_model=SummaryOut)
def summary(day: date = Query(default_factory=date.today, alias="date")):
    """Daily summary using local-day boundaries converted to UTC."""
    start, end = _day_bounds(day)
    with connect() as conn:
        total = conn.execute(
            "SELECT COUNT(*) AS n FROM events WHERE ts >= ? AND ts < ?", (start, end)
        ).fetchone()["n"]
        apps = conn.execute(
            """SELECT app, SUM(COALESCE(duration_seconds,0)) AS seconds
               FROM events
               WHERE source='hyprland' AND event_type='window_session'
                 AND ts >= ? AND ts < ?
               GROUP BY app ORDER BY seconds DESC LIMIT 10""",
            (start, end),
        ).fetchall()
        sites = conn.execute(
            """SELECT domain, SUM(COALESCE(duration_seconds,0)) AS seconds
               FROM events
               WHERE source='firefox' AND event_type='tab_session'
                 AND ts >= ? AND ts < ? AND domain IS NOT NULL
               GROUP BY domain ORDER BY seconds DESC LIMIT 10""",
            (start, end),
        ).fetchall()
        visits = {
            row["domain"]: row["visits"]
            for row in conn.execute(
                """SELECT domain, COUNT(*) AS visits
                   FROM events
                   WHERE source='firefox' AND event_type='tab_focus'
                     AND ts >= ? AND ts < ? AND domain IS NOT NULL
                   GROUP BY domain""",
                (start, end),
            )
        }
        commands = conn.execute(
            "SELECT COUNT(*) AS n FROM events WHERE source='fish' AND ts >= ? AND ts < ?",
            (start, end),
        ).fetchone()["n"]
        tracked_seconds = conn.execute(
            """SELECT COALESCE(SUM(COALESCE(duration_seconds,0)),0) AS s
               FROM events
               WHERE source='hyprland' AND event_type='window_session'
                 AND ts >= ? AND ts < ?""",
            (start, end),
        ).fetchone()["s"]
        site_count = conn.execute(
            """SELECT COUNT(DISTINCT domain) AS n
               FROM events
               WHERE source='firefox' AND event_type='tab_session'
                 AND ts >= ? AND ts < ? AND domain IS NOT NULL""",
            (start, end),
        ).fetchone()["n"]
        latest = conn.execute(
            """SELECT * FROM metrics
               WHERE ts >= ? AND ts < ?
               ORDER BY ts DESC, id DESC LIMIT 1""",
            (start, end),
        ).fetchone()
    return {
        "date": str(day),
        "event_count": total,
        "commands": commands,
        "tracked_seconds": float(tracked_seconds or 0),
        "site_count": int(site_count or 0),
        "apps": [dict(r) for r in apps],
        "sites": [
            {"domain": row["domain"], "seconds": row["seconds"], "visits": int(visits.get(row["domain"], 0))}
            for row in sites
        ],
        "latest_metrics": dict(latest) if latest else None,
        "paused": paused(),
    }


@app.get("/api/settings", response_model=SettingsOut)
def read_settings():
    """Return privacy, retention, and exclusion configuration."""
    return get_settings()


@app.post("/api/settings", dependencies=[Depends(require_api_token)], response_model=SettingsOut)
def write_settings(payload: SettingsIn):
    """Update privacy, retention, and exclusion configuration."""
    update_settings(**payload.model_dump(exclude_none=True))
    return get_settings()


@app.post("/api/control", dependencies=[Depends(require_api_token)], response_model=ControlOut)
def control(payload: ControlIn):
    """Pause or resume all collectors and ingestion."""
    with connect() as conn:
        conn.execute(
            "INSERT INTO settings(key,value) VALUES('paused',?) "
            "ON CONFLICT(key) DO UPDATE SET value=excluded.value",
            ("true" if payload.paused else "false",),
        )
    return {"ok": True, "paused": payload.paused}


@app.post("/api/control/delete-all", dependencies=[Depends(require_api_token)], response_model=DeleteOut)
def delete_all_data():
    """Delete all activity data (events and metrics)."""
    with connect() as conn:
        events = conn.execute("DELETE FROM events").rowcount
        metrics = conn.execute("DELETE FROM metrics").rowcount
    return {"ok": True, "deleted": {"events": events, "metrics": metrics}}
