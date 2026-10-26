"""A Docker-reported successful down is not enough if QA volumes remain."""

import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[3]
RESET = ROOT / "deploy" / "local-qa" / "reset-owned-qa.sh"

FAKE_DOCKER = """#!/bin/sh
case "$1 $2" in
  "compose "*) exit 0;;
  "ps -aq"*) exit 0;;
  "volume ls"*) echo dataflow-localqa_ch-data; exit 0;;
  "network ls"*) exit 0;;
esac
echo "Unexpected Docker command: $*" >&2
exit 98
"""


class ResetPostconditionTest(unittest.TestCase):
    def test_remaining_owned_volume_blocks_false_success(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            scripts = root / "deploy" / "local-qa"
            scripts.mkdir(parents=True)
            reset = scripts / RESET.name
            shutil.copyfile(RESET, reset)
            reset.chmod(0o700)
            guard = scripts / "verify-reset-scope.sh"
            guard.write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
            guard.chmod(0o700)
            docker = root / "bin" / "docker"
            docker.parent.mkdir()
            docker.write_text(FAKE_DOCKER, encoding="utf-8")
            docker.chmod(0o700)
            (root / ".env.qa.local").write_text("QA_ONLY=true\n", encoding="utf-8")

            bash = "bash"
            if os.name == "nt":
                git_bash = Path(os.environ.get("PROGRAMFILES", r"C:\Program Files")) / "Git" / "bin" / "bash.exe"
                if git_bash.exists():
                    bash = str(git_bash)
            env = os.environ.copy()
            env["PATH"] = str(docker.parent) + os.pathsep + env.get("PATH", "")
            env["DATAFLOW_QA_ALLOW_DISPOSABLE_RESET"] = "I_APPROVE_OWNED_QA_VOLUMES"
            result = subprocess.run(
                [bash, str(reset)], cwd=root, env=env, text=True, capture_output=True, check=False
            )
            self.assertEqual(result.returncode, 1, (result.stdout, result.stderr))
            self.assertIn("QA reset incomplete", result.stderr)


if __name__ == "__main__":
    unittest.main()
