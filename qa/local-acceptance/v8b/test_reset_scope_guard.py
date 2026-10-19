"""Contract QA: disposable reset guard must refuse extra project resources.

Runs with a stubbed Docker binary against a temporary fake repo. Never touches
the actual Docker daemon, real .env.qa.local or any volumes.
"""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT=Path(__file__).resolve().parents[3]
SCRIPT=ROOT/"deploy/local-qa/verify-reset-scope.sh"

FAKE_DOCKER="""#!/bin/sh
case "$1 $2" in
  "info "*) exit 0;;
  "ps -aq"*) exit 0;;
  "volume ls"*) printf '%s\\n' "$QA_FAKE_VOLUMES"; exit 0;;
  "volume inspect"*) exit 1;;
  "network ls"*) printf '%s\\n' "$QA_FAKE_NETWORKS"; exit 0;;
  "network inspect"*) exit 1;;
esac
echo "Unexpected Docker command: $*" >&2
exit 98
"""

class ResetScopeTest(unittest.TestCase):
 def scenario(self,volumes="",networks=""):
  with tempfile.TemporaryDirectory() as base:
   root=Path(base);script=root/"deploy/local-qa/verify-reset-scope.sh"
   script.parent.mkdir(parents=True)
   shutil.copyfile(SCRIPT,script);script.chmod(0o700)
   (root/".env.qa.local").write_text("POSTGRES_PASSWORD=synthetic-qa-only\\n")
   bindir=root/"bin";bindir.mkdir()
   docker=bindir/"docker";docker.write_text(FAKE_DOCKER);docker.chmod(0o700)
   env=os.environ.copy()
   env.update(PATH=str(bindir)+os.pathsep+env.get("PATH",""),
    QA_FAKE_VOLUMES=volumes,QA_FAKE_NETWORKS=networks)
   return subprocess.run(["bash",str(script)],env=env,cwd=root,
     text=True,capture_output=True,check=False)

 def test_clean_project_inventory_allows_preflight(self):
  x=self.scenario()
  self.assertEqual(x.returncode,0,x.stderr)
  self.assertIn("PASS:",x.stdout)

 def test_unknown_project_volume_refused(self):
  x=self.scenario("dataflow-localqa_unexpected-backup")
  self.assertEqual(x.returncode,2,(x.stdout,x.stderr))
  self.assertIn("unexpected project QA volume",x.stderr)

 def test_unknown_project_network_refused(self):
  x=self.scenario(networks="dataflow-localqa-net\\ndataflow-localqa-unexpected")
  self.assertEqual(x.returncode,2,(x.stdout,x.stderr))
  self.assertIn("unexpected project QA network",x.stderr)

if __name__=="__main__":unittest.main()
