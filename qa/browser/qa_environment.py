"""Describe the actual QA runner without assuming a hosted environment."""
import os


def evidence_environment(environ=None):
    values = os.environ if environ is None else environ
    if values.get("GITHUB_ACTIONS") == "true":
        runner = "GitHub Actions"
    elif values.get("DATAFLOW_QA_EXECUTION_ORIGIN") == "owner-local":
        runner = "owner-local"
    else:
        runner = "unspecified"
    return f"owned disposable four-service QA; runner={runner}"
