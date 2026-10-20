"""Owner-only 7m row-preserving core pipeline. Never enabled by ordinary CI."""
import json
import os
import time
from pathlib import Path

import pytest
import test_core_live as f
from test_core_negative_dq_live import wait_done, target_uuid

pytestmark = pytest.mark.skipif(
    os.getenv('DATAFLOW_CORE_FULL_7M') != 'YES_OWNED_DISPOSABLE_7M',
    reason='explicit full-scale owner QA required',
)
OUT = Path(os.getenv('DATAFLOW_7M_EVIDENCE', 'qa-results/core-run7-full'))
N = 7_000_000


def fingerprint(table):
    return f.ch(f"SELECT count(),uniqExact(_pk),sum(cityHash64(_pk,_payload)),"
                f"sum(JSONExtractInt(_payload,'TranNum')) FROM scratch.{table} FORMAT TSV")


def test_full_scale_pipeline():
    f.assert_environment()
    OUT.mkdir(parents=True, exist_ok=True)
    matrix = {}
    def record(stage, start, **extra):
        matrix[stage] = dict(status='PASS', seconds=round(time.monotonic()-start, 3), **extra)
        (OUT/'pipeline-result.json').write_text(json.dumps(matrix, indent=2)+'\n')
        print('7M_STAGE_PASS', stage, matrix[stage], flush=True)
    started = time.monotonic()
    source = f.shell('docker','exec','dataflow-localqa-mssql',
        '/opt/mssql-tools18/bin/sqlcmd','-C','-h','-1','-W','-S','localhost',
        '-U','qa_erp_reader','-P',f.READER_PASSWORD,'-d','DataFlowFixtureQA','-Q',
        "SET NOCOUNT ON; SELECT COUNT_BIG(*),SUM(TranNum),"
        "HAS_PERMS_BY_NAME('Erp.PartTranScale7M','OBJECT','SELECT'),"
        "HAS_PERMS_BY_NAME('Erp.PartTranScale7M','OBJECT','INSERT'),"
        "HAS_PERMS_BY_NAME('Erp.PartTranScale7M','OBJECT','UPDATE'),"
        "HAS_PERMS_BY_NAME('Erp.PartTranScale7M','OBJECT','DELETE'),"
        "HAS_PERMS_BY_NAME('Erp.PartTranScale7M','OBJECT','ALTER') FROM Erp.PartTranScale7M")
    assert source.split() == [str(N),str(N*(N+1)//2),'1','0','0','0','0']
    f.ch('CREATE TABLE scratch.dataflow_raw_events ('
         '_stream_id String,_pk String,_sysrevid String,_payload String,'
         "_run_id String,_batch_seq UInt64,_ingested_at DateTime64(3,'UTC') DEFAULT now64(3))"
         ' ENGINE=MergeTree ORDER BY (_stream_id,_pk,_sysrevid)')
    code, stream = f.api('/api/v1/managed/streams','POST',dict(
        source_schema='Erp',source_table='PartTranScale7M',target_database='scratch',
        target_table=f.RAW,batch_size=10000,workers=2,queue_capacity=2))
    assert code == 201
    names = ['scale7_'+f.SUFFIX+'_'+s for s in ('a','b','c')]
    prior = f.RAW
    for i,name in enumerate(names):
        model = dict(name=name, mode='visual',source=dict(database='scratch',table=prior),
            steps=[dict(id='rename'+str(i),type='rename',
                        column=('_stream_id' if i == 0 else 'stream'+str(i-1)),alias='stream'+str(i))])
        code,saved = f.api('/api/v1/transform/models','POST',dict(model=model))
        assert code == 201
        prior = 'dfv_'+name
    request = dict(stream_id=stream['id'],model_id=saved['id'],unique_column='_pk')
    target = 'dft_core_'+names[-1]
    code,queued = f.api('/api/v1/core/runs','POST',request)
    assert code == 202
    first = wait_done(queued['run_id'],1800)
    assert first['status'] == 'succeeded', first
    assert first['rows_verified'] == N, first
    expected = fingerprint(f.RAW)
    fields = expected.split('\t')
    assert fields[:2] == [str(N),str(N)] and fields[3] == str(N*(N+1)//2), expected
    assert fingerprint(target) == expected
    assert [s['model_name'] for s in first['steps']] == names
    first_uuid = target_uuid(target)
    record('full_sync_dbt_abc_dq_publish',started,rows=N,fingerprint=expected,
           run_id=queued['run_id'],rows_per_second=round(N/(time.monotonic()-started),2))
    started = time.monotonic()
    code,queued = f.api('/api/v1/core/runs','POST',request)
    assert code == 202
    second = wait_done(queued['run_id'],1800)
    assert second['status'] == 'succeeded' and second['rows_verified'] == N, second
    assert f.pg("SELECT rows_written FROM managed_runs WHERE id='"+second['managed_run_id']+"'") == '0'
    assert fingerprint(target) == expected and fingerprint(f.RAW) == expected
    assert target_uuid(target) != first_uuid and second['has_backup']
    generation = f.pg("SELECT generation FROM sync_checkpoints WHERE stream_id='"+stream['id']+"'")
    assert generation == '2'
    record('incremental_replay_publish',started,rows_written=0,rows_verified=N,checkpoint_generation=2)
    started = time.monotonic()
    for _ in range(2):
        code,rolled = f.api('/api/v1/core/actions','POST',dict(
            run_id=queued['run_id'],action='rollback',confirm='ROLLBACK'))
        assert code == 200 and rolled['status'] == 'rolled_back', rolled
        assert target_uuid(target) == first_uuid
    assert fingerprint(target) == expected
    assert f.pg("SELECT generation FROM sync_checkpoints WHERE stream_id='"+stream['id']+"'") == generation
    record('rollback_idempotency_checkpoint',started,rows=N)
    started = time.monotonic()
    # All 7m rows remain; a deliberately nonunique stream key must fail DQ.
    badname = 'scale7_'+f.SUFFIX+'_bad'
    code,bad = f.api('/api/v1/transform/models','POST',dict(model=dict(
        name=badname,mode='visual',source=dict(database='scratch',table=f.RAW),
        steps=[dict(id='stream',type='rename',column='_stream_id',alias='duplicate_key')])))
    assert code == 201
    code,bad_run = f.api('/api/v1/core/runs','POST',dict(
        stream_id=stream['id'],model_id=bad['id'],unique_column='duplicate_key'))
    assert code == 202
    rejected = wait_done(bad_run['run_id'],1800)
    assert rejected['status'] == 'failed', rejected
    assert target_uuid('dft_core_'+badname) == ''
    assert target_uuid(target) == first_uuid and fingerprint(target) == expected
    assert f.ch(f'SELECT count(),uniqExact(duplicate_key) FROM scratch.dfv_{badname} FORMAT TSV') == f'{N}\t1'
    record('negative_dq_no_publish',started,rows=N,existing_uuid_preserved=True)


def test_full_scale_active_sql_crash_recovery():
    from test_core_mid_sql_live import service, wait, physical
    assert os.getenv('DATAFLOW_LIVE_QA_OPT_IN') == 'true'
    for name in ('mssql','postgres','clickhouse'):
        f.owned('dataflow-localqa-'+name)
    worker = service('dbt-worker')
    assert f.ch('SELECT count(),uniqExact(_pk) FROM scratch.dataflow_raw_events FORMAT TSV') == f'{N}\t{N}'
    name = 'fault7_'+f.SUFFIX
    target = 'dft_'+name
    query = ('SELECT _pk AS id,cityHash64(_payload) AS payload_hash,'
             'sleepEachRow(0.00001) AS qa_delay FROM scratch.dataflow_raw_events '
             'SETTINGS max_block_size=8192,max_threads=1')
    model = f.pg("INSERT INTO transform_models(name,definition,compiled_sql) "
        f"VALUES('{name}','{{}}'::jsonb,'{query}') RETURNING id").splitlines()[0]
    job = f.pg('SELECT gen_random_uuid()::text')
    stage = 'dft_stage_'+job.replace('-','')
    f.ch(f'CREATE TABLE scratch.{target} ENGINE=MergeTree ORDER BY id AS '
         'SELECT _pk AS id,cityHash64(_payload) AS payload_hash,toUInt8(0) AS qa_delay '
         'FROM scratch.dataflow_raw_events')
    def digest(table):
        return f.ch(f'SELECT count(),uniqExact(id),sum(cityHash64(id,payload_hash)) FROM scratch.{table} FORMAT TSV')
    expected = digest(target)
    original = physical(target)
    checkpoints = f.pg('SELECT stream_id,generation FROM sync_checkpoints ORDER BY stream_id')
    started = time.monotonic()
    f.pg('INSERT INTO table_runs(id,model_id,model_name,compiled_sql,source_definition,'
         'stage_name,target_name,unique_column) '
         f"VALUES('{job}','{model}','{name}','{query}','{{}}','{stage}','{target}','id')")
    killed = False
    try:
        def active():
            return f.ch("SELECT count() FROM system.processes WHERE position(query,'sleepEachRow')>0 "
                        f"AND position(query,'{stage}')>0 AND position(query,'system.processes')=0 FORMAT TSV") != '0'
        wait(active,'7m dbt SQL observed executing',180)
        old_owner = f.pg(f"SELECT lease_owner FROM table_runs WHERE id='{job}' AND state='building' AND attempt_count=1")
        assert old_owner
        f.shell('docker','kill','--signal=KILL',worker)
        killed = True
        assert physical(target) == original
        assert f.pg(f"SELECT state FROM table_runs WHERE id='{job}'") == 'building'
        f.pg("UPDATE table_runs SET lease_until=clock_timestamp()-interval '1 second' "
             f"WHERE id='{job}' AND lease_owner='{old_owner}' AND state='building'")
        f.shell('docker','start',worker)
        killed = False
        def rebuilt():
            row = f.pg(f"SELECT state||'|'||attempt_count||'|'||coalesce(rows_verified,0) FROM table_runs WHERE id='{job}'")
            if row.startswith('failed|'):
                pytest.fail('7m crash recovery failed; inspect sanitized worker diagnostics')
            return row if row.startswith('ready|') else None
        assert wait(rebuilt,'7m dbt rebuilt and DQ passed',600) == f'ready|2|{N}'
        assert physical(target) == original and digest(stage) == expected
        # A stale ownership token cannot mutate the recovered durable job.
        stale = f.pg(f"UPDATE table_runs SET error_message='stale QA owner' WHERE id='{job}' "
                     f"AND lease_owner='{old_owner}' AND state='building' RETURNING id")
        assert stale == 'UPDATE 0', stale
        new = physical(stage)
        code,_ = f.api('/api/v1/transform/table/action','POST',dict(run_id=job,action='publish',confirm='PUBLISH'))
        assert code == 202
        wait(lambda: f.pg(f"SELECT state FROM table_runs WHERE id='{job}'") == 'published','7m physical publish',90)
        assert physical(target) == new and physical(stage) == original and digest(target) == expected
        code,_ = f.api('/api/v1/transform/table/action','POST',dict(run_id=job,action='rollback',confirm='ROLLBACK'))
        assert code == 202
        wait(lambda: f.pg(f"SELECT state FROM table_runs WHERE id='{job}'") == 'rolled_back','7m rollback',90)
        assert physical(target) == original and digest(target) == expected
        assert f.pg('SELECT stream_id,generation FROM sync_checkpoints ORDER BY stream_id') == checkpoints
        (OUT/'fault-result.json').write_text(json.dumps(dict(status='PASS',rows=N,
            seconds=round(time.monotonic()-started,3),actual_sql_observed=True,
            attempts=2,stale_owner_update_rows=0,restored_uuid=original,
            fingerprint=expected,checkpoint_preserved=True),indent=2)+'\n')
    finally:
        if killed:
            f.shell('docker','start',worker)
