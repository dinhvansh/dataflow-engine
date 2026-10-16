# Recommendations

1. Review and commit BUG-001..004 fixes on the QA branch only after independent diff review.
2. Add browser E2E (Playwright) for UI-01..06 and retain screenshots/video.
3. Define a product policy for late events and a real tombstone/reconciliation path; SysRevID cannot capture physical deletes.
4. Add sampled CPU/RAM/IO collection and delta 1k/10k/50k plus three-trial recovery benchmarks.
5. Run Epicor TEST only with explicit authorization and a proven SELECT-only login, starting with metadata/limited reads.
6. Keep production status NO-GO until RBAC/audit/backup lifecycle and DBA/IT approval are complete.

