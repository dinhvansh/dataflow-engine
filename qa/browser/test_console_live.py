"""Disposable full-stack browser acceptance; never point this at a real ERP."""
import base64
import json
import os
import subprocess
import time
import urllib.parse
import urllib.request
from pathlib import Path

import pytest
from playwright.sync_api import expect, sync_playwright


BASE = os.getenv("DATAFLOW_BROWSER_BASE_URL", "http://127.0.0.1:5173")
API = os.getenv("DATAFLOW_LIVE_API_URL", "http://127.0.0.1:8080")
TOKEN = os.environ.get("DATAFLOW_API_TOKEN", "")
CH_PASSWORD = os.environ.get("CLICKHOUSE_ADMIN_PASSWORD", "")
ARTIFACTS = Path("qa-results/v14-local-acceptance/artifacts/playwright-live")
CHAIN = {"a": "live_ui_a", "b": "live_ui_b", "c": "live_ui_c"}


def api(path, method="GET", body=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(API + path, data=data, method=method)
    req.add_header("Authorization", "Bearer " + TOKEN)
    if data is not None:
        req.add_header("Content-Type", "application/json")
    with urllib.request.urlopen(req, timeout=30) as response:
        return json.load(response)


def ch(query):
    req = urllib.request.Request("http://127.0.0.1:18123/", data=query.encode(), method="POST")
    auth = base64.b64encode(("qa_admin:" + CH_PASSWORD).encode()).decode()
    req.add_header("Authorization", "Basic " + auth)
    with urllib.request.urlopen(req, timeout=30) as response:
        return response.read().decode().strip()


def pg_scalar(sql):
    return subprocess.check_output([
        "docker", "exec", "dataflow-localqa-postgres", "psql", "-U", "dataflow_qa",
        "-d", "dataflow_qa", "-Atc", sql,
    ], text=True).strip()


@pytest.fixture(scope="session", autouse=True)
def live_fixtures():
    if not TOKEN or not CH_PASSWORD:
        pytest.fail("live QA credentials are required through environment variables")
    ch("DROP TABLE IF EXISTS scratch.live_ui_raw")
    ch("CREATE TABLE scratch.live_ui_raw (_pk String, Qty UInt64) ENGINE=MergeTree ORDER BY _pk")
    ch("INSERT INTO scratch.live_ui_raw VALUES ('P1',3),('P2',5)")
    models = [
        (CHAIN["a"], "live_ui_raw"),
        (CHAIN["b"], "dfv_" + CHAIN["a"]),
        (CHAIN["c"], "dfv_" + CHAIN["b"]),
    ]
    for name, source in models:
        api("/api/v1/transform/models", "POST", {"model": {
            "name": name, "mode": "visual", "source": {"database": "scratch", "table": source}, "steps": [],
        }})
    ch("DROP TABLE IF EXISTS scratch.dft_ui_v14")
    ch("DROP TABLE IF EXISTS scratch.dfd_ui_v14")
    ch("CREATE TABLE scratch.dft_ui_v14 (InvoiceNum String, Amount UInt64) ENGINE=MergeTree ORDER BY InvoiceNum")
    ch("INSERT INTO scratch.dft_ui_v14 VALUES ('I1',10)")
    ch("CREATE TABLE scratch.dfd_ui_v14 (InvoiceNum String, Amount UInt64, _df_version UInt64, _df_deleted UInt8) ENGINE=MergeTree ORDER BY (InvoiceNum,_df_version)")
    ch("INSERT INTO scratch.dfd_ui_v14 VALUES ('I1',20,1,0),('I2',30,2,0)")


@pytest.fixture(scope="session")
def browser():
    with sync_playwright() as pw:
        instance = pw.chromium.launch(headless=True)
        yield instance
        instance.close()


@pytest.fixture
def ui(browser, request):
    ARTIFACTS.mkdir(parents=True, exist_ok=True)
    context = browser.new_context(viewport={"width": 1440, "height": 1000})
    context.tracing.start(screenshots=True, snapshots=True, sources=True)
    page = context.new_page()
    page.goto(BASE, wait_until="networkidle")
    yield page
    page.screenshot(path=str(ARTIFACTS / (request.node.name + ".png")), full_page=True)
    context.tracing.stop(path=str(ARTIFACTS / (request.node.name + ".zip")))
    context.close()


def open_tab(page, name):
    page.get_by_role("button", name=name, exact=True).click()
    page.locator("input[type=password]").first.fill(TOKEN)


def test_live_ui01_source_explorer(ui):
    open_tab(ui, "Connections")
    ui.get_by_role("button", name="List", exact=True).click()
    expect(ui.get_by_text("Available tables", exact=False)).to_be_visible(timeout=15_000)
    ui.get_by_role("button", name="Inspect", exact=True).click()
    expect(ui.get_by_text("Erp.PartTran", exact=False)).to_be_visible(timeout=15_000)
    expect(ui.get_by_text("SysRowID", exact=True)).to_be_visible()
    assert int(pg_scalar("SELECT count(*) FROM bootstrap_runs")) >= 0


def test_live_ui02_visual_preview(ui):
    open_tab(ui, "Transform Studio")
    ui.get_by_label("Bảng nguồn").fill("live_ui_raw")
    ui.get_by_role("button", name="Lọc hàng").click()
    ui.get_by_role("button", name="Preview bước").click()
    expect(ui.get_by_text("Preview trả về", exact=False)).to_be_visible(timeout=15_000)
    assert ch("SELECT count() FROM scratch.live_ui_raw") == "2"


def test_live_ui03_save_reload(ui):
    open_tab(ui, "Transform Studio")
    ui.get_by_role("button", name="Lưu Model").click()
    expect(ui.get_by_text("Đã lưu model my_first_transform", exact=False)).to_be_visible(timeout=15_000)
    ui.get_by_role("button", name="my_first_transform", exact=True).click()
    expect(ui.get_by_text("Đã tải model my_first_transform", exact=False)).to_be_visible()
    assert pg_scalar("SELECT count(*) FROM transform_models WHERE name='my_first_transform'") == "1"


def test_live_ui04_lineage(ui):
    open_tab(ui, "Data Lineage")
    ui.get_by_role("button", name="Refresh Graph").click()
    node = ui.locator('button[title="live_ui_c"]')
    expect(node).to_be_visible(timeout=15_000)
    node.click()
    expect(ui.get_by_text("Model Inspector")).to_be_visible()
    assert int(pg_scalar("SELECT count(*) FROM transform_models WHERE name LIKE 'live_ui_%'")) == 3


def test_live_ui05_auto_build(ui):
    open_tab(ui, "Data Lineage")
    ui.get_by_role("button", name="Refresh Graph").click()
    ui.locator('button[title="live_ui_c"]').click()
    ui.get_by_role("button", name="▶ Run Model + Dependencies").click()
    expect(ui.get_by_text("Flow: succeeded", exact=False)).to_be_visible(timeout=90_000)
    assert ch("SELECT count() FROM scratch.dfv_live_ui_c") == "2"
    assert pg_scalar("SELECT status FROM model_flow_runs WHERE target_model_name='live_ui_c' ORDER BY created_at DESC LIMIT 1") == "succeeded"


def test_live_ui06_incremental_publish_rollback(ui):
    open_tab(ui, "Incremental CDC")
    labels = {
        "Khóa duy nhất": "InvoiceNum",
        "Target (bảng vật lý hiện có trong scratch)": "dft_ui_v14",
        "Delta / CDC table trong scratch": "dfd_ui_v14",
        "Cursor ban đầu (chỉ sử dụng khi đăng ký target lần đầu)": "0",
    }
    for label, value in labels.items():
        ui.get_by_label(label).fill(value)
    ui.on("dialog", lambda dialog: dialog.accept())
    ui.get_by_role("button", name="Build Delta Staging", exact=False).click()
    expect(ui.get_by_text("ready", exact=True)).to_be_visible(timeout=60_000)
    ui.get_by_role("button", name="Approve & Publish + Cursor", exact=False).click()
    expect(ui.get_by_text("published", exact=True)).to_be_visible(timeout=60_000)
    assert ch("SELECT count(),sum(Amount) FROM scratch.dft_ui_v14 FORMAT TSV") == "2\t50"
    ui.get_by_role("button", name="Rollback Data + Cursor", exact=False).click()
    expect(ui.get_by_text("rolled_back", exact=True)).to_be_visible(timeout=60_000)
    assert ch("SELECT count(),sum(Amount) FROM scratch.dft_ui_v14 FORMAT TSV") == "1\t10"
    assert pg_scalar("SELECT state FROM incremental_runs WHERE target_name='dft_ui_v14' ORDER BY created_at DESC LIMIT 1") == "rolled_back"
