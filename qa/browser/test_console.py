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
        "sql_jobs": [
            {"id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "name": "revenue_quality_check",
             "database": "scratch", "query": "SELECT count() AS total_rows FROM scratch.events", "enabled": True},
            {"id": "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "name": "inventory_snapshot_check",
             "database": "scratch", "query": "SELECT 1", "enabled": False},
        ],
    }

    def respond(route):
        req = route.request
        path = urlparse(req.url).path
        method = req.method
        body = json.loads(req.post_data) if req.post_data else {}
        state["calls"].append({"method": method, "path": path, "body": body})
        result = None
        if path == "/api/v1/sql/jobs":
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
        elif path == "/api/v1/sql/dags":
            result = {"pipelines": [
                {"id": "33333333-3333-4333-8333-333333333333", "name": "revenue_reporting",
                 "enabled": True, "nodes": [
                     {"id": "extract_invoice", "job_id": "44444444-4444-4444-8444-444444444444", "requires": []},
                     {"id": "revenue_check", "job_id": "55555555-5555-4555-8555-555555555555", "requires": ["extract_invoice"]}
                 ]}
            ]}
        elif path == "/api/v1/sql/dag-runs":
            if "id=" in req.url:
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

    page.get_by_label("Search connectors").fill("mysql")
    expect(page.get_by_text("MySQL / MariaDB", exact=True)).to_be_visible()
    expect(page.get_by_text("Microsoft SQL Server", exact=True)).not_to_be_visible()
    page.get_by_role("button", name="Clear connector search").click()
    expect(page.get_by_text("Microsoft SQL Server", exact=True)).to_be_visible()

    page.get_by_role("button", name="Planned", exact=True).click()
    expect(page.get_by_text("Microsoft SQL Server", exact=True)).not_to_be_visible()
    page.get_by_role("button", name="Available", exact=True).click()
    expect(page.get_by_text("Microsoft SQL Server", exact=True)).to_be_visible()

    page.get_by_role("tab", name=re.compile("Destinations")).click()
    expect(page.get_by_text("ClickHouse", exact=True)).to_be_visible()
    page.get_by_role("tab", name=re.compile("Data routes")).click()
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
