import os
import sqlite3
import json
import secrets
from datetime import datetime, timezone
from pathlib import Path
from contextlib import contextmanager
import stat

APP_DIR = Path(os.environ.get("XDG_DATA_HOME", Path.home() / ".local/share")) / "activity-monitor"
DB_PATH = Path(os.environ.get("ACTIVITY_MONITOR_DB", APP_DIR / "activity.db"))

# Schema version for migrations
SCHEMA_VERSION = 1

DEFAULT_SETTINGS = {
    "paused": "false",
    "schema_version": str(SCHEMA_VERSION),
    "retention_days": "30",
    "domain_storage_mode": "full",  # 'full' or 'domain_only'
    "excluded_domains": "[]",
    "excluded_apps": "[]",
    "exclude_titles": "false",
    "strip_query_fragments": "true",
    "store_cwd": "true",
    "metrics_interval_seconds": "5",
    "api_token": "",
}


def paused():
    """Check if activity monitoring is paused."""
    with connect() as conn:
        row = conn.execute("SELECT value FROM settings WHERE key='paused'").fetchone()
    return bool(row and row["value"] == "true")


def init_db():
    """Initialize the database with schema and indexes. Supports safe migrations."""
    # Never create group/world-readable database files, even under a permissive umask.
    os.umask(0o077)
    DB_PATH.parent.mkdir(parents=True, exist_ok=True)

    # Set restrictive permissions on the database directory
    try:
        DB_PATH.parent.chmod(stat.S_IRWXU)  # Owner only: rwx
    except (NameError, PermissionError, OSError):
        pass

    with connect() as conn:
        conn.executescript("""
        -- Events table
        CREATE TABLE IF NOT EXISTS events (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            ts TEXT NOT NULL,
            source TEXT NOT NULL,
            event_type TEXT NOT NULL,
            app TEXT,
            title TEXT,
            url TEXT,
            domain TEXT,
            cwd TEXT,
            command TEXT,
            duration_seconds REAL,
            details_json TEXT
        );
        CREATE INDEX IF NOT EXISTS idx_events_ts ON events(ts DESC);
        CREATE INDEX IF NOT EXISTS idx_events_source_ts ON events(source, ts DESC);
        CREATE INDEX IF NOT EXISTS idx_events_domain_ts ON events(domain, ts DESC);
        CREATE INDEX IF NOT EXISTS idx_events_app_ts ON events(app, ts DESC);

        -- Metrics table
        CREATE TABLE IF NOT EXISTS metrics (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            ts TEXT NOT NULL,
            cpu_percent REAL,
            memory_percent REAL,
            memory_used_bytes INTEGER,
            net_sent_bytes INTEGER,
            net_recv_bytes INTEGER,
            gpu_percent REAL,
            gpu_memory_used_mb REAL,
            gpu_temperature_c REAL
        );
        CREATE INDEX IF NOT EXISTS idx_metrics_ts ON metrics(ts DESC);

        -- Settings table
        CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
        INSERT OR IGNORE INTO settings(key, value) VALUES ('paused', 'false');
        INSERT OR IGNORE INTO settings(key, value) VALUES ('schema_version', '1');
        INSERT OR IGNORE INTO settings(key, value) VALUES ('retention_days', '30');
        INSERT OR IGNORE INTO settings(key, value) VALUES ('domain_storage_mode', 'full');  -- 'full' or 'domain_only'

        -- Privacy settings
        INSERT OR IGNORE INTO settings(key, value) VALUES ('excluded_domains', '[]');
        INSERT OR IGNORE INTO settings(key, value) VALUES ('excluded_apps', '[]');
        INSERT OR IGNORE INTO settings(key, value) VALUES ('exclude_titles', 'false');
        INSERT OR IGNORE INTO settings(key, value) VALUES ('strip_query_fragments', 'true');
        INSERT OR IGNORE INTO settings(key, value) VALUES ('store_cwd', 'true');
        INSERT OR IGNORE INTO settings(key, value) VALUES ('metrics_interval_seconds', '5');
        INSERT OR IGNORE INTO settings(key, value) VALUES ('api_token', '');
        """)

    # Run migrations if needed
    run_migrations()

    # Enforce owner-only permissions on the database and SQLite sidecar files.
    for suffix in ("", "-wal", "-shm"):
        path = DB_PATH.with_name(DB_PATH.name + suffix)
        if path.exists():
            try:
                path.chmod(stat.S_IRUSR | stat.S_IWUSR)  # 0600
            except OSError:
                pass


def run_migrations():
    """Run database schema migrations if needed."""
    with connect() as conn:
        current_version = conn.execute(
            "SELECT value FROM settings WHERE key='schema_version'"
        ).fetchone()
        if current_version and current_version["value"] == str(SCHEMA_VERSION):
            return  # Already migrated to current version

        # Migration from v0 to v1 (initial schema already created above)
        # Add any future migrations here

        # Update schema version
        conn.execute(
            "UPDATE settings SET value = ? WHERE key = 'schema_version'",
            (str(SCHEMA_VERSION),),
        )


def _get_setting(key, default=None):
    with connect() as conn:
        row = conn.execute("SELECT value FROM settings WHERE key=?", (key,)).fetchone()
    return row["value"] if row else default


def _get_json_setting(key):
    raw = _get_setting(key, "[]")
    try:
        value = json.loads(raw) if raw else []
    except (json.JSONDecodeError, TypeError):
        return []
    return value if isinstance(value, list) else []


def _get_excluded_domains():
    """Get the list of excluded domains from settings."""
    return [str(d).lower() for d in _get_json_setting("excluded_domains") if d]


def _get_excluded_apps():
    """Get the list of excluded application identifiers from settings."""
    return [str(a).lower() for a in _get_json_setting("excluded_apps") if a]


def _should_exclude_domain(domain):
    """True only when a domain is present and matches an exclusion rule."""
    if not domain:
        return False
    domain = domain.lower()
    # Also auto-exclude localhost/127.0.0.1/.local
    if domain in ("localhost", "127.0.0.1", "::1") or domain.endswith(".local"):
        return True
    for d in _get_excluded_domains():
        d = d.lstrip(".").lower()
        if domain == d or domain.endswith("." + d):
            return True
    return False


def _should_exclude_app(app):
    """True only when an app identifier matches an exclusion rule."""
    if not app:
        return False
    app = app.lower()
    for excluded in _get_excluded_apps():
        if app == excluded:
            return True
    return False


def _should_strip_query_fragments():
    """Check if query strings/fragments should be stripped from URLs."""
    return _get_setting("strip_query_fragments", "true") in ("true", "True", "1")


def _should_exclude_titles():
    """Check if window/page titles must be omitted before persistence."""
    return _get_setting("exclude_titles", "false") in ("true", "True", "1")


def _should_store_cwd():
    """Check if fish working directories may be persisted."""
    return _get_setting("store_cwd", "true") in ("true", "True", "1")


def get_metrics_interval():
    """Metrics sampling interval in seconds, clamped to a safe range."""
    try:
        seconds = int(_get_setting("metrics_interval_seconds", "5"))
    except (ValueError, TypeError):
        return 5
    return min(300, max(2, seconds))


def _domain_storage_mode():
    mode = _get_setting("domain_storage_mode", "full")
    return mode if mode in ("full", "domain_only") else "full"


def _strip_query_fragment(url):
    """Strip query strings and fragments from URL."""
    if not url:
        return url
    from urllib.parse import urlparse, urlunparse
    parsed = urlparse(url)
    # Remove query and fragment
    cleaned = urlunparse((parsed.scheme, parsed.netloc, parsed.path, parsed.params, "", ""))
    return cleaned


def normalize_ts(value):
    """Normalize an ISO-8601 timestamp to UTC, falling back to now()."""
    if value:
        try:
            parsed = datetime.fromisoformat(str(value).replace("Z", "+00:00"))
            if parsed.tzinfo is None:
                parsed = parsed.astimezone()
            return parsed.astimezone(timezone.utc).isoformat(timespec="seconds")
        except (ValueError, TypeError):
            pass
    return datetime.now(timezone.utc).isoformat(timespec="seconds")


def get_retention_days():
    """Get the retention period in days from settings."""
    try:
        days = int(_get_setting("retention_days", "30"))
    except (ValueError, TypeError):
        return 30
    return days


def get_settings():
    """Return the user-facing privacy/retention configuration."""
    return {
        "excluded_domains": _get_excluded_domains(),
        "excluded_apps": _get_excluded_apps(),
        "retention_days": get_retention_days(),
        "strip_query_fragments": _should_strip_query_fragments(),
        "domain_storage_mode": _domain_storage_mode(),
        "exclude_titles": _should_exclude_titles(),
        "store_cwd": _should_store_cwd(),
        "metrics_interval_seconds": get_metrics_interval(),
    }


def update_settings(**changes):
    """Persist a partial settings update. Only known keys are accepted."""
    serializers = {
        "excluded_domains": lambda v: json.dumps([str(d).strip().lstrip(".").lower() for d in v if str(d).strip()]),
        "excluded_apps": lambda v: json.dumps([str(a).strip().lower() for a in v if str(a).strip()]),
        "retention_days": lambda v: str(int(v)),
        "strip_query_fragments": lambda v: "true" if v else "false",
        "domain_storage_mode": lambda v: v,
        "exclude_titles": lambda v: "true" if v else "false",
        "store_cwd": lambda v: "true" if v else "false",
        "metrics_interval_seconds": lambda v: str(min(300, max(2, int(v)))),
    }
    with connect() as conn:
        for key, value in changes.items():
            if key not in serializers or value is None:
                continue
            conn.execute(
                "INSERT INTO settings(key,value) VALUES(?,?) "
                "ON CONFLICT(key) DO UPDATE SET value=excluded.value",
                (key, serializers[key](value)),
            )


def get_api_token():
    """Get the API token from settings, generating one if it doesn't exist."""
    with connect() as conn:
        row = conn.execute("SELECT value FROM settings WHERE key='api_token'").fetchone()
        if row and row["value"]:
            return row["value"]
        # Generate a new token
        import secrets
        token = secrets.token_urlsafe(32)
        conn.execute(
            "UPDATE settings SET value = ? WHERE key = 'api_token'",
            (token,),
        )
        return token


def validate_api_token(token: str) -> bool:
    """Validate the provided API token against the stored one."""
    if not token:
        return False
    stored = get_api_token()
    return secrets.compare_digest(token, stored)


@contextmanager
def connect():
    """Get a database connection with proper settings."""
    conn = sqlite3.connect(str(DB_PATH), timeout=10)
    conn.row_factory = sqlite3.Row
    # Use WAL mode for better concurrent read performance
    conn.execute("PRAGMA journal_mode=WAL")
    # Set busy timeout to handle concurrent access
    conn.execute("PRAGMA busy_timeout = 5000")
    try:
        yield conn
        conn.commit()
    finally:
        conn.close()