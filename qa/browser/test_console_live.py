"""Disposable full-stack browser acceptance; never point this at a real ERP."""
import base64
import json
import os
import subprocess
import urllib.parse
import urllib.request
import uuid
import zipfile
from pathlib import Path

import pytest
from playwright.sync_api import expect, sync_playwright


BASE = os.getenv("DATAFLOW_BROWSER_BASE_URL", "http://127.0.0.1:5173")
API = os.getenv("DATAFLOW_LIVE_API_URL", "http://127.0.0.1:8080")
SAFE_TRACE_TOKEN = "synthetic-live-browser-token-not-a-secret"
TOKEN = os.environ.get("DATAFLOW_LIVE_API_TOKEN", "")
CH_PASSWORD = os.environ.get("CLICKHOUSE_ADMIN_PASSWORD", "")
ARTIFACTS = Path("qa-results/v14-local-acceptance/artifacts/playwright-live")
RUN_SUFFIX = uuid.uuid4().hex[:10]
CHAIN = {key: f"live_{RUN_SUFFIX}_{key}" for key in ("a", "b", "c")}
RAW = f"live_raw_{RUN_SUFFIX}"
TARGET = f"dft_ui_{RUN_SUFFIX}"
DELTA = f"dfd_ui_{RUN_SUFFIX}"
SAVED = f"live_saved_{RUN_SUFFIX}"
pytestmark = pytest.mark.skipif(
    os.getenv("DATAFLOW_LIVE_QA_OPT_IN") != "true",
    reason="live browser QA requires DATAFLOW_LIVE_QA_OPT_IN=true",
)


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


def assert_loopback(url):
    parsed = urllib.parse.urlparse(url)
    if parsed.scheme != "http" or parsed.hostname not in {"127.0.0.1", "localhost"}:
        pytest.fail(f"live QA endpoint must be loopback HTTP: {url}")


def assert_owned_container(name):
    labels = subprocess.check_output([
        "docker", "inspect", "-f",
        '{{index .Config.Labels "com.dataflow.owner"}} {{index .Config.Labels "com.dataflow.role"}}', name,
    ], text=True).strip()
    if labels != "dataflow-localqa local-qa":
        pytest.fail(f"refusing unowned container {name}: {labels}")


def assert_trace_safe(path):
    secrets = [os.environ.get(name, "") for name in (
        "MSSQL_SA_PASSWORD", "QA_SEED_PASSWORD", "QA_READER_PASSWORD", "POSTGRES_PASSWORD",
        "CLICKHOUSE_ADMIN_PASSWORD", "CLICKHOUSE_READER_PASSWORD", "CLICKHOUSE_WRITER_PASSWORD",
        "DATAFLOW_API_TOKEN",
    )]
    with zipfile.ZipFile(path) as archive:
        payload = b"".join(archive.read(name) for name in archive.namelist())
    for secret in filter(None, secrets):
        if secret.encode() in payload:
            path.unlink(missing_ok=True)
            pytest.fail(f"trace contained a protected environment value: {path.name}")


@pytest.fixture(scope="session", autouse=True)
def live_fixtures():
    if TOKEN != SAFE_TRACE_TOKEN:
        pytest.fail("live browser traces require the documented synthetic API token")
    if not CH_PASSWORD:
        pytest.fail("live QA ClickHouse credential is required through the environment")
    assert_loopback(BASE)
    assert_loopback(API)
    for name in ("dataflow-localqa-mssql", "dataflow-localqa-postgres", "dataflow-localqa-clickhouse"):
        assert_owned_container(name)
    with urllib.request.urlopen(API + "/healthz", timeout=5) as response:
        if response.read().decode().strip() != "ok":
            pytest.fail("live QA API health check failed")
    database = subprocess.check_output([
        "docker", "exec", "dataflow-localqa-mssql", "/opt/mssql-tools18/bin/sqlcmd", "-C", "-h", "-1", "-W",
        "-S", "localhost", "-U", "qa_erp_reader", "-P", os.environ.get("QA_READER_PASSWORD", ""),
        "-d", "DataFlowFixtureQA", "-Q", "SET NOCOUNT ON; SELECT DB_NAME()",
    ], text=True).strip()
    if database != "DataFlowFixtureQA":
        pytest.fail(f"refusing SQL Server database {database!r}")
    ch(f"CREATE TABLE scratch.{RAW} (_pk String, Qty UInt64) ENGINE=MergeTree ORDER BY _pk")
    ch(f"INSERT INTO scratch.{RAW} VALUES ('P1',3),('P2',5)")
    models = [
        (CHAIN["a"], RAW),
        (CHAIN["b"], "dfv_" + CHAIN["a"]),
        (CHAIN["c"], "dfv_" + CHAIN["b"]),
    ]
    for name, source in models:
        api("/api/v1/transform/models", "POST", {"model": {
            "name": name, "mode": "visual", "source": {"database": "scratch", "table": source}, "steps": [],
        }})
    ch(f"CREATE TABLE scratch.{TARGET} (InvoiceNum String, Amount UInt64) ENGINE=MergeTree ORDER BY InvoiceNum")
    ch(f"INSERT INTO scratch.{TARGET} VALUES ('I1',10)")
    ch(f"CREATE TABLE scratch.{DELTA} (InvoiceNum String, Amount UInt64, _df_version UInt64, _df_deleted UInt8) ENGINE=MergeTree ORDER BY (InvoiceNum,_df_version)")
    ch(f"INSERT INTO scratch.{DELTA} VALUES ('I1',20,1,0),('I2',30,2,0)")
    yield
    for table in (f"dfv_{CHAIN['c']}", f"dfv_{CHAIN['b']}", f"dfv_{CHAIN['a']}", TARGET, DELTA, RAW):
        ch(f"DROP TABLE IF EXISTS scratch.{table}")
    pg_scalar("DELETE FROM incremental_runs WHERE target_name='" + TARGET + "'")
    pg_scalar("DELETE FROM incremental_targets WHERE target_name='" + TARGET + "'")
    owned = "('" + "','".join((*CHAIN.values(), SAVED)) + "')"
    pg_scalar("DELETE FROM model_flow_steps WHERE model_name IN " + owned)
    pg_scalar("DELETE FROM model_flow_runs WHERE target_model_name IN " + owned)
    pg_scalar("DELETE FROM dbt_runs WHERE model_name IN " + owned)
    pg_scalar("DELETE FROM transform_models WHERE name IN " + owned)


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
    trace = ARTIFACTS / (request.node.name + ".zip")
    context.tracing.stop(path=str(trace))
    assert_trace_safe(trace)
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
    ui.get_by_label("Bảng nguồn").fill(RAW)
    ui.get_by_role("button", name="Lọc hàng").click()
    ui.get_by_role("button", name="Preview bước").click()
    expect(ui.get_by_text("Preview trả về", exact=False)).to_be_visible(timeout=15_000)
    assert ch(f"SELECT count() FROM scratch.{RAW}") == "2"


def test_live_ui03_save_reload(ui):
    open_tab(ui, "Transform Studio")
    ui.get_by_label("Tên model").fill(SAVED)
    ui.get_by_role("button", name="Lưu Model").click()
    expect(ui.get_by_text("Đã lưu model " + SAVED, exact=False)).to_be_visible(timeout=15_000)
    ui.get_by_role("button", name=SAVED, exact=True).click()
    expect(ui.get_by_text("Đã tải model " + SAVED, exact=False)).to_be_visible()
    assert pg_scalar("SELECT count(*) FROM transform_models WHERE name='" + SAVED + "'") == "1"


def test_live_ui04_lineage(ui):
    open_tab(ui, "Data Lineage")
    ui.get_by_role("button", name="Refresh Graph").click()
    node = ui.locator(f'button[title="{CHAIN["c"]}"]')
    expect(node).to_be_visible(timeout=15_000)
    node.click()
    expect(ui.get_by_text("Model Inspector")).to_be_visible()
    assert int(pg_scalar("SELECT count(*) FROM transform_models WHERE name LIKE 'live_" + RUN_SUFFIX + "_%'")) == 3


def test_live_ui05_auto_build(ui):
    open_tab(ui, "Data Lineage")
    ui.get_by_role("button", name="Refresh Graph").click()
    ui.locator(f'button[title="{CHAIN["c"]}"]').click()
    ui.get_by_role("button", name="▶ Run Model + Dependencies").click()
    expect(ui.get_by_text("Flow: succeeded", exact=False)).to_be_visible(timeout=90_000)
    assert ch(f"SELECT count() FROM scratch.dfv_{CHAIN['c']}") == "2"
    assert pg_scalar("SELECT status FROM model_flow_runs WHERE target_model_name='" + CHAIN["c"] + "' ORDER BY created_at DESC LIMIT 1") == "succeeded"


def test_live_ui06_incremental_publish_rollback(ui):
    open_tab(ui, "Incremental CDC")
    labels = {
        "Khóa duy nhất": "InvoiceNum",
        "Target (bảng vật lý hiện có trong scratch)": TARGET,
        "Delta / CDC table trong scratch": DELTA,
        "Cursor ban đầu (chỉ sử dụng khi đăng ký target lần đầu)": "0",
    }
    for label, value in labels.items():
        ui.get_by_label(label).fill(value)
    ui.on("dialog", lambda dialog: dialog.accept())
    ui.get_by_role("button", name="Build Delta Staging", exact=False).click()
    expect(ui.get_by_text("ready", exact=True)).to_be_visible(timeout=60_000)
    ui.get_by_role("button", name="Approve & Publish + Cursor", exact=False).click()
    expect(ui.get_by_text("published", exact=True)).to_be_visible(timeout=60_000)
    assert ch(f"SELECT count(),sum(Amount) FROM scratch.{TARGET} FORMAT TSV") == "2\t50"
    ui.get_by_role("button", name="Rollback Data + Cursor", exact=False).click()
    expect(ui.get_by_text("rolled_back", exact=True)).to_be_visible(timeout=60_000)
    assert ch(f"SELECT count(),sum(Amount) FROM scratch.{TARGET} FORMAT TSV") == "1\t10"
    assert pg_scalar("SELECT state FROM incremental_runs WHERE target_name='" + TARGET + "' ORDER BY created_at DESC LIMIT 1") == "rolled_back"
