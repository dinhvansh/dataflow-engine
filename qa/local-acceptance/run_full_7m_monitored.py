"""Windows owner QA monitor: E storage, C watchdog, sanitized stage logs."""
import datetime
import json
import os
from pathlib import Path
import subprocess
import threading
import time
import shutil
import sys

sys.stdout.reconfigure(encoding='utf-8', errors='replace')

assert os.name == 'nt', 'This owner-local storage attestation is Windows-specific'
assert os.environ.get('DATAFLOW_CORE_FULL_7M') == 'YES_OWNED_DISPOSABLE_7M'
assert Path.cwd().drive.upper() == 'E:'
for key in ('TEMP','TMP','GOCACHE','GOMODCACHE'):
    assert Path(os.environ[key]).drive.upper() == 'E:', key+' must be on E:'
assert Path('E:/docker/DockerDesktopWSL/disk/docker_data.vhdx').is_file()
settings=json.loads((Path(os.environ['APPDATA'])/'Docker/settings-store.json').read_text(encoding='utf-8'))
assert Path(settings['CustomWslDistroDir']).resolve() == Path('E:/docker/DockerDesktopWSL').resolve()
assert subprocess.check_output(['docker','context','show'],text=True).strip() == 'desktop-linux'
assert shutil.disk_usage('E:/').free >= 16*1024**3
assert shutil.disk_usage('C:/').free > 2.2*1024**3
out = Path(os.environ['DATAFLOW_7M_EVIDENCE'])
out.mkdir(parents=True,exist_ok=False)
secret_values = [v for k,v in os.environ.items() if v and any(s in k for s in ('PASSWORD','TOKEN','KEY_B64','DSN'))]
def safe(s):
    for value in secret_values:
        s=s.replace(value,'[REDACTED]')
    return s
def command(args):
    return subprocess.check_output(args,text=True,stderr=subprocess.DEVNULL).strip()
sha=command(['git','rev-parse','HEAD'])
(out/'environment.json').write_text(json.dumps(dict(git_sha=sha,
    dirty=bool(command(['git','status','--porcelain'])),docker=command(['docker','info','--format','{{.NCPU}} CPUs; {{.MemTotal}} RAM bytes']),
    storage='E: workspace/caches/temp/Docker VHDX',c_floor_bytes=2*1024**3,
    e_floor_bytes=16*1024**3,production_certified=False),indent=2)+'\n')
stop=threading.Event()
blocked=threading.Event()
def monitor():
    with (out/'resources.jsonl').open('w') as log:
        while not stop.is_set():
            try:
                cfree=shutil.disk_usage('C:/').free
                efree=shutil.disk_usage('E:/').free
                ids=command(['docker','ps','-q','--filter','label=com.docker.compose.project=dataflow-localqa']).split()
                stats=command(['docker','stats','--no-stream','--format','{{json .}}',*ids]) if ids else ''
                log.write(json.dumps(dict(utc=datetime.datetime.now(datetime.timezone.utc).isoformat(),
                    c_free_bytes=cfree,e_free_bytes=efree,stats=stats))+'\n');log.flush()
                if cfree <= 2.2*1024**3 or efree < 16*1024**3:
                    raise RuntimeError('disk floor approached')
            except Exception:
                blocked.set()
                ids=command(['docker','ps','-q','--filter','label=com.docker.compose.project=dataflow-localqa',
                             '--filter','label=com.dataflow.owner=dataflow-localqa','--filter','label=com.dataflow.role=local-qa']).split()
                if ids: subprocess.run(['docker','stop',*ids],stdout=subprocess.DEVNULL)
                return
            stop.wait(10)
thread=threading.Thread(target=monitor,daemon=True);thread.start()
matrix={stage:'NOT_RUN' for stage in ('ingestion_benchmark','operator_build','pipeline_and_fault')}
bash='C:/Program Files/Git/bin/bash.exe'
steps=[('ingestion_benchmark',[bash,'-lc','bash deploy/local-qa/run-core-7m.sh']),
       ('operator_build',[bash,'-lc',
        'qa_ca=$(mktemp deploy/local-qa/.qa-build-ca.XXXXXXXX); trap \'rm -f "$qa_ca"\' EXIT; '
        'cp "$CODEX_PROXY_CERT" "$qa_ca"; export CODEX_PROXY_CERT="$qa_ca"; '
        'docker compose --project-name dataflow-localqa --env-file .env.qa.local '
        '-f deploy/local-qa/compose.qa.yml --profile operator up -d --build --wait dbt-worker api web']),
       ('pipeline_and_fault',['python','-m','pytest','qa/browser/test_core_seven_million_live.py','-v','-s','--tb=short'])]
try:
    for stage,args in steps:
        if blocked.is_set(): matrix[stage]='BLOCKED';break
        start=time.monotonic()
        with (out/(stage+'.log')).open('w',encoding='utf-8') as log:
            proc=subprocess.Popen(args,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,encoding='utf-8',errors='replace')
            for line in proc.stdout:
                line=safe(line);log.write(line);log.flush();print(line,end='',flush=True)
            rc=proc.wait()
        matrix[stage]=dict(status='BLOCKED' if blocked.is_set() else ('PASS' if rc==0 else 'FAIL'),
                           seconds=round(time.monotonic()-start,3),exit_code=rc)
        if rc or blocked.is_set():break
except BaseException as exc:
    matrix[stage]=dict(status='FAIL',reason=type(exc).__name__)
    raise
finally:
    stop.set();thread.join(timeout=20)
    (out/'test-matrix.json').write_text(json.dumps(matrix,indent=2)+'\n')
    (out/'REPORT.md').write_text('# CORE-RUN-7 full-scale owner QA\n\n'
        +'Git SHA: `'+sha+'` (see environment.json for dirty state).\n\n'
        +'```json\n'+json.dumps(matrix,indent=2)+'\n```\n\n'
        +'Synthetic disposable QA only. See pipeline-result.json and fault-result.json for assertions. '
        +'Any failed or missing stage means full CORE-RUN-7 is NOT PASS. Not production certification.\n')
print('Evidence:',out,flush=True)
raise SystemExit(0 if all(isinstance(v,dict) and v['status']=='PASS' for v in matrix.values()) else 1)
