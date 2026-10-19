"""Regression: evidence must describe local, hosted and unspecified runners."""
import pytest
from qa_environment import evidence_environment


@pytest.mark.parametrize(("environ", "expected"), [
    ({"DATAFLOW_QA_EXECUTION_ORIGIN": "owner-local"}, "owner-local"),
    ({"GITHUB_ACTIONS": "true"}, "GitHub Actions"),
    ({"GITHUB_ACTIONS": "false"}, "unspecified"),
    ({}, "unspecified"),
])
def test_runner_origin_is_observed(environ, expected):
    assert evidence_environment(environ) == (
        "owned disposable four-service QA; runner=" + expected
    )
