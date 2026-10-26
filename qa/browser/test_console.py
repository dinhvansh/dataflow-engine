"""DataFlow browser interaction checks with ONLY mocked API responses.

These checks are useful for UI behavior and accessibility regressions.
They are NOT real SQL Server/PG/ClickHouse/dbt end-to-end acceptance.
"""
import json
import os
import re
from pathlib import Path
from urllib.parse import urlparse

import pytest
from playwright.sync_api import expect, sync_playwright

BASE = os.getenv("DATAFLOW_BROWSER_BASE_URL", "http://127.0.0.1:5173")
TOKEN = "synthetic-browser-token-not-a-secret"


@pytest.fixture(scope="session")
def browser():
    with sync_playwright() as pw:
        browser = pw.chromium.launch(headless=True)
        yield browser
        browser.close()


@pytest.fixture
def ui(browser, request):
    context = browser.new_context(viewport={"width": 1440, "height": 900})
    context.tracing.start(screenshots=True, snapshots=True, sources=True)
    page = context.new_page()
    state = {
        "calls": [],
        "models": [
            {"id": "qa_model_a", "name": "m_a", "model": {"name": "m_a", "mode": "visual", "source": {"database": "scratch", "table": "events"}, "steps": []}},
            {"id": "qa_model_b", "name": "m_b", "model": {"name": "m_b", "mode": "visual", "source": {"database": "scratch", "table": "dfv_m_a"}, "steps": []}},
            {"id": "qa_model_c", "name": "m_c", "model": {"name": "m_c", "mode": "visual", "source": {"database": "scratch", "table": "dfv_m_b"}, "steps": []}},
        ],
        "incremental": None,
        "core_runs": {},
        "core_count": 0,
        "sql_jobs": [
            {"id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "name": "revenue_quality_check",
             "database": "scratch", "query": "SELECT count() AS total_rows FROM scratch.events", "enabled": True},
            {"id": "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "name": "inventory_snapshot_check",
             "database": "scratch", "query": "SELECT 1", "enabled": False},
        ],
        "draft_connections": [],
        "runtime_connections": [],
        "pipelines": [
            {"id": "33333333-3333-4333-8333-333333333333", "name": "revenue_reporting",
             "enabled": True, "nodes": [
                 {"id": "extract_invoice", "job_id": "44444444-4444-4444-8444-444444444444", "requires": []},
                 {"id": "revenue_check", "job_id": "55555555-5555-4555-8555-555555555555", "requires": ["extract_invoice"]}
             ]},
        ],
    }

    def respond(route):
        req = route.request
        path = urlparse(req.url).path
        method = req.method
        body = json.loads(req.post_data) if req.post_data else {}
        state["calls"].append({"method": method, "path": path, "body": body})
        result = None
        if path == "/api/v1/auth/session":
            # Legacy browser tests intentionally run without a configured
            # local session provider. Session-mode regressions test auth separately.
            route.fulfill(status=404, body="session mode disabled")
            return
        if path == "/api/v1/connections/definitions":
            can_write = req.headers.get("x-dataflow-connection-admin", "") == "synthetic-admin-token"
            if method == "POST":
                if not can_write:
                    route.fulfill(status=403, body="admin permission required")
                    return
                if any(item["name"] == body.get("name") for item in state["draft_connections"]):
                    route.fulfill(status=409, body="duplicate name")
                    return
                new_id = f"aaaaaaaa-aaaa-4aaa-8aaa-{len(state['draft_connections'])+1:012d}"
                new_item = {"id": new_id, "name": body["name"], "driver": body["driver"],
                            "kind": body["kind"], "workspace": "lab", "status": "draft",
                            "revision": 1, "created_at": "2026-10-17T12:00:00Z"}
                state["draft_connections"].append(new_item)
                result = new_item
            else:
                result = {"definitions": state["draft_connections"], "write_enabled": can_write}
        elif path == "/api/v1/connections/runtime":
            if req.headers.get("x-dataflow-connection-admin") != "synthetic-admin-token":
                route.fulfill(status=403, body="independent admin required")
                return
            if method == "POST":
                new_id = f"b0000000-0000-4000-8000-{len(state['runtime_connections'])+1:012d}"
                new_item = {"id": new_id, "name": body["name"], "driver": body["driver"],
                            "kind": "source" if body["driver"] == "sqlserver" else "destination",
                            "host": body["host"], "port": body["port"],
                            "database": body["database"], "username": body["username"],
                            "status": "draft", "revision": 1}
                state["runtime_connections"].append(new_item)
                result = new_item
            else:
                result = {"profiles": state["runtime_connections"]}
        elif path == "/api/v1/connections/runtime/test":
            for item in state["runtime_connections"]:
                if item["id"] == body.get("id"):
                    item["status"] = "verified"
                    item["revision"] = 2
            result = {"id": body.get("id"), "passed": True,
                      "tested_at": "2026-10-19T10:00:00Z"}
        elif path == "/api/v1/connections/runtime/discover":
            if "table=" in req.url:
                result = {"columns": [
                    {"name": name, "sql_type": typ, "nullable": False,
                     "primary_key": name == "SysRowID", "cursor_candidate": name == "SysRevID",
                     "max_length": 0, "precision": 0, "scale": 0, "identity": False}
                    for name, typ in [
                        ("SysRowID", "uniqueidentifier"), ("SysRevID", "timestamp"),
                        ("PartNum", "nvarchar"), ("TranQty", "decimal")]
                ]}
            else:
                result = {"tables": [{"schema": "Erp", "name": "Part"}, {"schema":"Erp","name":"InvcHead"}]}
        elif path == "/api/v1/connections/runtime/streams/batch":
            result = {"count": len(body.get("streams", [])), "streams": [
                {"stream_id": f"abcdabcd-abcd-4bcd-8bcd-{i+1:012d}",
                 "source": x.get("source_schema", "")+"."+x.get("source_table", ""),
                 "mode": x.get("mode", ""),
                 "destination": "scratch.dfqa_"+x.get("target_table","")+"_123456789abc"}
                for i,x in enumerate(body.get("streams", []))]}
        elif path == "/api/v1/connections/runtime/streams":
            if method == "POST":
                result = {"stream_id": "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
                          "bound": True, "mode": body.get("mode")}
            else:
                route.fulfill(status=405, body="POST required")
                return
        elif path == "/api/v1/managed/runs":
            if method == "POST":
                result = {"run_id": "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee",
                          "status": "queued", "stream_id": body.get("stream_id")}
            else:
                result = {"run_id": "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee",
                          "status": "succeeded", "rows_written": 200, "batches_written": 2}
        elif path == "/api/v1/connections/runtime/bind":
            result = {"bound": True, "stream_id": body.get("stream_id")}
        elif path == "/api/v1/connections":
            result = {"connections": [
                {"id": "env-sqlserver-source", "name": "SQL Server source", "kind": "source",
                 "driver": "sqlserver", "usage": "Read-only source discovery and test ingestion",
                 "configuration_state": "configured", "management_mode": "server_environment",
                 "supports_multiple_instances": False, "supports_test_connection": False},
                {"id": "env-clickhouse-target", "name": "ClickHouse scratch target", "kind": "destination",
                 "driver": "clickhouse", "usage": "Disposable scratch warehouse only",
                 "configuration_state": "configured", "management_mode": "server_environment",
                 "supports_multiple_instances": False, "supports_test_connection": False},
                {"id": "env-postgres-control", "name": "PostgreSQL control plane", "kind": "internal",
                 "driver": "postgresql", "usage": "Metadata, queue and run state; not a selectable data source/sink",
                 "configuration_state": "configured", "management_mode": "server_environment",
                 "supports_multiple_instances": False, "supports_test_connection": False},
            ]}
        elif path == "/api/v1/sql/jobs":
            if method == "POST":
                saved = {"id": "88888888-8888-4888-8888-888888888888",
                         "name": body.get("name", "qa_sql_job"),
                         "database": body.get("database", "scratch"),
                         "query": body.get("query", "SELECT 1"),
                         "enabled": body.get("enabled", False)}
                state["sql_jobs"] = [x for x in state["sql_jobs"] if x["name"] != saved["name"]] + [saved]
                result = saved
            else:
                result = {"jobs": state["sql_jobs"]}
        elif path == "/api/v1/pipelines/validate":
            nodes = body.get("nodes", [])
            result = {"valid": True, "nodes": len(nodes),
                      "ready_on_start": [n.get("id") for n in nodes if not n.get("requires")]}
        elif path == "/api/v1/sql/dags":
            if method == "POST":
                previous = next((d for d in state["pipelines"] if d["name"] == body.get("name")), None)
                saved = {"id": previous["id"] if previous else "99999999-9999-4999-8999-999999999999",
                         "name": body.get("name"), "enabled": body.get("enabled", False),
                         "nodes": body.get("nodes", [])}
                state["pipelines"] = [d for d in state["pipelines"] if d["name"] != saved["name"]] + [saved]
                result = saved
            else:
                result = {"pipelines": state["pipelines"]}
        elif path == "/api/v1/sql/dag-runs":
            if method == "POST":
                result = {
                    "run_id": "abababab-abab-4bab-8bab-abababababab",
                    "dag_id": body.get("dag_id"), "status": "queued"
                }
            elif "id=abababab-abab-4bab-8bab-abababababab" in req.url:
                result = {
                    "run_id": "abababab-abab-4bab-8bab-abababababab",
                    "dag_id": "33333333-3333-4333-8333-333333333333",
                    "status": "succeeded",
                    "result": {"state": {"extract_invoice": "succeeded", "revenue_check": "succeeded"},
                               "attempts": {"extract_invoice": 1, "revenue_check": 1}, "errors": {}}
                }
            elif "id=" in req.url:
                result = {
                    "run_id": "66666666-6666-4666-8666-666666666666",
                    "dag_id": "33333333-3333-4333-8333-333333333333",
                    "status": "failed", "error": "Quality gate failed",
                    "result": {"state": {"extract_invoice": "succeeded", "revenue_check": "failed"},
                               "attempts": {"extract_invoice": 1, "revenue_check": 3},
                               "errors": {"revenue_check": "Row count mismatch"}}
                }
            else:
                result = {"runs": [
                    {"run_id": "66666666-6666-4666-8666-666666666666",
                     "dag_id": "33333333-3333-4333-8333-333333333333",
                     "status": "failed", "error": "Quality gate failed"},
                    {"run_id": "77777777-7777-4777-8777-777777777777",
                     "dag_id": "33333333-3333-4333-8333-333333333333",
                     "status": "succeeded", "error": ""}
                ]}
        elif path == "/api/v1/managed/streams":
            if method == "POST":
                result = {**body, "id": "cccccccc-cccc-4ccc-8ccc-cccccccccccc", "lower_cursor": "0x0000000000000000"}
            else:
                result = {"streams": [
                {"id": "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
                 "source_schema": "Erp", "source_table": "PartTran",
                 "target_database": "scratch", "target_table": "dataflow_raw_events",
                 "batch_size": 1000, "workers": 2, "queue_capacity": 2,
                 "lower_cursor": "0x0000000000000000"}
            ]}
        elif path == "/api/v1/core/runs":
            if method == "POST":
                state["core_count"] += 1
                number = state["core_count"]
                core_id = f"f{number:07d}-1111-4111-8111-111111111111"
                state["core_runs"][core_id] = {
                    "run_id": core_id, "status": "succeeded",
                    "managed_run_id": "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee",
                    "managed_status": "succeeded",
                    "model_flow_id": "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
                    "model_flow_status": "succeeded",
                    "target": "scratch.dft_core_m_c",
                    "rows_verified": 10000, "error": "",
                    "has_backup": number > 1,
                }
                result = {"run_id": core_id,
                          "status": "waiting_sync",
                          "dependency_order": ["m_a", "m_b", "m_c"]}
            else:
                run_id = req.url.split("id=", 1)[-1]
                result = state["core_runs"].get(run_id)
                if result is None:
                    route.fulfill(status=404, body="unknown core run")
                    return
        elif path == "/api/v1/core/actions":
            run_id = body.get("run_id")
            current = state["core_runs"].get(run_id)
            if method != "POST" or body.get("action") != "rollback" or body.get("confirm") != "ROLLBACK" or not current or not current["has_backup"] or current["status"] != "succeeded":
                route.fulfill(status=409, body="rollback denied")
                return
            current["status"] = "rolled_back"
            result = {"run_id": run_id, "status": "rolled_back",
                      "target": current["target"], "checkpoint_changed": False}
        elif path == "/api/v1/bootstrap/history":
            result = {"count": 2, "runs": [
                {"run_id": "11111111-1111-4111-8111-111111111111", "status": "succeeded",
                 "source": "Erp.Part", "target": "scratch.part",
                 "rows_written": 1000, "batches_written": 1,
                 "created_at": "2026-10-16T05:00:00Z"},
                {"run_id": "22222222-2222-4222-8222-222222222222", "status": "failed",
                 "source": "Erp.PartTran", "target": "scratch.parttran",
                 "rows_written": 0, "batches_written": 0,
                 "created_at": "2026-10-16T06:00:00Z"},
            ]}
        elif path == "/api/v1/source/tables":
            result = [{"schema": "Erp", "name": "PartTran"}]
        elif path == "/api/v1/source/columns":
            result = [
                {"name": "SysRowID", "sql_type": "uniqueidentifier", "max_length": 16, "precision": 0, "scale": 0, "nullable": False, "identity": False, "primary_key": True, "cursor_candidate": False},
                {"name": "SysRevID", "sql_type": "timestamp", "max_length": 8, "precision": 0, "scale": 0, "nullable": False, "identity": False, "primary_key": False, "cursor_candidate": True},
            ]
        elif path == "/api/v1/source/rowversion-upper":
            result = {"upper_exclusive": "0x0000000000000005"}
        elif path == "/api/v1/transform/columns":
            result = {"columns": [{"name": "PartNum", "type": "String"}, {"name": "Qty", "type": "UInt64"}]}
        elif path == "/api/v1/transform/preview":
            result = {"sql": "SELECT PartNum, Qty FROM scratch.events LIMIT 50", "rows": 1,
                      "data": [{"PartNum": "P0001", "Qty": 3}],
                      "meta": [{"name": "PartNum", "type": "String"}, {"name": "Qty", "type": "UInt64"}]}
        elif path == "/api/v1/transform/models":
            if method == "POST":
                model = body.get("model", {})
                saved = {"id": "qa_saved_model", "name": model.get("name", "unnamed"),
                         "model": model, "compiled_sql": "SELECT 1"}
                state["models"] = [m for m in state["models"] if m["id"] != saved["id"]] + [saved]
                result = saved
            else:
                result = {"models": state["models"]}
        elif path == "/api/v1/transform/lineage":
            result = {
                "nodes": [
                    {"id": "source:events", "kind": "source", "label": "events", "status": "ready"},
                    {"id": "model:m_a", "kind": "model", "label": "m_a", "status": "ready"},
                    {"id": "model:m_b", "kind": "model", "label": "m_b", "status": "ready"},
                    {"id": "model:m_c", "kind": "model", "label": "m_c", "status": "ready"},
                ],
                "edges": [
                    {"from": "source:events", "to": "model:m_a"},
                    {"from": "model:m_a", "to": "model:m_b"},
                    {"from": "model:m_b", "to": "model:m_c"},
                ],
                "model_order": ["m_a", "m_b", "m_c"], "problems": [],
            }
        elif path == "/api/v1/transform/dbt/runs":
            result = {"runs": []}
        elif path == "/api/v1/transform/auto-build":
            result = {
                "run_id": "11111111-1111-4111-8111-111111111111",
                "status": "succeeded", "target_model": "m_c",
                "step_count": 3, "current_step": 3, "order": ["m_a", "m_b", "m_c"],
                "steps": [{"index": i, "model_name": n, "status": "succeeded",
                           "dbt_run_id": "", "rows_verified": 1, "error": ""}
                          for i, n in enumerate(["m_a", "m_b", "m_c"], start=1)],
            }
        elif path == "/api/v1/transform/incremental/runs":
            if method == "POST":
                state["incremental"] = {
                    "run_id": "22222222-2222-4222-8222-222222222222",
                    "state": "ready", "target": "scratch.dft_sales_demo",
                    "delta_table": "scratch.dfd_sales_demo", "cursor_lower": "0",
                    "cursor_upper": "4", "delta_rows": 4, "rows_verified": 3,
                    "has_backup": False,
                }
                result = state["incremental"]
            else:
                result = {"runs": [state["incremental"]] if state["incremental"] else []}
        elif path == "/api/v1/transform/incremental/action":
            if not state["incremental"]:
                route.fulfill(status=409, body="no staged run")
                return
            action = body.get("action")
            if action == "publish":
                state["incremental"] = {**state["incremental"], "state": "published", "has_backup": True}
            elif action == "rollback":
                state["incremental"] = {**state["incremental"], "state": "rolled_back"}
            else:
                route.fulfill(status=422, body="invalid action")
                return
            result = state["incremental"]
        else:
            route.fulfill(status=404, body=f"unmocked endpoint: {path}")
            return
        route.fulfill(status=200, content_type="application/json", body=json.dumps(result))

    page.route("**/healthz", lambda route: route.fulfill(status=200, body="OK"))
    page.route("**/api/v1/**", respond)
    page.goto(BASE, wait_until="domcontentloaded")
    yield page, state

    artifacts = Path("qa/browser/artifacts")
    artifacts.mkdir(parents=True, exist_ok=True)
    context.tracing.stop(path=str(artifacts / (request.node.name + ".zip")))
    context.close()


def open_tab(page, name):
    page.get_by_role("button", name=name, exact=True).click()
    page.locator("input[type=password]").first.fill(TOKEN)


def test_ui01_source_metadata_read_only(ui):
    page, state = ui
    open_tab(page, "Connections")
    page.get_by_role("button", name="List", exact=True).click()
    expect(page.get_by_text("Available tables (1)")).to_be_visible()
    page.get_by_role("button", name="Inspect", exact=True).click()
    expect(page.get_by_text("Erp.PartTran")).to_be_visible()
    expect(page.get_by_text("SysRowID", exact=True)).to_be_visible()
    source_calls = [c for c in state["calls"] if c["path"].startswith("/api/v1/source/")]
    assert source_calls and all(c["method"] == "GET" for c in source_calls)


def test_ui02_visual_steps_and_preview(ui):
    page, state = ui
    open_tab(page, "Transform Studio")
    page.get_by_role("button", name=re.compile("Lọc hàng")).click()
    expect(page.get_by_label("Bật tắt Lọc hàng")).to_be_visible()
    page.get_by_role("button", name="Preview bước").click()
    expect(page.get_by_text("Preview trả về 1 dòng", exact=False)).to_be_visible()
    assert any(c["path"] == "/api/v1/transform/preview" and c["method"] == "POST" for c in state["calls"])


def test_ui03_model_save_reload(ui):
    page, state = ui
    open_tab(page, "Transform Studio")
    page.get_by_role("button", name="Lưu Model").click()
    expect(page.get_by_text("Đã lưu model my_first_transform", exact=False)).to_be_visible()
    page.get_by_role("button", name="my_first_transform", exact=True).click()
    expect(page.get_by_text("Đã tải model my_first_transform", exact=False)).to_be_visible()
    assert any(m["name"] == "my_first_transform" for m in state["models"])


def test_ui04_lineage_node_and_impact(ui):
    page, state = ui
    open_tab(page, "Data Lineage")
    expect(page.get_by_text("Dependency Map")).to_be_visible()
    page.locator('button[title="m_c"]').click()
    expect(page.get_by_text("Model Inspector")).to_be_visible()
    assert any(c["path"] == "/api/v1/transform/lineage" for c in state["calls"])


def test_ui05_auto_build_action_and_order(ui):
    page, state = ui
    open_tab(page, "Data Lineage")
    page.locator('button[title="m_c"]').click()
    page.get_by_role("button", name=re.compile("Run Model \\+ Dependencies")).click()
    # Wait for the asynchronous React/API update before inspecting captured calls.
    expect(page.get_by_text("succeeded", exact=False).first).to_be_visible()
    assert any(c["path"] == "/api/v1/transform/auto-build" and c["method"] == "POST" for c in state["calls"])


def test_ui06_incremental_publish_and_rollback_buttons(ui):
    page, state = ui
    open_tab(page, "Incremental CDC")
    page.get_by_role("button", name=re.compile("Build Delta Staging")).click()
    expect(page.get_by_text("ready", exact=True)).to_be_visible()
    page.get_by_role("button", name=re.compile("Approve & Publish \\+ Cursor")).click()
    expect(page.get_by_role("dialog", name="Confirm incremental publish")).to_be_visible()
    page.get_by_role("button", name="Cancel").click()
    assert not any(c["path"] == "/api/v1/transform/incremental/action" for c in state["calls"])
    page.get_by_role("button", name=re.compile("Approve & Publish \\+ Cursor")).click()
    page.get_by_role("button", name="Confirm publish").click()
    expect(page.get_by_text("published", exact=True)).to_be_visible()
    page.get_by_role("button", name=re.compile("Rollback Data \\+ Cursor")).click()
    expect(page.get_by_role("dialog", name="Confirm incremental rollback")).to_be_visible()
    page.get_by_role("button", name="Confirm rollback").click()
    expect(page.get_by_text("rolled_back", exact=True)).to_be_visible()
    actions = [c["body"].get("confirm") for c in state["calls"]
               if c["path"] == "/api/v1/transform/incremental/action"]
    assert actions == ["PUBLISH", "ROLLBACK"]


def test_ui07_enterprise_overview_light_dark_persist_and_shortcuts(ui):
    page, state = ui
    expect(page.get_by_role("heading", name="One workspace for your data pipelines.")).to_be_visible()
    expect(page.get_by_text("Current browser session only")).to_be_visible()

    page.get_by_role("button", name="Switch to light mode").click()
    expect(page.locator("html")).to_have_class(re.compile("theme-light"))
    assert page.evaluate("localStorage.getItem('dataflow-theme')") == "light"
    page.reload(wait_until="domcontentloaded")
    expect(page.locator("html")).to_have_class(re.compile("theme-light"))
    page.get_by_role("button", name="Switch to dark mode").click()
    expect(page.locator("html")).to_have_class(re.compile("theme-dark"))

    page.get_by_role("button", name="Collapse sidebar").click()
    expect(page.get_by_role("button", name="Connections", exact=True)).to_be_visible()
    page.get_by_role("button", name="Connections", exact=True).click()
    expect(page.get_by_text("Connector Hub", exact=True)).to_be_visible()
    assert all(c["method"] == "GET" for c in state["calls"])


def test_ui08_recent_runs_filter_uses_loaded_rows_only(ui):
    page, state = ui
    page.get_by_role("button", name="Run Inspector", exact=True).click()
    page.locator("input[type=password]").first.fill(TOKEN)
    page.get_by_role("button", name="Refresh", exact=True).click()
    expect(page.get_by_text("Erp.PartTran", exact=True)).to_be_visible()
    expect(page.get_by_text("Erp.Part", exact=True)).to_be_visible()
    page.get_by_label("Search recent runs").fill("PartTran")
    expect(page.get_by_text("Erp.PartTran", exact=True)).to_be_visible()
    expect(page.get_by_text("Erp.Part", exact=True)).not_to_be_visible()
    page.get_by_label("Search recent runs").fill("")
    page.get_by_label("Filter run status").select_option("failed")
    expect(page.get_by_text("Erp.PartTran", exact=True)).to_be_visible()
    expect(page.get_by_text("Erp.Part", exact=True)).not_to_be_visible()
    assert len([c for c in state["calls"] if c["path"] == "/api/v1/bootstrap/history"]) == 1


def test_ui09_sql_dag_operations_read_only_inspector(ui):
    page, state = ui
    page.get_by_role("button", name="Operations Center", exact=True).click()
    expect(page.get_by_role("heading", name="Pipeline Operations Center")).to_be_visible()
    page.get_by_label("Operations API token").fill(TOKEN)
    page.get_by_role("button", name="Refresh runs").click()
    expect(page.get_by_text("Among retrieved SQL DAG runs only").first).to_be_visible()
    expect(page.get_by_text("revenue_reporting").first).to_be_visible()
    page.get_by_label("Filter pipeline status").select_option("failed")
    expect(page.get_by_text(re.compile("of 1 matching runs"))).to_be_visible()
    page.get_by_role("button", name="Inspect pipeline run 66666666-6666-4666-8666-666666666666").click()
    expect(page.get_by_label("Pipeline run inspector")).to_be_visible()
    expect(page.get_by_text("Quality gate failed")).to_be_visible()
    expect(page.get_by_text("Row count mismatch")).to_be_visible()
    graph = page.get_by_label("DAG execution map")
    expect(graph.get_by_text("revenue_check", exact=True)).to_be_visible()
    expect(graph.get_by_text("extract_invoice", exact=True).first).to_be_visible()
    expect(graph.get_by_text("Attempts: 3")).to_be_visible()
    expect(graph.get_by_text("Row count mismatch")).to_be_visible()
    operations_calls = [c for c in state["calls"] if c["path"].startswith("/api/v1/sql/")]
    assert operations_calls and all(c["method"] == "GET" for c in operations_calls)


def test_ui10_signalroom_design_tokens_in_both_themes(ui):
    page, state = ui
    expect(page.locator("html")).to_have_class(re.compile("theme-dark"))
    assert page.evaluate("getComputedStyle(document.documentElement).getPropertyValue('--console-accent').trim()") == "#c0f273"
    expect(page.get_by_text("SIGNAL / DATA OPS", exact=True)).to_be_visible()
    page.get_by_role("button", name="Operations Center", exact=True).click()
    page.get_by_label("Operations API token").fill(TOKEN)
    page.get_by_role("button", name="Refresh runs").click()
    page.get_by_role("button", name="Inspect pipeline run 66666666-6666-4666-8666-666666666666").click()
    expect(page.get_by_label("DAG execution map")).to_be_visible()
    expect(page.locator(".xyflow-signal-canvas .react-flow__node")).to_have_count(2)
    expect(page.locator(".xyflow-signal-canvas .react-flow__edge")).to_have_count(1)
    expect(page.locator(".xyflow-signal-canvas .react-flow__controls")).to_be_visible()
    assert page.locator(".xyflow-signal-canvas").evaluate(
        "(el) => getComputedStyle(el).backgroundColor !== 'rgba(0, 0, 0, 0)'"
    )
    page.get_by_role("button", name="Switch to light mode").click()
    expect(page.locator("html")).to_have_class(re.compile("theme-light"))
    assert page.evaluate("getComputedStyle(document.documentElement).getPropertyValue('--console-accent').trim()") == "#238047"
    assert page.evaluate("getComputedStyle(document.documentElement).getPropertyValue('--console-bg').trim()") == "#fafbfa"
    assert page.evaluate("getComputedStyle(document.documentElement).getPropertyValue('--console-card').trim()") == "#ffffff"
    assert page.evaluate("getComputedStyle(document.documentElement).getPropertyValue('--console-sidebar').trim()") == "#ffffff"
    assert page.evaluate("getComputedStyle(document.documentElement).getPropertyValue('--console-header').trim()") == "#ffffff"
    assert page.evaluate("getComputedStyle(document.documentElement).getPropertyValue('--console-accent').trim()") != "#245fc3"
    assert all(c["method"] == "GET" for c in state["calls"])


def test_ui11_connector_hub_catalog_and_route_preview_only(ui):
    page, state = ui
    page.get_by_role("button", name="Connections", exact=True).click()
    expect(page.get_by_text("Connector Hub", exact=True)).to_be_visible()
    expect(page.get_by_role("tab", name=re.compile("Sources"))).to_have_attribute("aria-selected", "true")

    source_catalog = page.get_by_label("Source catalog")
    source_catalog.get_by_label("Search connectors").fill("mysql")
    expect(source_catalog.get_by_text("MySQL / MariaDB", exact=True)).to_be_visible()
    expect(source_catalog.get_by_text("Microsoft SQL Server", exact=True)).not_to_be_visible()
    source_catalog.get_by_role("button", name="Clear connector search").click()
    expect(source_catalog.get_by_text("Microsoft SQL Server", exact=True)).to_be_visible()

    source_catalog.get_by_role("button", name="Planned", exact=True).click()
    expect(source_catalog.get_by_text("Microsoft SQL Server", exact=True)).not_to_be_visible()
    source_catalog.get_by_role("button", name="Available", exact=True).click()
    expect(source_catalog.get_by_text("Microsoft SQL Server", exact=True)).to_be_visible()

    page.get_by_role("tab", name=re.compile("Destinations")).click()
    expect(page.get_by_label("Destination catalog").get_by_text("ClickHouse", exact=True)).to_be_visible()
    page.get_by_role("tab", name="Connections", exact=True).click()
    expect(page.get_by_text("Supported underlying path", exact=False)).to_be_visible()
    page.get_by_label("Route source").select_option("mysql")
    expect(page.get_by_text("Planned route", exact=False)).to_be_visible()
    page.get_by_label("Route source").select_option("sqlserver")
    expect(page.get_by_text("Supported underlying path", exact=False)).to_be_visible()
    assert all(c["method"] == "GET" for c in state["calls"])


def test_ui12_shared_button_uses_semantic_light_dark_tokens(ui):
    page, state = ui
    action = page.get_by_role("button", name="Explore connections")
    expect(action).to_have_class(re.compile("df-btn-primary"))
    expect(action).to_have_css("background-color", "rgb(192, 242, 115)")
    page.get_by_role("button", name="Switch to light mode").click()
    expect(action).to_have_css("background-color", "rgb(35, 128, 71)")
    expect(page.get_by_role("button", name="Switch to dark mode")).to_be_visible()
    assert all(c["method"] == "GET" for c in state["calls"])


def test_ui13_sql_job_studio_editor_registry_search_and_light_theme(ui):
    page, state = ui
    page.get_by_role("button", name="SQL Jobs", exact=True).click()
    expect(page.get_by_role("heading", name="SQL Job Studio")).to_be_visible()
    page.get_by_label("SQL Jobs API token").fill(TOKEN)
    page.get_by_role("button", name="Refresh Jobs").click()
    expect(page.get_by_text("2 loaded jobs")).to_be_visible()
    page.get_by_label("Search SQL jobs").fill("inventory")
    expect(page.get_by_text("inventory_snapshot_check", exact=True)).to_be_visible()
    expect(page.get_by_text("revenue_quality_check", exact=True)).not_to_be_visible()
    page.get_by_label("Search SQL jobs").fill("")
    page.get_by_role("checkbox", name="Enabled only").check()
    expect(page.get_by_text("inventory_snapshot_check", exact=True)).not_to_be_visible()
    page.get_by_role("checkbox", name="Enabled only").uncheck()
    page.get_by_role("button", name=re.compile("inventory_snapshot_check")).click()
    expect(page.get_by_label("SQL job name")).to_have_value("inventory_snapshot_check")
    expect(page.get_by_label("SQL SELECT query")).to_have_value("SELECT 1")
    page.get_by_role("button", name="Switch to light mode").click()
    expect(page.locator(".df-studio-code-box")).to_be_visible()
    assert not any(c["method"] != "GET" for c in state["calls"])


def test_ui14_transform_development_workspace_retains_actions(ui):
    page, state = ui
    page.get_by_role("button", name="Transform Studio", exact=True).click()
    expect(page.locator(".df-transform-workspace")).to_be_visible()
    expect(page.locator(".df-studio-preview")).to_be_visible()
    expect(page.get_by_text("DEVELOPMENT / MODELS / 02")).to_be_visible()
    page.locator("input[type=password]").first.fill(TOKEN)
    page.get_by_role("button", name=re.compile("Lọc hàng")).click()
    expect(page.get_by_label("Bật tắt Lọc hàng")).to_be_visible()
    page.get_by_role("button", name="Preview bước").click()
    expect(page.get_by_text("Preview trả về 1 dòng", exact=False)).to_be_visible()
    assert any(c["path"] == "/api/v1/transform/preview" and c["method"] == "POST" for c in state["calls"])


def test_ui15_lineage_and_incremental_workspace_mounts(ui):
    page, state = ui
    page.get_by_role("button", name="Data Lineage", exact=True).click()
    expect(page.locator(".df-lineage-workspace")).to_be_visible()
    expect(page.get_by_text("DEVELOPMENT / LINEAGE / 03")).to_be_visible()
    page.get_by_role("button", name="Incremental CDC", exact=True).click()
    expect(page.locator(".df-incremental-workspace")).to_be_visible()
    expect(page.get_by_text("OPERATIONS / INCREMENTAL / 01")).to_be_visible()
    assert all(c["method"] == "GET" for c in state["calls"])


def test_ui16_all_frontend_screens_use_semantic_palette():
    """Keep freshly added screens from reverting to fixed Tailwind blue/slate styles."""
    root = Path(__file__).resolve().parents[2]
    source_files = [root / "web/src/App.tsx", *sorted((root / "web/src/components").rglob("*.tsx"))]
    forbidden = re.compile(
        r"(?<![\w-])(?:hover:|focus-visible:)?"
        r"(?:bg|text|border|ring|outline)-"
        r"(?:slate|sky|cyan|emerald|rose|red|amber|indigo|gray|blue|green)-[0-9]+"
    )
    offenses = {str(path.relative_to(root)): forbidden.findall(path.read_text(encoding="utf-8"))
                for path in source_files}
    offenses = {path: matches for path, matches in offenses.items() if matches}
    assert not offenses, f"Legacy palette utilities detected: {offenses}"
    assert not any("window.confirm(" in p.read_text(encoding="utf-8") for p in source_files)


def test_ui17_all_ten_navigation_screens_mount_in_light_mode(ui):
    page, state = ui
    page.get_by_role("button", name="Switch to light mode").click()
    screens = [
        ("Overview", "One workspace for your data pipelines."),
        ("Connections", "Connector Hub"),
        ("Sync Jobs", "Experimental full-load / rowversion-window trial"),
        ("Transform Studio", "Data Preview"),
        ("Data Lineage", "Data Lineage & Impact Analysis"),
        ("SQL Jobs", "Query editor"),
        ("Operations Center", "Pipeline Operations Center"),
        ("Pipelines", "Pipeline Definition (JSON)"),
        ("Incremental CDC", "Safe Incremental CDC"),
        ("Run Inspector", "Recent Runs"),
    ]
    for label, visible_text in screens:
        page.get_by_role("button", name=label, exact=True).click()
        expect(page.get_by_text(visible_text, exact=False).first).to_be_visible()
        assert page.evaluate(
            "getComputedStyle(document.documentElement).getPropertyValue('--console-card').trim()"
        ) == "#ffffff"
    assert not any(c["method"] in ("DELETE", "PUT", "PATCH") for c in state["calls"])

def test_ui18_sql_line_gutter_and_source_metadata_filters_are_local(ui):
    """Editing and filtering are browser-only; no ERP or backend mutations."""
    page, state = ui
    page.get_by_role("button", name="Connections", exact=True).click()
    page.get_by_label("Source API token").fill(TOKEN)
    page.get_by_role("button", name="List", exact=True).click()
    table_filter = page.get_by_label("Filter available tables")
    expect(table_filter).to_be_visible()
    table_filter.fill("not_a_real_table")
    expect(page.get_by_text("No tables match this filter.")).to_be_visible()
    table_filter.fill("PartTran")
    expect(page.get_by_role("button", name="PartTran", exact=True)).to_be_visible()
    page.get_by_role("button", name="Inspect", exact=True).click()
    column_filter = page.get_by_label("Filter inspected columns")
    expect(column_filter).to_be_visible()
    column_filter.fill("SysRevID")
    expect(page.get_by_text("SysRevID", exact=True)).to_be_visible()
    expect(page.get_by_text("SysRowID", exact=True)).not_to_be_visible()
    # Changing source identity must not show columns from the previous table.
    page.get_by_role("textbox", name="SQL Server table", exact=True).fill("NewTable")
    expect(page.get_by_label("Inspected SQL Server columns")).not_to_be_visible()
    page.get_by_label("SQL Server schema").fill("OtherSchema")
    expect(page.get_by_label("Inspected SQL Server columns")).not_to_be_visible()

    page.get_by_role("button", name="SQL Jobs", exact=True).click()
    query = page.get_by_label("SQL SELECT query")
    query.fill("SELECT 1\nFROM scratch.events")
    expect(page.locator(".df-studio-line-numbers > div")).to_have_count(2)
    expect(page.get_by_text("2 lines", exact=False)).to_be_visible()
    page.get_by_label("SQL Jobs API token").fill(TOKEN)
    page.get_by_role("button", name="Refresh Jobs").click()
    page.get_by_label("Search SQL jobs").fill("inventory")
    expect(page.get_by_role("button", name="Clear SQL job search")).to_be_visible()
    page.get_by_role("button", name="Clear SQL job search").click()
    expect(page.get_by_label("Search SQL jobs")).to_have_value("")
    # A long SQL query must scroll inside the editor, with a synchronized gutter.
    query.fill("\n".join(f"SELECT {i}" for i in range(60)))
    expect(page.locator(".df-studio-line-numbers > div")).to_have_count(60)
    scrolled = query.evaluate("""(el) => {
        el.scrollTop = 120;
        el.dispatchEvent(new Event("scroll", { bubbles: true }));
        return { top: el.scrollTop, viewport: el.clientHeight, content: el.scrollHeight };
    }""")
    assert scrolled["content"] > scrolled["viewport"], "SQL editor must have a vertical scrolling viewport"
    assert scrolled["top"] > 0, "SQL editor must accept vertical scrolling"
    gutter = page.locator(".df-studio-line-numbers")
    gutter_size = gutter.evaluate("(el) => ({ viewport: el.clientHeight, content: el.scrollHeight })")
    assert gutter_size["content"] > gutter_size["viewport"], "Gutter must have a bounded scrollable viewport"
    assert gutter_size["viewport"] == scrolled["viewport"], "Gutter and SQL editor viewport heights must match"
    expect(gutter).to_have_js_property("scrollTop", scrolled["top"])
    assert all(c["method"] == "GET" for c in state["calls"])


def test_ui19_visual_pipeline_builder_edits_draft_and_validates_without_execution(ui):
    """Visual canvas and JSON stay in sync; runtime endpoints must not be invoked."""
    page, state = ui
    page.get_by_role("button", name="Pipelines", exact=True).click()
    expect(page.get_by_label("Visual pipeline builder")).to_be_visible()
    expect(page.get_by_label("Editable dependency canvas")).to_be_visible()
    json_editor = page.get_by_label("Pipeline definition JSON")
    assert json.loads(json_editor.input_value()) == {"nodes": []}

    page.get_by_label("Pipeline API token").fill(TOKEN)
    page.get_by_role("button", name="Load registered SQL jobs").click()
    expect(page.get_by_role("option", name="revenue_quality_check")).to_have_count(1)
    page.get_by_role("combobox", name="Registered SQL Job", exact=True).select_option("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
    page.get_by_role("button", name="Add SQL job").click()
    first = json.loads(json_editor.input_value())["nodes"]
    assert len(first) == 1 and first[0]["job_id"] == "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
    assert first[0]["requires"] == []

    page.get_by_role("combobox", name="Registered SQL Job", exact=True).select_option("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
    page.get_by_role("button", name="Add SQL job").click()
    nodes = json.loads(json_editor.input_value())["nodes"]
    assert len(nodes) == 2 and nodes[0]["id"] != nodes[1]["id"]
    assert nodes[1]["job_id"] == "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
    assert page.locator(".df-pipeline-graph-node").count() == 2

    page.get_by_label("Inspect pipeline node").select_option(nodes[1]["id"])
    page.get_by_label("Dependency source").select_option(nodes[0]["id"])
    page.get_by_role("button", name="Connect dependency").click()
    assert json.loads(json_editor.input_value())["nodes"][1]["requires"] == [nodes[0]["id"]]
    expect(page.locator(".df-pipeline-edit-canvas .react-flow__edge")).to_have_count(1)

    # A backwards dependency would introduce a cycle, which the UI must reject.
    page.get_by_label("Inspect pipeline node").select_option(nodes[0]["id"])
    page.get_by_label("Dependency source").select_option(nodes[1]["id"])
    page.get_by_role("button", name="Connect dependency").click()
    expect(page.get_by_text("Connection rejected: it would introduce a dependency cycle.")).to_be_visible()
    assert json.loads(json_editor.input_value())["nodes"][0]["requires"] == []

    page.get_by_role("button", name="Remove node").click()
    remaining = json.loads(json_editor.input_value())["nodes"]
    assert len(remaining) == 1 and remaining[0]["requires"] == []
    expect(page.locator(".df-pipeline-graph-node")).to_have_count(1)

    # Manual JSON edits are bidirectional; malformed JSON never becomes an API request.
    json_editor.fill('{"nodes": [')
    expect(page.get_by_text("Canvas paused:", exact=False)).to_be_visible()
    expect(page.get_by_role("button", name="Validate DAG")).to_be_disabled()
    json_editor.fill('{"nodes": []}')
    expect(page.get_by_label("Editable dependency canvas")).to_be_visible()
    page.get_by_role("button", name="Validate DAG").click()
    expect(page.get_by_text("DAG valid", exact=True)).to_be_visible()

    assert any(c["path"] == "/api/v1/sql/jobs" and c["method"] == "GET" for c in state["calls"])
    validation = [c for c in state["calls"] if c["path"] == "/api/v1/pipelines/validate"]
    assert len(validation) == 1 and validation[0]["body"] == {"nodes": []}
    assert all(c["path"] not in ("/api/v1/sql/dags", "/api/v1/sql/dag-runs") for c in state["calls"])
    assert not any(c["method"] in ("PUT", "PATCH", "DELETE") for c in state["calls"])


def test_ui20_pipeline_drag_drop_responsive_light_dark(ui):
    """Use real browser HTML5 drag/drop and inspect canvas at mobile/tablet/desktop."""
    page, state = ui
    page.get_by_role("button", name="Pipelines", exact=True).click()
    page.get_by_label("Pipeline API token").fill(TOKEN)
    page.get_by_role("button", name="Load registered SQL jobs").click()

    palette_job = page.get_by_label("Drag SQL job revenue_quality_check", exact=True)
    canvas = page.get_by_label("Editable dependency canvas", exact=True)
    expect(palette_job).to_have_attribute("draggable", "true")
    palette_job.drag_to(canvas, target_position={"x": 360, "y": 175})
    expect(page.locator(".df-pipeline-graph-node")).to_have_count(1)
    assert json.loads(page.get_by_label("Pipeline definition JSON").input_value())["nodes"][0]["job_id"] == "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

    for width, height in [(1440, 900), (1366, 768), (390, 844)]:
        page.set_viewport_size({"width": width, "height": height})
        expect(canvas).to_be_visible()
        expect(page.locator(".df-pipeline-graph-node")).to_have_count(1)
        measurements = page.evaluate("""() => ({
            documentWidth: document.documentElement.scrollWidth,
            viewportWidth: document.documentElement.clientWidth,
            canvasWidth: document.querySelector(".df-pipeline-edit-canvas").clientWidth,
            canvasHeight: document.querySelector(".df-pipeline-edit-canvas").clientHeight
        })""")
        assert measurements["documentWidth"] <= measurements["viewportWidth"], measurements
        assert measurements["canvasWidth"] > 0 and measurements["canvasHeight"] >= 400, measurements

    page.get_by_role("button", name="Switch to light mode").click()
    expect(canvas).to_be_visible()
    assert page.evaluate("getComputedStyle(document.documentElement).getPropertyValue('--console-card').trim()") == "#ffffff"
    page.get_by_role("button", name="Switch to dark mode").click()
    expect(canvas).to_be_visible()
    assert page.evaluate("getComputedStyle(document.documentElement).getPropertyValue('--console-card').trim()") != "#ffffff"
    assert all(c["method"] == "GET" for c in state["calls"] if c["path"].startswith("/api/v1/sql/"))


def test_ui21_pipeline_save_requires_validation_and_confirms_overwrite(ui):
    """Saved DAG uses existing API only; no execution; overwrite requires confirmation."""
    page, state = ui
    page.get_by_role("button", name="Pipelines", exact=True).click()
    save = page.get_by_role("button", name="Save pipeline", exact=True)
    expect(save).to_be_disabled()
    page.get_by_label("Pipeline API token").fill(TOKEN)
    page.get_by_role("button", name="Load registered SQL jobs").click()
    page.get_by_role("combobox", name="Registered SQL Job", exact=True).select_option(
        "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
    page.get_by_role("button", name="Add SQL job").click()
    page.get_by_label("Pipeline save workspace").get_by_role("textbox", name="Pipeline name", exact=True).fill("qa_visual_dag")
    expect(save).to_be_disabled()
    page.get_by_role("button", name="Validate DAG").click()
    expect(page.get_by_text("DAG valid", exact=True)).to_be_visible()
    expect(save).to_be_enabled()
    save.click()
    expect(page.get_by_text("Saved pipeline qa_visual_dag", exact=False)).to_be_visible()
    writes = [c for c in state["calls"] if c["path"] == "/api/v1/sql/dags" and c["method"] == "POST"]
    assert len(writes) == 1
    payload = writes[0]["body"]
    assert payload["name"] == "qa_visual_dag" and payload["enabled"] is False
    assert len(payload["nodes"]) == 1
    assert payload["nodes"][0]["job_id"] == "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

    save.click()
    expect(page.get_by_role("dialog", name="Overwrite saved pipeline?")).to_be_visible()
    page.get_by_role("button", name="Cancel", exact=True).click()
    assert len([c for c in state["calls"] if c["path"] == "/api/v1/sql/dags" and c["method"] == "POST"]) == 1
    save.click()
    page.get_by_role("button", name="Overwrite & save").click()
    expect(page.get_by_text("Saved pipeline qa_visual_dag", exact=False)).to_be_visible()
    assert len([c for c in state["calls"] if c["path"] == "/api/v1/sql/dags" and c["method"] == "POST"]) == 2

    page.get_by_role("button", name="Load saved DAGs").click()
    page.get_by_role("combobox", name="Saved pipeline", exact=True).select_option(
        "99999999-9999-4999-8999-999999999999")
    page.get_by_role("button", name="Load into editor").click()
    expect(save).to_be_disabled()
    assert page.get_by_label("Pipeline save workspace").get_by_role("textbox", name="Pipeline name", exact=True).input_value() == "qa_visual_dag"

    page.get_by_label("Pipeline definition JSON").fill('{"nodes": [')
    expect(save).to_be_disabled()
    assert all(c["path"] not in ("/api/v1/sql/dag-runs", "/api/v1/bootstrap/runs") for c in state["calls"])


def test_ui22_pipeline_run_confirms_and_inspects_real_mocked_backend_status(ui):
    """Explicit TEST confirmation before starting DAG; show only server results."""
    page, state = ui
    page.get_by_role("button", name="Pipelines", exact=True).click()
    page.get_by_label("Pipeline API token").fill(TOKEN)
    expect(page.get_by_label("Pipeline run inspector")).to_be_visible()
    run_button = page.get_by_role("button", name="Run selected DAG", exact=True)
    expect(run_button).to_have_count(0)

    page.get_by_role("button", name="Load saved DAGs").click()
    page.get_by_role("combobox", name="Saved pipeline", exact=True).select_option(
        "33333333-3333-4333-8333-333333333333")
    expect(run_button).to_be_enabled()
    run_button.click()
    expect(page.get_by_role("dialog", name="Run saved SQL DAG in TEST?")).to_be_visible()
    page.get_by_role("button", name="Cancel", exact=True).click()
    assert not any(c["method"] == "POST" and c["path"] == "/api/v1/sql/dag-runs" for c in state["calls"])

    run_button.click()
    page.get_by_role("button", name="Confirm TEST run").click()
    expect(page.get_by_text("Status: queued", exact=True)).to_be_visible()
    started = [c for c in state["calls"] if c["method"] == "POST" and c["path"] == "/api/v1/sql/dag-runs"]
    assert len(started) == 1 and started[0]["body"] == {
        "dag_id": "33333333-3333-4333-8333-333333333333"
    }

    page.get_by_role("button", name="Refresh run").click()
    expect(page.get_by_text("Status: succeeded", exact=True)).to_be_visible()
    expect(page.get_by_label("DAG execution map")).to_be_visible()
    expect(page.locator(".xyflow-signal-node")).to_have_count(2)
    expect(page.get_by_text("Attempts", exact=True)).to_be_visible()
    assert any(c["method"] == "GET" and c["path"] == "/api/v1/sql/dag-runs" for c in state["calls"])
    assert not any(c["method"] == "POST" and c["path"] in (
        "/api/v1/bootstrap/runs", "/api/v1/managed/runs") for c in state["calls"])


def test_ui23_run_directly_from_validated_saved_canvas(ui):
    """Run canvas is a direct entrypoint, but never executes an unsaved draft."""
    page, state = ui
    page.get_by_role("button", name="Pipelines", exact=True).click()
    button = page.get_by_role("button", name="Run canvas TEST")
    expect(button).to_be_disabled()
    page.get_by_label("Pipeline API token").fill(TOKEN)
    page.get_by_role("button", name="Load saved DAGs").click()
    page.get_by_role("combobox", name="Saved pipeline", exact=True).select_option(
        "33333333-3333-4333-8333-333333333333")
    page.get_by_role("button", name="Load into editor").click()
    expect(button).to_be_disabled()
    expect(page.get_by_text("Validate this canvas before running.")).to_be_visible()

    page.get_by_role("button", name="Validate DAG").click()
    expect(page.get_by_text("DAG valid", exact=True)).to_be_visible()
    expect(button).to_be_enabled()
    button.click()
    expect(page.get_by_role("dialog", name="Run saved SQL DAG in TEST?")).to_be_visible()
    page.get_by_role("button", name="Cancel", exact=True).click()
    assert not any(c["method"] == "POST" and c["path"] == "/api/v1/sql/dag-runs" for c in state["calls"])

    button.click()
    page.get_by_role("button", name="Confirm TEST run").click()
    expect(page.get_by_text("Status: queued", exact=True)).to_be_visible()
    run_posts = [c for c in state["calls"] if c["method"] == "POST" and c["path"] == "/api/v1/sql/dag-runs"]
    assert len(run_posts) == 1
    assert run_posts[0]["body"] == {"dag_id": "33333333-3333-4333-8333-333333333333"}
    page.get_by_role("button", name="Refresh run").click()
    expect(page.get_by_text("Status: succeeded", exact=True)).to_be_visible()
    expect(page.get_by_label("DAG execution map")).to_be_visible()
    page.get_by_label("Pipeline definition JSON").fill('{"nodes": []}')
    expect(button).to_be_disabled()
    assert len([c for c in state["calls"] if c["method"] == "POST" and c["path"] == "/api/v1/sql/dag-runs"]) == 1


def test_ui24_canvas_run_rechecks_server_before_execution(ui):
    """New enabled DAG can be run on canvas, but altered server definition fails closed."""
    page, state = ui
    page.get_by_role("button", name="Pipelines", exact=True).click()
    page.get_by_label("Pipeline API token").fill(TOKEN)
    page.get_by_role("button", name="Load registered SQL jobs").click()
    page.get_by_role("combobox", name="Registered SQL Job", exact=True).select_option(
        "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
    page.get_by_role("button", name="Add SQL job").click()
    name = page.get_by_label("Pipeline save workspace").get_by_role("textbox", name="Pipeline name", exact=True)
    name.fill("qa_canvas_direct")
    page.get_by_label("Enable TEST execution", exact=True).check()
    page.get_by_role("button", name="Validate DAG").click()
    expect(page.get_by_text("DAG valid", exact=True)).to_be_visible()
    page.get_by_role("button", name="Save pipeline", exact=True).click()
    expect(page.get_by_role("dialog", name="Enable TEST pipeline?")).to_be_visible()
    page.get_by_role("button", name="Enable & save", exact=True).click()
    expect(page.get_by_text("Saved pipeline qa_canvas_direct", exact=False)).to_be_visible()
    canvas_run = page.get_by_role("button", name="Run canvas TEST")
    expect(canvas_run).to_be_enabled()
    canvas_run.click()
    expect(page.get_by_role("dialog", name="Run saved SQL DAG in TEST?")).to_be_visible()

    # Simulate a concurrent server-side disable between confirmation display and submit.
    for dag in state["pipelines"]:
        if dag["name"] == "qa_canvas_direct":
            dag["enabled"] = False
    page.get_by_role("button", name="Confirm TEST run").click()
    expect(page.get_by_text("Saved DAG changed or was disabled before Run.", exact=False)).to_be_visible()
    assert not any(c["method"] == "POST" and c["path"] == "/api/v1/sql/dag-runs" for c in state["calls"])
    assert any(c["method"] == "GET" and c["path"] == "/api/v1/sql/dags" for c in state["calls"])



def test_ui25_connection_inventory_requires_token_and_never_exposes_secrets(ui):
    """Authenticated GET-only configuration inventory; no live network test or credential leak."""
    page, state = ui
    page.get_by_role("button", name="Connections", exact=True).click()
    page.get_by_role("tab", name="Connections", exact=True).click()
    inventory = page.get_by_label("Configured connection inventory")
    expect(inventory).to_be_visible()
    load = page.get_by_role("button", name="Refresh configured connections")
    expect(load).to_be_disabled()
    assert not any(c["path"] == "/api/v1/connections" for c in state["calls"])

    token = page.get_by_label("Lab API token (shared across modules)")
    token.fill(TOKEN)
    load.click()
    expect(page.get_by_label("Connection env-sqlserver-source")).to_be_visible()
    expect(page.get_by_label("Connection env-clickhouse-target")).to_be_visible()
    expect(page.get_by_label("Connection env-postgres-control")).to_be_visible()
    expect(inventory.get_by_text("3 server slots listed", exact=False)).to_be_visible()
    assert inventory.get_by_text("Configured · not tested", exact=True).count() == 3
    assert all(c["method"] == "GET" and c["body"] == {} for c in state["calls"]
               if c["path"] == "/api/v1/connections")
    assert len([c for c in state["calls"] if c["path"] == "/api/v1/connections"]) == 1
    rendered = inventory.inner_text().lower()
    for sensitive in [TOKEN.lower(), "sqlserver://", "postgres://", "password", "private-host", "admin@"]:
        assert sensitive not in rendered

    # A changed token must never leave the last authenticated inventory visible.
    token.fill("another-synthetic-token")
    expect(page.get_by_label("Connection env-sqlserver-source")).to_have_count(0)
    assert not any(c["path"] == "/api/v1/connections" and c["method"] != "GET"
                   for c in state["calls"])


def test_ui26_connection_draft_registry_admin_only_metadata(ui):
    """Draft registry persists names only; it cannot dial, bind or execute."""
    page, state = ui
    page.get_by_role("button", name="Connections", exact=True).click()
    page.get_by_role("tab", name="Connections", exact=True).click()
    panel = page.get_by_label("Connection draft registry", exact=True)
    expect(panel).to_be_visible()
    admin = page.get_by_label("Connection registry admin token")
    load = page.get_by_role("button", name="Load drafts", exact=True)
    expect(load).to_be_disabled()
    page.get_by_label("Lab API token (shared across modules)").fill(TOKEN)
    load.click()
    expect(panel.get_by_text("Registry create is disabled.", exact=False)).to_be_visible()
    expect(page.get_by_role("button", name="Create draft")).to_be_disabled()
    assert not any(c["path"] == "/api/v1/connections/definitions" and c["method"] == "POST"
                   for c in state["calls"])
    admin.fill("synthetic-admin-token")
    load.click()
    expect(panel.get_by_text("No draft connection definitions in the registry.")).to_be_visible()
    page.get_by_label("Draft connection name").fill("QA Finance Source")
    page.get_by_label("Draft connection type").select_option("sqlserver")
    page.get_by_role("button", name="Create draft").click()
    expect(page.get_by_label("Draft definition QA Finance Source")).to_be_visible()
    expect(panel.get_by_text("Created inactive draft:", exact=False)).to_be_visible()
    created_calls = [c for c in state["calls"]
                     if c["path"] == "/api/v1/connections/definitions" and c["method"] == "POST"]
    assert created_calls == [{"path": "/api/v1/connections/definitions", "method": "POST",
                              "body": {"name": "QA Finance Source",
                                       "driver": "sqlserver", "kind": "source"}}]
    admin.fill("changed-admin-token")
    expect(page.get_by_label("Draft definition QA Finance Source")).to_have_count(0)
    assert all(c["path"] != "/api/v1/bootstrap/runs" for c in state["calls"])
    assert all(c["path"] != "/api/v1/sql/dag-runs" for c in state["calls"])
    assert not any(k in str(c["body"]).lower() for c in created_calls
                   for k in ["password", "dsn", "host", "endpoint", "token", "secret"])


def test_ui27_core_real_api_panel_wires_run_and_backup_gated_rollback(ui):
    """Mocks prove UI contract only; real four-service execution is covered separately in hosted SQL QA."""
    page, state = ui
    open_tab(page, "Pipelines")
    panel = page.get_by_label("Durable core pipeline operator")
    expect(panel).to_be_visible()
    expect(panel.get_by_role("button", name="Run complete core pipeline")).to_be_disabled()
    panel.get_by_role("button", name="Load saved streams and models").click()
    expect(panel.get_by_label("Core source stream")).to_have_value("cccccccc-cccc-4ccc-8ccc-cccccccccccc")
    panel.get_by_label("Core final model").select_option("qa_model_c")
    panel.get_by_label("Core final output unique key").fill("stream")
    assert not any(c["path"] == "/api/v1/core/runs" for c in state["calls"])

    panel.get_by_role("button", name="Run complete core pipeline").click()
    expect(page.get_by_role("dialog", name="Start real core execution in disposable QA?")).to_be_visible()
    assert not any(c["path"] == "/api/v1/core/runs" and c["method"] == "POST"
                   for c in state["calls"])
    page.get_by_role("button", name="Start CORE Run").click()
    expect(panel.get_by_text("Published rows:")).to_be_visible()
    expect(panel.get_by_text("10,000", exact=True)).to_be_visible()
    expect(panel.get_by_role("button", name="Rollback to previous physical version")).to_have_count(0)
    starts = [c for c in state["calls"] if c["path"] == "/api/v1/core/runs" and c["method"] == "POST"]
    assert len(starts) == 1
    assert starts[0]["body"] == {
        "stream_id": "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
        "model_id": "qa_model_c",
        "unique_column": "stream",
    }

    # A later real backend publication can expose a prior physical backup,
    # but the action remains an explicit, user-confirmed POST.
    panel.get_by_role("button", name="Run complete core pipeline").click()
    page.get_by_role("button", name="Start CORE Run").click()
    rollback = panel.get_by_role("button", name="Rollback to previous physical version")
    expect(rollback).to_be_visible()
    rollback.click()
    expect(page.get_by_role("dialog", name="Restore previous scratch table version?")).to_be_visible()
    assert not any(c["path"] == "/api/v1/core/actions" for c in state["calls"])
    page.get_by_role("button", name="Confirm scratch rollback").click()
    expect(panel.get_by_text("Rollback acknowledged by PostgreSQL. Checkpoint unchanged.")).to_be_visible()
    actions = [c for c in state["calls"] if c["path"] == "/api/v1/core/actions"]
    assert len(actions) == 1
    assert actions[0]["method"] == "POST"
    assert actions[0]["body"] == {
        "run_id": "f0000002-1111-4111-8111-111111111111",
        "action": "rollback", "confirm": "ROLLBACK",
    }
    expect(panel.get_by_role("button", name="Rollback to previous physical version")).to_have_count(0)
    panel.get_by_label("Core API bearer token").fill("other-qa-token")
    expect(panel.get_by_label("Core run UUID")).to_have_value("")
    assert all(c["path"] != "/api/v1/bootstrap/runs" for c in state["calls"])


def test_ui_runtime_connection_add_test_save_bind_uses_backend_and_hides_credentials(ui):
    """UI contract only: real driver and encrypted PG acceptance is separate."""
    page, state = ui
    page.get_by_label("Lab API token (shared across modules)").fill(TOKEN)
    page.get_by_role("button", name="Connections", exact=True).click()
    panel = page.get_by_label("Live lab connection manager")
    panel.get_by_label("Live connection admin credential").fill("synthetic-admin-token")
    panel.get_by_role("button", name="Load connections").click()
    expect(panel.get_by_text("No saved connections.")).to_be_visible()

    for driver, name, host, port, database, username in [
        ("sqlserver", "QA SQL Profile", "qa-sql", "1433", "DataFlowFixtureQA", "qa_erp_reader"),
        ("clickhouse", "QA CH Profile", "qa-ch", "8123", "scratch", "qa_writer"),
    ]:
        panel.get_by_label("Live connection driver").select_option(driver)
        panel.get_by_label("Live connection name").fill(name)
        panel.get_by_label("Live connection host").fill(host)
        panel.get_by_label("Live connection port").fill(port)
        panel.get_by_label("Live connection database").fill(database)
        panel.get_by_label("Live connection username").fill(username)
        panel.get_by_label("Live connection password").fill("synthetic-credential-do-not-log")
        panel.get_by_role("button", name="Save encrypted draft").click()
        expect(panel.get_by_text(name, exact=True)).to_be_visible()
        expect(panel.get_by_label("Live connection password")).to_have_value("")

    assert "synthetic-credential-do-not-log" not in panel.inner_text()
    buttons = panel.get_by_role("button", name="Test real connection")
    buttons.nth(0).click()
    expect(panel.get_by_text("Verified", exact=True)).to_have_count(1)
    buttons.nth(1).click()
    expect(panel.get_by_text("Verified", exact=True)).to_have_count(2)
    panel.get_by_label("Runtime bind stream").select_option(
        "cccccccc-cccc-4ccc-8ccc-cccccccccccc")
    panel.get_by_label("Runtime bind source").select_option(
        "b0000000-0000-4000-8000-000000000001")
    panel.get_by_label("Runtime bind target").select_option(
        "b0000000-0000-4000-8000-000000000002")
    panel.get_by_role("button", name="Bind to pipeline").click()
    expect(panel.get_by_text("Verified connection pair bound", exact=False)).to_be_visible()
    calls = [x["path"] for x in state["calls"] if x["path"].startswith("/api/v1/connections/runtime")]
    assert calls.count("/api/v1/connections/runtime") >= 3
    assert calls.count("/api/v1/connections/runtime/test") == 2
    assert calls.count("/api/v1/connections/runtime/bind") == 1


def test_ui_local_session_login_roles_and_logout(ui):
    """UI-only check: real credential/session RBAC is covered by Go tests."""
    page, state = ui
    signed = {"role": None}

    def session_api(route):
        method = route.request.method
        if method == "GET":
            payload = {"authenticated": signed["role"] is not None, "role": signed["role"]}
            route.fulfill(status=200, content_type="application/json", body=json.dumps(payload))
        elif method == "POST":
            data = json.loads(route.request.post_data or "{}")
            role = data.get("username")
            if (role, data.get("password")) not in {
                ("operator", "synthetic-operator-password"),
                ("admin", "synthetic-admin-password"),
            }:
                route.fulfill(status=401, body="invalid credentials")
                return
            signed["role"] = role
            route.fulfill(status=200, content_type="application/json",
                          body=json.dumps({"authenticated": True, "role": role}))
        else:
            signed["role"] = None
            route.fulfill(status=204, body="")

    page.route("**/api/v1/auth/session", session_api)
    # In session mode the backend authorizes the signed-in Administrator;
    # the global legacy-token stub above deliberately requires a distinct admin token.
    def session_runtime_api(route):
        state["calls"].append({"method": route.request.method,
                               "path": "/api/v1/connections/runtime", "body": {}})
        if signed["role"] != "admin":
            route.fulfill(status=403, body="admin permission required")
            return
        route.fulfill(status=200, content_type="application/json",
                      body=json.dumps({"profiles": []}))

    page.route("**/api/v1/connections/runtime", session_runtime_api)
    page.reload(wait_until="domcontentloaded")
    expect(page.get_by_label("DataFlow sign in")).to_be_visible()
    expect(page.get_by_label("Lab API token (shared across modules)")).to_have_count(0)
    expect(page.get_by_label("Account")).to_have_count(0)

    page.get_by_label("Username").fill("admin")
    page.get_by_label("Sign in password").fill("incorrect-password")
    page.get_by_role("button", name="Sign in", exact=True).click()
    expect(page.get_by_label("DataFlow sign in")).to_be_visible()
    expect(page.get_by_role("alert")).to_contain_text("Incorrect login")

    page.get_by_label("Username").fill("operator")
    page.get_by_label("Sign in password").fill("synthetic-operator-password")
    page.get_by_role("button", name="Sign in", exact=True).click()
    page.get_by_role("button", name="Connections", exact=True).click()
    expect(page.get_by_label("Live lab connection manager")).to_have_count(0)
    page.get_by_role("tab", name="Connections", exact=True).click()
    expect(page.get_by_label("Saved connections access")).to_contain_text("Administrator access")
    assert not any(c["path"] == "/api/v1/connections/runtime" for c in state["calls"])
    page.get_by_role("button", name="Sign out").click()
    expect(page.get_by_label("DataFlow sign in")).to_be_visible()

    page.get_by_label("Username").fill("admin")
    page.get_by_label("Sign in password").fill("synthetic-admin-password")
    page.get_by_role("button", name="Sign in", exact=True).click()
    page.get_by_role("button", name="Connections", exact=True).click()
    expect(page.get_by_label("Live lab connection manager")).to_be_visible()
    expect(page.get_by_label("Live connection admin credential")).to_have_count(0)
    expect(page.get_by_label("Lab API token (shared across modules)")).to_have_count(0)
    expect(page.get_by_label("Saved QA profiles")).to_be_visible()
    expect(page.get_by_label("Saved QA profiles").get_by_text("No saved connections.")).to_be_visible()
    assert any(c["path"] == "/api/v1/connections/runtime" and c["method"] == "GET" for c in state["calls"])


def test_ui_airbyte_stream_columns_are_selected_and_saved(ui):
    """A stream column selection must reach the API; read-only metadata GET is distinct."""
    page, state = ui

    def columns(route):
        route.fulfill(status=200, content_type="application/json", body=json.dumps([
            {"name": name, "sql_type": sql_type, "max_length": 0,
             "precision": 0, "scale": 0, "nullable": False,
             "identity": False, "primary_key": name == "SysRowID",
             "cursor_candidate": name == "SysRevID"}
            for name, sql_type in [
                ("SysRowID", "uniqueidentifier"), ("SysRevID", "timestamp"),
                ("PartNum", "nvarchar"), ("TranQty", "decimal")]
        ]))

    page.route("**/api/v1/source/columns*", columns)
    open_tab(page, "Sync Jobs")
    panel = page.get_by_label("Airbyte-style source discovery and field projection")
    save = page.get_by_role("button", name="Save selected stream")
    expect(save).to_be_disabled()
    panel.get_by_role("button", name="Discover tables").click()
    panel.get_by_label("Discovered source table").select_option("PartTran")
    panel.get_by_role("button", name="Inspect columns").click()
    expect(panel.get_by_label("Sync column SysRowID")).to_be_disabled()
    expect(panel.get_by_label("Sync column SysRevID")).to_be_disabled()
    panel.get_by_label("Sync column TranQty").uncheck()
    expect(save).to_be_enabled()
    save.click()
    posts = [call for call in state["calls"]
             if call["path"] == "/api/v1/managed/streams" and call["method"] == "POST"]
    assert len(posts) == 1
    assert posts[0]["body"]["source_table"] == "PartTran"
    assert posts[0]["body"]["selected_columns"] == ["SysRowID", "SysRevID", "PartNum"]
    assert all(call["method"] == "GET" for call in state["calls"]
               if call["path"].startswith("/api/v1/source/"))


def test_qa_sync_setup_uses_verified_profiles_to_discover_save_bind_and_run(ui):
    """Browser contract: selected profile scopes discovery and source/target refs persist."""
    page, state = ui
    page.get_by_label("Lab API token (shared across modules)").fill(TOKEN)
    page.get_by_role("button", name="Connections", exact=True).click()
    panel = page.get_by_label("Live lab connection manager")
    panel.get_by_label("Live connection admin credential").fill("synthetic-admin-token")
    panel.get_by_role("button", name="Load connections").click()

    for driver,name,host,port,db,user in [
        ("sqlserver","Wizard SQL","qa-sql","1433","DataFlowFixtureQA","qa_erp_reader"),
        ("clickhouse","Wizard CH","qa-ch","8123","scratch","qa_writer"),
    ]:
        panel.get_by_label("Live connection driver").select_option(driver)
        panel.get_by_label("Live connection name").fill(name)
        panel.get_by_label("Live connection host").fill(host)
        panel.get_by_label("Live connection port").fill(port)
        panel.get_by_label("Live connection database").fill(db)
        panel.get_by_label("Live connection username").fill(user)
        panel.get_by_label("Live connection password").fill("synthetic-not-secret")
        panel.get_by_role("button", name="Save encrypted draft").click()
    buttons=panel.get_by_role("button", name="Test real connection")
    buttons.nth(0).click()
    buttons.nth(1).click()
    wizard=panel.get_by_label("QA sync setup wizard")
    wizard.get_by_label("Wizard source connection").select_option(
        "b0000000-0000-4000-8000-000000000001")
    wizard.get_by_label("Wizard target connection").select_option(
        "b0000000-0000-4000-8000-000000000002")
    wizard.get_by_role("button", name="Discover", exact=True).click()
    wizard.get_by_label("Wizard source table").select_option("Part")
    wizard.get_by_role("button", name="Columns", exact=True).click()
    expect(wizard.get_by_label("Wizard column SysRevID")).to_be_disabled()
    expect(wizard.get_by_label("Wizard column SysRowID")).to_be_disabled()
    wizard.get_by_label("Wizard column TranQty").uncheck()
    wizard.get_by_role("button", name="Save stream + Bind").click()
    expect(wizard.get_by_text("Saved stream ID", exact=False)).to_be_visible()
    posts=[c for c in state["calls"] if c["path"]=="/api/v1/connections/runtime/streams"]
    assert len(posts)==1
    assert posts[0]["body"]["source_connection_id"]=="b0000000-0000-4000-8000-000000000001"
    assert posts[0]["body"]["target_connection_id"]=="b0000000-0000-4000-8000-000000000002"
    assert posts[0]["body"]["selected_columns"]==["SysRowID","SysRevID","PartNum"]
    assert posts[0]["body"]["mode"]=="incremental_rowversion"
    wizard.get_by_role("button", name="Run selected stream").click()
    assert any(c["path"]=="/api/v1/managed/runs" and c["method"]=="POST"
               and c["body"]["stream_id"]=="dddddddd-dddd-4ddd-8ddd-dddddddddddd"
               for c in state["calls"])


def test_qa_batch_full_refresh_custom_dest_and_multistream_save(ui):
    page,state=ui
    page.get_by_label("Lab API token (shared across modules)").fill(TOKEN)
    page.get_by_role("button",name="Connections",exact=True).click()
    panel=page.get_by_label("Live lab connection manager")
    panel.get_by_label("Live connection admin credential").fill("synthetic-admin-token")
    panel.get_by_role("button",name="Load connections").click()
    for driver,name,host,port,db,user in [
        ("sqlserver","Full source","qa-sql","1433","DataFlowFixtureQA","qa_erp_reader"),
        ("clickhouse","Full dest","qa-ch","8123","scratch","qa_writer"),
    ]:
        panel.get_by_label("Live connection driver").select_option(driver)
        panel.get_by_label("Live connection name").fill(name)
        panel.get_by_label("Live connection host").fill(host)
        panel.get_by_label("Live connection port").fill(port)
        panel.get_by_label("Live connection database").fill(db)
        panel.get_by_label("Live connection username").fill(user)
        panel.get_by_label("Live connection password").fill("synthetic-qa-secret")
        panel.get_by_role("button",name="Save encrypted draft").click()
    buttons=panel.get_by_role("button",name="Test real connection")
    buttons.nth(0).click()
    buttons.nth(1).click()
    wizard=panel.get_by_label("Batch sync setup wizard")
    wizard.get_by_label("Batch source connection").select_option("b0000000-0000-4000-8000-000000000001")
    wizard.get_by_label("Batch target connection").select_option("b0000000-0000-4000-8000-000000000002")
    wizard.get_by_role("button",name="Discover").click()
    wizard.get_by_label("Batch source table").select_option("Part")
    wizard.get_by_role("button",name="Columns").click()
    wizard.get_by_label("Batch sync mode").select_option("full_refresh")
    wizard.get_by_label("Batch target alias").fill("full_parts")
    wizard.get_by_label("Batch column TranQty").uncheck()
    wizard.get_by_role("button",name="Add configured table to batch").click()
    wizard.get_by_label("Batch source table").select_option("InvcHead")
    wizard.get_by_role("button",name="Columns").click()
    wizard.get_by_label("Batch sync mode").select_option("incremental_rowversion")
    wizard.get_by_label("Batch target alias").fill("invoice_delta")
    wizard.get_by_role("button",name="Add configured table to batch").click()
    wizard.get_by_role("button",name="Save & Bind 2 streams").click()
    saved=[c for c in state["calls"] if c["path"]=="/api/v1/connections/runtime/streams/batch"]
    assert len(saved)==1
    data=saved[0]["body"]
    assert len(data["streams"])==2
    assert data["streams"][0]["mode"]=="full_refresh"
    assert data["streams"][0]["target_table"]=="full_parts"
    assert data["streams"][1]["mode"]=="incremental_rowversion"
    assert data["streams"][1]["target_table"]=="invoice_delta"
    assert "TranQty" not in data["streams"][0]["selected_columns"]
    wizard.get_by_role("button",name="Run All (2) — sequential enqueue").click()
    expect(wizard.get_by_text("2 / 2 streams queued.", exact=False)).to_be_visible()
    runs=[c for c in state["calls"] if c["path"]=="/api/v1/managed/runs" and c["method"]=="POST"]
    assert len(runs)==2
    assert {c["body"]["stream_id"] for c in runs}=={
        "abcdabcd-abcd-4bcd-8bcd-000000000001",
        "abcdabcd-abcd-4bcd-8bcd-000000000002",
    }
