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
    for service in ("mssql", "postgres", "clickhouse"):
        owned("dataflow-localqa-" + service)
    # Compose doesn't give API/Web/worker explicit container_name; query by
    # project/service and verify labels, without relying on generated names.
    for service in ("api", "web", "dbt-worker"):
        cid = shell(
            "docker", "compose", "--project-name", "dataflow-localqa",
            "--env-file", ".env.qa.local", "-f", "deploy/local-qa/compose.qa.yml",
            "--profile", "operator", "ps", "-q", service,
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


def execute_ui_run(page, section, prior_id=None):
    section.get_by_role("button", name="Run complete core pipeline").click()
    page.get_by_role("button", name="Start CORE Run").click()
    run_input = section.get_by_label("Core run UUID")
    # The form retains the previous persisted UUID until the new POST returns.
    # Do not accidentally count the old successful run as this new one.
    if prior_id is not None:
        expect(run_input).not_to_have_value(prior_id, timeout=30000)
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
    assert result["dependency_order"] == [A, B, C], (
        "GET must read immutable dependency order back from PostgreSQL"
    )
    assert [step["model_name"] for step in result["steps"]] == [A, B, C]
    assert [step["index"] for step in result["steps"]] == [0, 1, 2]
    assert all(step["status"] == "succeeded" for step in result["steps"])
    runtime = section.locator('[aria-label="Durable Core runtime canvas"]')
    expect(runtime.locator('[data-testid="core-runtime-node-TRANSFORM"]')).to_have_count(3)
    expect(runtime.locator('[data-testid="core-runtime-node-QUALITY"] '
                           '[data-testid="core-runtime-status"]')).to_have_text("succeeded")
    expect(runtime.locator('[data-testid="core-runtime-node-PUBLISH"] '
                           '[data-testid="core-runtime-status"]')).to_have_text("succeeded")
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
            runtime = section.locator('[aria-label="Durable Core runtime canvas"]')
            expect(runtime.locator('[data-testid="core-runtime-node-TRANSFORM"]')).to_have_count(3)
            expect(runtime.locator('[data-testid="core-runtime-node-TRANSFORM"] '
                                   '[data-testid="core-runtime-status"]')).to_have_text(
                                       ["succeeded", "succeeded", "succeeded"])
            expect(section.get_by_text("Server dependency order: " + " → ".join([A,B,C]))).to_be_visible()
            section.get_by_role("button", name="Load saved streams and models").click()
            section.get_by_label("Core source stream").select_option(stream_id)
            section.get_by_label("Core final model").select_option(model_id)
            section.get_by_label("Core final output unique key").fill("stream")
            second_id, second = execute_ui_run(page, section, prior_id=first_id)
            assert second_id != first_id, "Second operator POST must persist a new UUID"
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
            # CORE-RUN-8B: save a real typed graph, then launch its exact
            # immutable revision from the new React Flow operator surface.
            models_code, model_rows = api("/api/v1/transform/models")
            assert models_code == 200
            names = {item["name"]: item["id"] for item in model_rows["models"]}
            graph = {"nodes": [
                {"id": "source", "type": "SOURCE", "stream_id": stream_id,
                 "requires": [], "x": 40, "y": 50},
                {"id": "sync", "type": "SYNC", "requires": ["source"], "x": 290, "y": 50},
                {"id": "transform_a", "type": "TRANSFORM", "model_id": names[A],
                 "requires": ["sync"], "x": 540, "y": 50},
                {"id": "transform_b", "type": "TRANSFORM", "model_id": names[B],
                 "requires": ["transform_a"], "x": 790, "y": 50},
                {"id": "transform_c", "type": "TRANSFORM", "model_id": names[C],
                 "requires": ["transform_b"], "x": 1040, "y": 50},
                {"id": "quality", "type": "QUALITY", "unique_column": "stream",
                 "requires": ["transform_c"], "x": 1290, "y": 50},
                {"id": "publish", "type": "PUBLISH",
                 "requires": ["quality"], "x": 1540, "y": 50}
            ]}
            graph_name = "qa_core_" + SUFFIX
            checked, verified = api("/api/v1/core/graphs", "POST", {
                "action": "validate", "name": graph_name,
                "expected_version": 0, "graph": graph
            })
            assert checked == 200 and verified["valid"]
            saved_code, saved = api("/api/v1/core/graphs", "POST", {
                "action": "save", "name": graph_name,
                "expected_version": 0, "graph": graph
            })
            assert saved_code == 200 and saved["version"] == 1
            assert saved["graph_digest"] == verified["digest"]
            invalid_graph = json.loads(json.dumps(graph))
            invalid_graph["nodes"][5]["requires"] = ["sync"]
            try:
                api("/api/v1/core/graphs", "POST", {
                    "action": "validate", "name": graph_name,
                    "expected_version": 0, "graph": invalid_graph
                })
                pytest.fail("forked graph was accepted")
            except urllib.error.HTTPError as exc:
                assert exc.code == 422, "Graph forks must fail closed"
            try:
                api("/api/v1/core/graphs", "POST", {
                    "action": "save", "name": graph_name,
                    "expected_version": 0, "graph": graph
                })
                pytest.fail("stale graph revision silently overwrote saved graph")
            except urllib.error.HTTPError as exc:
                assert exc.code == 409
            try:
                api("/api/v1/core/runs", "POST", {
                    "stream_id": stream_id, "model_id": model_id,
                    "unique_column": "stream", "graph_id": saved["id"],
                    "graph_version": 2
                })
                pytest.fail("stale saved graph version started a real run")
            except urllib.error.HTTPError as exc:
                assert exc.code == 409
            editor = section.locator('[aria-label="Editable mixed Core Canvas"]')
            expect(editor).to_be_visible()
            editor.get_by_role("button", name="Load graphs").click()
            editor.get_by_label("Saved Core graphs").select_option(saved["id"])
            expect(editor.get_by_text("Persisted graph: " + graph_name, exact=False)).to_be_visible()
            editor.get_by_role("button", name="Run saved graph").click()
            page.get_by_role("button", name="Start saved Core graph").click()
            third_input = section.get_by_label("Core run UUID")
            expect(third_input).not_to_have_value(second_id, timeout=25000)
            expect(third_input).to_have_value(re.compile(r"^[a-f0-9-]{36}$"), timeout=25000)
            graph_run_id = third_input.input_value()
            expect(section.locator('[aria-label="Observed real core pipeline run"]')
                   .locator('[role="status"]')).to_have_text("succeeded", timeout=240000)
            _, graph_result = api("/api/v1/core/runs?id=" + graph_run_id)
            assert graph_result["graph_id"] == saved["id"]
            assert graph_result["graph_version"] == 1
            assert graph_result["graph_digest"] == saved["graph_digest"]
            assert [n["type"] for n in graph_result["graph_snapshot"]["nodes"]] == [
                "SOURCE", "SYNC", "TRANSFORM", "TRANSFORM", "TRANSFORM",
                "QUALITY", "PUBLISH"
            ]
            assert [step["model_name"] for step in graph_result["steps"]] == [A, B, C]
            assert pg("SELECT generation FROM sync_checkpoints WHERE "
                      f"stream_id='{stream_id}'") == "3"
            assert pg("SELECT rows_written FROM managed_runs WHERE id='"
                      + graph_result["managed_run_id"] + "'") == "0"
            page.reload(wait_until="networkidle")
            page.get_by_role("button", name="Pipelines", exact=True).click()
            section = page.locator('section[aria-label="Durable core pipeline operator"]')
            section.get_by_label("Core API bearer token").fill(TOKEN)
            section.get_by_label("Core run UUID").fill(graph_run_id)
            section.get_by_role("button", name="Inspect backend state").click()
            expect(section.locator('[aria-label="Durable Core runtime canvas"]')
                   .locator('[data-testid="core-runtime-node-TRANSFORM"]')).to_have_count(3)
            assert api("/api/v1/core/runs?id=" + graph_run_id)[1]["graph_snapshot"] == graph_result["graph_snapshot"]
            # GUI acceptance, not just API acceptance: after a complete
            # browser reload, exercise the actual editor controls against
            # this real PG-backed graph. Adding an unrelated dbt step must
            # fail server validation, removing it repairs the linear chain.
            section.get_by_role(
                "button", name="Load saved streams and models"
            ).click()
            editor = section.locator('[aria-label="Editable mixed Core Canvas"]')
            editor.get_by_role("button", name="Load graphs").click()
            editor.get_by_label("Saved Core graphs").select_option(saved["id"])
            expect(editor.get_by_text(
                "Persisted graph: " + graph_name, exact=False
            )).to_be_visible()
            editor.get_by_label("Palette Core transform model").select_option(names[A])
            editor.get_by_role("button", name="Add transform").click()
            expect(editor.get_by_role("button", name="Remove transform")).to_be_visible()
            expect(editor.get_by_role("button", name="Run saved graph")).to_be_disabled()
            editor.get_by_role("button", name="Validate").click()
            expect(editor.locator('[role="alert"]')).to_contain_text(
                "Core graph rejected", timeout=15000
            )
            editor.get_by_role("button", name="Remove transform").click()
            editor.locator('.react-flow__node[data-id="quality"]').click()
            unique_key = editor.get_by_label("Core graph DQ key")
            expect(unique_key).to_have_value("stream")
            unique_key.fill("events")
            expect(editor.get_by_role("button", name="Run saved graph")).to_be_disabled()
            editor.get_by_role("button", name="Validate").click()
            expect(editor.locator('[role="status"]').filter(
                has_text="Server validated graph digest"
            )).to_be_visible(timeout=15000)
            editor.get_by_role("button", name="Save Core graph").click()
            expect(editor.get_by_text(
                "Persisted graph: " + graph_name + " · rev 2"
            )).to_be_visible(timeout=15000)
            page.screenshot(path=str(OUT / "operator-editor-revision.png"),
                            full_page=True)
            # Graph lifecycle: the actual UI edit must preserve the frozen
            # snapshot of the earlier completed graph-bound Core Run.
            graph_rc, registry_v2 = api("/api/v1/core/graphs")
            assert graph_rc == 200
            graph_v2 = next(x for x in registry_v2["graphs"]
                            if x["id"] == saved["id"])
            assert graph_v2["version"] == 2
            assert graph_v2["graph_digest"] != saved["graph_digest"]
            assert graph_v2["id"] == saved["id"], "Editing must not switch identity"
            assert next(n for n in graph_v2["graph"]["nodes"]
                        if n["type"] == "QUALITY")["unique_column"] == "events"
            altered = graph_v2["graph"]
            # Old draft must fail CAS without silently advancing the revision.
            try:
                api("/api/v1/core/graphs", "POST", {
                    "action": "save", "name": graph_name,
                    "expected_version": 1, "graph": altered
                })
                pytest.fail("old version overwrite accepted after revision 2")
            except urllib.error.HTTPError as exc:
                assert exc.code == 409
            # Old Run's graph is a frozen DB snapshot, unaffected by edits.
            _, frozen_run = api("/api/v1/core/runs?id=" + graph_run_id)
            assert frozen_run["graph_version"] == 1
            assert frozen_run["graph_digest"] == saved["graph_digest"]
            assert frozen_run["graph_snapshot"] == graph_result["graph_snapshot"]
            assert pg("SELECT graph_version FROM core_pipeline_runs "
                      f"WHERE id='{graph_run_id}'") == "1"
            # A stale graph launch must fail before creating a managed run.
            before_count = pg("SELECT count(*) FROM managed_runs")
            try:
                api("/api/v1/core/runs", "POST", {
                    "stream_id": stream_id, "model_id": model_id,
                    "unique_column": "stream",
                    "graph_id": saved["id"], "graph_version": 1
                })
                pytest.fail("stale graph revision launched after edit")
            except urllib.error.HTTPError as exc:
                assert exc.code == 409
            assert pg("SELECT count(*) FROM managed_runs") == before_count
            assert pg("SELECT generation FROM sync_checkpoints "
                      f"WHERE stream_id='{stream_id}'") == "3"
            # Server keeps only the latest editable definition in registry,
            # but every run keeps its own permanently readable old snapshot.
            list_rc, catalog = api("/api/v1/core/graphs")
            assert list_rc == 200
            stored = next(g for g in catalog["graphs"] if g["id"] == saved["id"])
            assert stored["version"] == 2 and stored["graph_digest"] == graph_v2["graph_digest"]
            # Real authorization regression: persisted graph registry is private.
            try:
                api("/api/v1/core/graphs", token="invalid-qa-bearer")
                pytest.fail("Graph registry exposed without a valid bearer token")
            except urllib.error.HTTPError as exc:
                assert exc.code == 401
            page.screenshot(path=str(OUT / "operator-graph-reload.png"), full_page=True)
            summary = {
                "gate": "CORE-RUN-5", "scope": "isolated real UI/API/PG/MSSQL/CH/dbt",
                "source": "DataFlowFixtureQA.Erp.PartTran",
                "source_rows": 10000, "target_rows": 1, "source_events": 10000,
                "first_run_id": first_id, "second_run_id": second_id,
                "model_flow_status": second["model_flow_status"],
                "managed_status": second["managed_status"],
                "checkpoint_generation": 2,
                "rollback_target_uuid_matches_first": True,
                "graph_run_id": graph_run_id, "graph_version": 1,
                "immutable_graph_snapshot_survives_reload": True,
                "stale_graph_and_fork_rejected": True,
                "updated_graph_version": 2,
                "old_run_snapshot_survives_graph_edit": True,
                "stale_revision_launch_no_managed_run": True,
                "unauthorized_graph_registry_denied": True,
                "interactive_editor_controls_qa": True,
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
