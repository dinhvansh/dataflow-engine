"""CORE-RUN-5 opt-in, non-mocked browser → API → four actual disposable services.

Refuses shared tables, unexpected containers, non-SELECT-only QA source identities,
and non-synthetic bearer tokens. A green mocked browser CI is NOT this test.
"""
import base64
import json
import os
import re
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
import zipfile
from pathlib import Path

import pytest
from playwright.sync_api import expect, sync_playwright

BASE = os.getenv("DATAFLOW_BROWSER_BASE_URL", "http://127.0.0.1:15173")
API = os.getenv("DATAFLOW_LIVE_API_URL", "http://127.0.0.1:18080")
TOKEN = os.getenv("DATAFLOW_LIVE_API_TOKEN", "")
SYNTHETIC_TOKEN = "synthetic-live-browser-token-not-a-secret"
CH_PASSWORD = os.getenv("CLICKHOUSE_ADMIN_PASSWORD", "")
READER_PASSWORD = os.getenv("QA_READER_PASSWORD", "")
OUT = Path("qa-results/core-run5")
SUFFIX = uuid.uuid4().hex[:10]
A, B, C = (f"cr5_{SUFFIX}_{step}" for step in ("a", "b", "c"))
TARGET = "dft_core_" + C
RAW = "dataflow_raw_events"

pytestmark = pytest.mark.skipif(
    os.getenv("DATAFLOW_LIVE_QA_OPT_IN") != "true",
    reason="explicit live isolated QA opt-in required",
)


def shell(*command):
    return subprocess.check_output(command, text=True, stderr=subprocess.DEVNULL).strip()


def owned(name):
    label = shell(
        "docker", "inspect", "-f",
        '{{index .Config.Labels "com.dataflow.owner"}} {{index .Config.Labels "com.dataflow.role"}}',
        name,
    )
    assert label == "dataflow-localqa local-qa", f"Unowned container refused: {name}"


def assert_loopback(url):
    parsed = urllib.parse.urlparse(url)
    assert parsed.scheme == "http" and parsed.hostname in {"127.0.0.1", "localhost"}
    assert not parsed.username and not parsed.password and not parsed.query


def api(path, method="GET", data=None, token=TOKEN):
    raw = None if data is None else json.dumps(data).encode()
    request = urllib.request.Request(API + path, data=raw, method=method)
    request.add_header("Authorization", "Bearer " + token)
    if raw is not None:
        request.add_header("Content-Type", "application/json")
    with urllib.request.urlopen(request, timeout=30) as response:
        return response.status, json.load(response)


def ch(query):
    url = "http://127.0.0.1:18123/?query=" + urllib.parse.quote(query)
    request = urllib.request.Request(url, method="POST")
    request.add_header("Authorization", "Basic " + base64.b64encode(
        ("qa_admin:" + CH_PASSWORD).encode()).decode())
    with urllib.request.urlopen(request, timeout=45) as response:
        return response.read().decode().strip()


def pg(query):
    return shell(
        "docker", "exec", "dataflow-localqa-postgres", "psql",
        "-U", "dataflow_qa", "-d", "dataflow_qa", "-Atc", query,
    )


def read_only_source():
    result = shell(
        "docker", "exec", "dataflow-localqa-mssql",
        "/opt/mssql-tools18/bin/sqlcmd", "-C", "-h", "-1", "-W",
        "-S", "localhost", "-U", "qa_erp_reader", "-P", READER_PASSWORD,
        "-d", "DataFlowFixtureQA",
        "-Q", "SET NOCOUNT ON; SELECT DB_NAME(),ORIGINAL_LOGIN(),COUNT_BIG(*), "
              "HAS_PERMS_BY_NAME(N'Erp.PartTran',N'OBJECT',N'SELECT'), "
              "HAS_PERMS_BY_NAME(N'Erp.PartTran',N'OBJECT',N'INSERT'), "
              "HAS_PERMS_BY_NAME(N'Erp.PartTran',N'OBJECT',N'UPDATE'), "
              "HAS_PERMS_BY_NAME(N'Erp.PartTran',N'OBJECT',N'DELETE'), "
              "HAS_PERMS_BY_NAME(N'Erp.PartTran',N'OBJECT',N'ALTER') "
              "FROM Erp.PartTran;",
    )
    values = result.split()
    assert values == ["DataFlowFixtureQA", "qa_erp_reader", "10000", "1", "0", "0", "0", "0"], (
        "Refusing source unless EXACTLY DataFlowFixtureQA/qa_erp_reader has 10k rows "
        "and effective SELECT-only permissions"
    )


def safe_trace(path):
    protected = [
        os.getenv(key, "") for key in (
            "MSSQL_SA_PASSWORD", "QA_SEED_PASSWORD", "QA_READER_PASSWORD",
            "POSTGRES_PASSWORD", "CLICKHOUSE_ADMIN_PASSWORD",
            "CLICKHOUSE_READER_PASSWORD", "CLICKHOUSE_WRITER_PASSWORD",
            "DATAFLOW_API_TOKEN",
        )
    ]
    with zipfile.ZipFile(path) as zf:
        content = b"".join(zf.read(key) for key in zf.namelist())
    for secret in filter(None, protected):
        if secret.encode() in content:
            path.unlink(missing_ok=True)
            pytest.fail("Browser trace leaked a protected environment secret")


def assert_environment():
    assert TOKEN == SYNTHETIC_TOKEN, "Never record a real bearer token in Playwright"
    assert CH_PASSWORD and READER_PASSWORD, "QA credentials must be supplied securely"
    assert_loopback(API)
    assert_loopback(BASE)
    for container in ("mssql", "postgres", "clickhouse", "api", "web", "dbt-worker"):
        owned("dataflow-localqa-" + container) if container in ("mssql", "postgres", "clickhouse") else None
    # Compose doesn't give API/Web/worker explicit container_name; query by
    # project/service and verify labels, without relying on generated names.
    for service in ("api", "web", "dbt-worker"):
        cid = shell(
            "docker", "compose", "--project-name", "dataflow-localqa",
            "--env-file", ".env.qa.local", "-f", "deploy/local-qa/compose.qa.yml",
            "ps", "-q", service,
        )
        assert cid, f"Missing running {service}"
        owned(cid)
    read_only_source()
    with urllib.request.urlopen(API + "/healthz", timeout=5) as response:
        assert response.read() == b"ok"
    with urllib.request.urlopen(BASE + "/healthz", timeout=5) as response:
        assert response.read() == b"ok"
    try:
        api("/api/v1/managed/streams", token="not-the-test-token")
        pytest.fail("Unauthenticated access unexpectedly permitted")
    except urllib.error.HTTPError as exc:
        assert exc.code == 401, "Missing bearer must fail closed"
    assert ch(
        "SELECT count() FROM system.tables "
        "WHERE database='scratch' AND name='dataflow_raw_events' FORMAT TSV"
    ) == "0", "Refusing pre-existing ClickHouse raw table: fresh disposable QA required"
    assert pg("SELECT count(*) FROM sync_streams") == "0", (
        "Fresh disposable PG metadata required; never overwrite an existing checkpoint"
    )


def build_fixture():
    # DDL here touches only independently verified disposable ClickHouse scratch.
    ch(
        "CREATE TABLE scratch.dataflow_raw_events ("
        "_stream_id String,_pk String,_sysrevid String,_payload String,"
        "_run_id String,_batch_seq UInt64,"
        "_ingested_at DateTime64(3,'UTC') DEFAULT now64(3))"
        " ENGINE=MergeTree ORDER BY (_stream_id,_pk,_sysrevid)"
    )
    status, stream = api("/api/v1/managed/streams", "POST", {
        "source_schema": "Erp", "source_table": "PartTran",
        "target_database": "scratch", "target_table": RAW,
        "batch_size": 500, "workers": 2, "queue_capacity": 2,
    })
    assert status == 201 and re.fullmatch(r"[0-9a-f-]{36}", stream["id"])
    models = [
        {"name": A, "mode": "visual", "source": {"database": "scratch", "table": RAW},
         "steps": [{"id": "rename_stream", "type": "rename",
                    "column": "_stream_id", "alias": "stream"}]},
        {"name": B, "mode": "visual", "source": {"database": "scratch", "table": "dfv_" + A},
         "steps": [{"id": "add_batch", "type": "calculate", "column": "_batch_seq",
                    "alias": "batch_plus_one", "formula_operator": "add", "value": "1"}]},
        {"name": C, "mode": "visual", "source": {"database": "scratch", "table": "dfv_" + B},
         "steps": [{"id": "count_stream", "type": "aggregate",
                    "group_by": ["stream"], "function": "count", "alias": "events"}]},
    ]
    final = ""
    for model in models:
        code, result = api("/api/v1/transform/models", "POST", {"model": model})
        assert code == 201 and result["name"] == model["name"]
        if model["name"] == C:
            final = result["id"]
    return stream["id"], final


def operator(page, stream_id, model_id):
    page.get_by_role("button", name="Pipelines", exact=True).click()
    section = page.get_by_role("region", name="Durable core pipeline operator")
    # Some Chromium versions expose section as generic unless a role is explicit.
    if section.count() == 0:
        section = page.locator('section[aria-label="Durable core pipeline operator"]')
    expect(section).to_be_visible()
    section.get_by_label("Core API bearer token").fill(TOKEN)
    section.get_by_role("button", name="Load saved streams and models").click()
    expect(section.get_by_label("Core source stream")).to_contain_text("Erp.PartTran")
    section.get_by_label("Core source stream").select_option(stream_id)
    section.get_by_label("Core final model").select_option(model_id)
    section.get_by_label("Core final output unique key").fill("stream")
    return section


def execute_ui_run(page, section):
    section.get_by_role("button", name="Run complete core pipeline").click()
    page.get_by_role("button", name="Start CORE Run").click()
    run_input = section.get_by_label("Core run UUID")
    expect(run_input).to_have_value(re.compile(r"^[a-f0-9-]{36}$"), timeout=15000)
    run_id = run_input.input_value()
    observed = section.locator('[aria-label="Observed real core pipeline run"]')
    expect(observed.locator('[role="status"]')).to_have_text("succeeded", timeout=240000)
    code, result = api("/api/v1/core/runs?id=" + run_id)
    assert code == 200
    assert result["status"] == "succeeded"
    assert result["managed_status"] == "succeeded"
    assert result["model_flow_status"] == "succeeded"
    assert result["rows_verified"] == 1
    return run_id, result


def test_core_run5_full_live_browser():
    assert_environment()
    OUT.mkdir(parents=True, exist_ok=True)
    stream_id, model_id = build_fixture()
    started = time.monotonic()
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        context = browser.new_context(viewport={"width": 1440, "height": 950})
        context.tracing.start(screenshots=True, snapshots=True)
        page = context.new_page()
        try:
            page.goto(BASE, wait_until="networkidle")
            section = operator(page, stream_id, model_id)
            first_id, first = execute_ui_run(page, section)
            assert first["has_backup"] is False
            assert ch(f"SELECT stream,events FROM scratch.{TARGET} FORMAT TSV") == (
                f"{stream_id}\t10000"
            ), "Published ClickHouse count must match actual SQL Server 10k fixture"
            first_uuid = ch(
                "SELECT toString(uuid) FROM system.tables "
                f"WHERE database='scratch' AND name='{TARGET}' FORMAT TSV"
            )
            assert first_uuid
            assert pg("SELECT generation FROM sync_checkpoints WHERE "
                      f"stream_id='{stream_id}'") == "1"
            # A page refresh cannot synthesize previous local React state.
            page.reload(wait_until="networkidle")
            page.get_by_role("button", name="Pipelines", exact=True).click()
            section = page.locator('section[aria-label="Durable core pipeline operator"]')
            section.get_by_label("Core API bearer token").fill(TOKEN)
            section.get_by_label("Core run UUID").fill(first_id)
            section.get_by_role("button", name="Inspect backend state").click()
            expect(section.locator('[role="status"]')).to_have_text("succeeded")
            section.get_by_role("button", name="Load saved streams and models").click()
            section.get_by_label("Core source stream").select_option(stream_id)
            section.get_by_label("Core final model").select_option(model_id)
            section.get_by_label("Core final output unique key").fill("stream")
            second_id, second = execute_ui_run(page, section)
            assert second["has_backup"] is True
            second_uuid = ch(
                "SELECT toString(uuid) FROM system.tables "
                f"WHERE database='scratch' AND name='{TARGET}' FORMAT TSV"
            )
            assert second_uuid and second_uuid != first_uuid
            assert pg("SELECT rows_written FROM managed_runs WHERE id='"
                      + second["managed_run_id"] + "'") == "0", (
                "Second SQL Server checkpoint run must transfer zero new rows"
            )
            expect(section.get_by_role(
                "button", name="Rollback to previous physical version"
            )).to_be_visible()
            section.get_by_role(
                "button", name="Rollback to previous physical version"
            ).click()
            page.get_by_role("button", name="Confirm scratch rollback").click()
            expect(section.locator('[role="status"]')).to_have_text("rolled_back", timeout=30000)
            assert ch(
                "SELECT toString(uuid) FROM system.tables "
                f"WHERE database='scratch' AND name='{TARGET}' FORMAT TSV"
            ) == first_uuid, "Rollback did not restore exact physical target UUID"
            assert pg("SELECT generation FROM sync_checkpoints WHERE "
                      f"stream_id='{stream_id}'") == "2", (
                "Rollback must not rewind source checkpoint"
            )
            assert ch(f"SELECT stream,events FROM scratch.{TARGET} FORMAT TSV") == (
                f"{stream_id}\t10000"
            )
            page.screenshot(path=str(OUT / "operator-rollback.png"), full_page=True)
            summary = {
                "gate": "CORE-RUN-5", "scope": "isolated real UI/API/PG/MSSQL/CH/dbt",
                "source": "DataFlowFixtureQA.Erp.PartTran",
                "source_rows": 10000, "target_rows": 1, "source_events": 10000,
                "first_run_id": first_id, "second_run_id": second_id,
                "model_flow_status": second["model_flow_status"],
                "managed_status": second["managed_status"],
                "checkpoint_generation": 2,
                "rollback_target_uuid_matches_first": True,
                "elapsed_seconds": round(time.monotonic() - started, 3),
            }
            (OUT / "operator-result.json").write_text(
                json.dumps(summary, indent=2) + "\n", encoding="utf-8"
            )
        finally:
            trace = OUT / "operator-trace.zip"
            context.tracing.stop(path=str(trace))
            safe_trace(trace)
            context.close()
            browser.close()
