"""Real fault injection: ONLY disposable Docker QA, never any ERP production source."""
import json
import os
import time
from pathlib import Path

import pytest
import test_core_live as fixture

pytestmark = pytest.mark.skipif(
    os.getenv("DATAFLOW_LIVE_QA_OPT_IN") != "true" or
    os.getenv("DATAFLOW_CORE_FAULT_OPT_IN") != "true",
    reason="two explicit disposable fault-test opt-ins required",
)
NETWORK = "dataflow-localqa-net"
OUT = Path("qa-results/core-run6")


def owned_service(name):
    cid = fixture.shell("docker", "compose", "--project-name", "dataflow-localqa",
        "--env-file", ".env.qa.local", "-f", "deploy/local-qa/compose.qa.yml",
        "--profile", "operator", "ps", "-q", name)
    assert cid, f"Required QA service missing: {name}"
    fixture.owned(cid)
    return cid


def assert_network():
    labels = fixture.shell("docker", "network", "inspect", "-f",
        '{{index .Labels "com.dataflow.owner"}} {{index .Labels "com.dataflow.role"}}',
        NETWORK)
    assert labels == "dataflow-localqa local-qa", "Refusing unowned Docker network"


def connected(cid):
    return fixture.shell("docker", "inspect", "-f",
        '{{if index .NetworkSettings.Networks "dataflow-localqa-net"}}yes{{else}}no{{end}}',
        cid) == "yes"


def until(callback, description, seconds=180):
    limit = time.monotonic() + seconds
    while time.monotonic() < limit:
        value = callback()
        if value:
            return value
        time.sleep(.5)
    pytest.fail(f"Timeout: {description}")


def state(run_id):
    code, result = fixture.api("/api/v1/core/runs?id=" + run_id)
    assert code == 200
    return result


def run(stream_id, model_id):
    code, result = fixture.api("/api/v1/core/runs", "POST", {
        "stream_id": stream_id, "model_id": model_id, "unique_column": "stream",
    })
    assert code == 202, result
    return result["run_id"]


def cursor(stream_id):
    return int(fixture.pg(
        f"SELECT generation FROM sync_checkpoints WHERE stream_id='{stream_id}'"))


def uuid():
    return fixture.ch("SELECT toString(uuid) FROM system.tables "
        f"WHERE database='scratch' AND name='{fixture.TARGET}' FORMAT TSV")


def raw(stream_id):
    return fixture.ch("SELECT count(),uniqExact(_pk) FROM scratch.dataflow_raw_events "
        f"WHERE _stream_id='{stream_id}' FORMAT TSV")


def test_core_run6_sigkill_and_network_failure_recovery():
    fixture.assert_environment()
    assert_network()
    OUT.mkdir(parents=True, exist_ok=True)
    stream_id, model_id = fixture.build_fixture()
    api_id = owned_service("api")
    dbt_id = owned_service("dbt-worker")
    ch_id = owned_service("clickhouse")
    paused = False
    disconnected = False
    first_id = second_id = recovery_id = ""
    events = []
    try:
        # Freeze disposable dbt worker to deterministically hold a REAL active
        # PG core run at waiting_flow after SELECT-only 10k source ingestion.
        fixture.shell("docker", "pause", dbt_id)
        paused = True
        first_id = run(stream_id, model_id)
        until(lambda: state(first_id)["status"] == "waiting_flow",
              "managed sync to waiting_flow", 150)
        assert cursor(stream_id) == 1
        assert raw(stream_id) == "10000\t10000"
        fixture.shell("docker", "kill", "--signal=KILL", api_id)
        assert fixture.pg(f"SELECT status FROM core_pipeline_runs WHERE id='{first_id}'") == "waiting_flow"
        events.append("actual API SIGKILL while durable waiting_flow persisted")

        fixture.shell("docker", "unpause", dbt_id)
        paused = False
        fixture.shell("docker", "start", api_id)
        until(lambda: fixture.shell("docker", "inspect", "-f",
              "{{.State.Health.Status}}", api_id) == "healthy",
              "API health after SIGKILL", 90)
        result = until(lambda: (s if s["status"] in ("succeeded","failed") else None)
              if (s := state(first_id)) else None, "recovery from API SIGKILL", 190)
        assert result["status"] == "succeeded", result
        assert result["managed_status"] == result["model_flow_status"] == "succeeded"
        assert result["rows_verified"] == 1
        assert fixture.ch(f"SELECT stream,events FROM scratch.{fixture.TARGET} FORMAT TSV") == f"{stream_id}\t10000"
        original = uuid()
        assert original and cursor(stream_id) == 1
        events.append("same persisted run resumed dbt A-B-C + DQ + physical publish")

        # Real network isolation before POST: the HTTP/API and PG survive
        # but the worker cannot contact ClickHouse. Terminal failure must
        # never advance checkpoint or replace the already published table.
        assert connected(ch_id)
        fixture.shell("docker", "network", "disconnect", "-f", NETWORK, ch_id)
        disconnected = True
        assert not connected(ch_id)
        second_id = run(stream_id, model_id)
        failed = until(lambda: (s if s["status"] in ("succeeded","failed") else None)
              if (s := state(second_id)) else None,
              "terminal ClickHouse outage result", 210)
        assert failed["status"] == "failed", failed
        assert cursor(stream_id) == 1
        assert uuid() == original and raw(stream_id) == "10000\t10000"
        events.append("actual CH network disconnect left cursor, raw data and target UUID unchanged")
    finally:
        # Restore ONLY the owner-verified QA resources even on test assertion failure.
        if disconnected and not connected(ch_id):
            fixture.shell("docker", "network", "connect", "--alias", "clickhouse", NETWORK, ch_id)
        if paused:
            fixture.shell("docker", "unpause", dbt_id)
        if fixture.shell("docker", "inspect", "-f", "{{.State.Running}}", api_id) != "true":
            fixture.shell("docker", "start", api_id)

    assert state(second_id)["status"] == "failed"
    recovery_id = run(stream_id, model_id)  # deliberate new operator run
    result = until(lambda: (s if s["status"] in ("succeeded","failed") else None)
            if (s := state(recovery_id)) else None, "explicit retry after reconnect", 190)
    assert result["status"] == "succeeded", result
    assert result["has_backup"] is True
    assert cursor(stream_id) == 2 and raw(stream_id) == "10000\t10000"
    assert fixture.pg("SELECT rows_written FROM managed_runs "
        f"WHERE id='{result['managed_run_id']}'") == "0"
    assert uuid() and uuid() != original
    code, rollback = fixture.api("/api/v1/core/actions", "POST",
        {"run_id": recovery_id, "action": "rollback", "confirm": "ROLLBACK"})
    assert code == 200 and rollback["status"] == "rolled_back"
    assert uuid() == original and cursor(stream_id) == 2
    events.append("resubmitted 0-row incremental run; exact physical UUID rollback; cursor unchanged")
    (OUT / "fault-result.json").write_text(json.dumps({
        "gate":"CORE-RUN-6", "scope":"hosted disposable QA only",
        "source":"DataFlowFixtureQA.Erp.PartTran SELECT-only",
        "source_rows":10000, "first_run":first_id, "outage_run":second_id,
        "operator_resubmission_run":recovery_id, "cursor_generations":[1,1,2],
        "restored_original_uuid":True, "events":events,
        "automatic_resume_after_terminal_network_failure":"NOT CLAIMED",
    },indent=2)+"\n", encoding="utf-8")
