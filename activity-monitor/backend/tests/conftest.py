import os
import tempfile

# Must be set before activity_monitor.db is imported: DB_PATH is resolved once.
_TEST_DIR = tempfile.mkdtemp(prefix="activity-monitor-tests-")
os.environ["ACTIVITY_MONITOR_DB"] = os.path.join(_TEST_DIR, "activity.db")

import pytest

from activity_monitor.db import connect, init_db


@pytest.fixture()
def client():
    """TestClient with a clean database and no background collectors."""
    from fastapi.testclient import TestClient

    from activity_monitor.main import app

    init_db()
    with connect() as conn:
        conn.execute("DELETE FROM events")
        conn.execute("DELETE FROM metrics")
        conn.execute("UPDATE settings SET value='false' WHERE key='paused'")
        conn.execute("UPDATE settings SET value='30' WHERE key='retention_days'")
        conn.execute("UPDATE settings SET value='full' WHERE key='domain_storage_mode'")
        conn.execute("UPDATE settings SET value='[]' WHERE key='excluded_domains'")
        conn.execute("UPDATE settings SET value='[]' WHERE key='excluded_apps'")
        conn.execute("UPDATE settings SET value='false' WHERE key='exclude_titles'")
        conn.execute("UPDATE settings SET value='true' WHERE key='strip_query_fragments'")
        conn.execute("UPDATE settings SET value='true' WHERE key='store_cwd'")
        conn.execute("UPDATE settings SET value='5' WHERE key='metrics_interval_seconds'")

    # No context manager: skip the FastAPI lifespan so collectors do not run.
    return TestClient(app)


@pytest.fixture()
def token():
    from activity_monitor.db import get_api_token

    return get_api_token()


@pytest.fixture()
def auth(token):
    return {"X-API-Token": token}
