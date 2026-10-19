import importlib.util
import pathlib
import tempfile
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location("evidence",pathlib.Path(__file__).with_name("verify_core_final_evidence.py"))
m=importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)

class EvidenceTest(unittest.TestCase):
 def test_contract_is_static_and_requires_lifecycle_assertions(self):
  self.assertEqual(m.contract()["contract"],"PASS")
 def test_missing_report_refused(self):
  with self.assertRaises(ValueError):
   m.verify("../unowned")
 def test_7m_is_never_a_required_pass(self):
  self.assertNotIn("SCALE-7M",m.MANDATORY)
  self.assertEqual(set(m.LIVE),set(m.REQUIRED_JSON))
 def test_traversal_refused(self):
  with self.assertRaises(ValueError):
   m.contained_evidence("../../etc/passwd")
 def test_incomplete_matrix_blocked(self):
  with tempfile.TemporaryDirectory() as folder:
   p=pathlib.Path(folder)
   root=p/"qa-results";root.mkdir()
   rd=root/"trial";rd.mkdir()
   (rd/"02_TEST_MATRIX.csv").write_text("id,status,actual,elapsed_s,evidence\nSCALE-7M,NOT_RUN,,,\n")
   with patch.object(m,"ROOT",p):
    with self.assertRaises(ValueError):
     m.verify(rd)

if __name__=="__main__":unittest.main()
