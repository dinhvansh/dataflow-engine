# DATAFLOW ENGINE — LOCAL QA / UAT / BENCHMARK MISSION

**Mission:** Codex Local tự dựng môi trường thử nghiệm tách biệt, tự tạo dữ liệu giả lập có đáp án chuẩn, chạy và sửa lỗi DataFlow Engine v1.3, sau đó trả lại một bộ bằng chứng để ChatGPT rà soát.

**Repo:** `https://github.com/dinhvansh/dataflow-engine`  
**Baseline tham chiếu lúc giao nhiệm vụ:** `main` commit `b7cfc40d3ca88f0c21ace3eb28adc361f705ce74` (phải kiểm tra lại `origin/main` tại thời điểm Codex bắt đầu).  
**Mục tiêu:** Kiểm thử local trên máy Windows, Docker Desktop; mô phỏng Epicor TEST bằng SQL Server **tách biệt**, ClickHouse và PostgreSQL mới; sau đó **tùy chọn** đọc Epicor TEST thật bằng tài khoản read-only, nếu người dùng cung cấp và cho phép.

## 0. Quy tắc bắt buộc: không phá hệ thống, không bịa kết quả

1. **PRE-FLIGHT GIT:** chạy `git status --porcelain`, `git branch --show-current`, `git fetch origin`, `git rev-parse HEAD`, `git rev-parse origin/main`, `git rev-list --left-right --count HEAD...origin/main`. Nếu working tree bẩn, HEAD diverge, thiếu migration hoặc có thay đổi mới: **báo và bảo toàn**, không reset/checkout/overwrite tùy tiện. Tạo nhánh thử nghiệm mới, ví dụ `test/local-qa-v14-YYYYMMDD`; không sửa/commit vào `main` và **không merge/push** nếu chưa được người dùng cho phép. Có thể commit fix trên nhánh cục bộ, kèm diff và chứng cứ.
2. **TÁCH BIỆT MÔI TRƯỜNG:** ưu tiên Docker Compose project riêng `dataflow-localqa`, network/volume riêng với tên đặc thù QA, database fixture khác hoàn toàn database nội bộ. Chỉ bind ports về `127.0.0.1`, kiểm tra port chưa bị dùng. Không đụng các container/volume đang chạy của Airbyte, CHouse UI, ClickHouse thật, Metabase, ERP hoặc ứng dụng công ty.
3. **EPICOR BẤT KHẢ XÂM PHẠM:** SQL Server Epicor TEST thật chỉ được `SELECT`, đọc catalog và các thủ tục read-only đã cho phép. **Không** `CREATE`, `INSERT`, `UPDATE`, `DELETE`, `MERGE`, `TRUNCATE`, `DROP`, `ALTER`, `DBCC` hoặc sửa isolation/cấu hình trên `SRVERPTEST / ERPDBTEST`. Mọi lệnh sinh dữ liệu/đổi dữ liệu/kiểm tra DELETE chỉ chạy trên **SQL Server Docker giả lập do Codex tự dựng**, sau khi xác thực tên database/instance và container ID thuộc project QA.
4. **KHÔNG DÙNG NHẦM SCRATCH:** vì test hiện có tạo/xóa bảng cố định trong `scratch`, chỉ chạy các integration tests với **ClickHouse và PostgreSQL QA rỗng vừa tạo**. Không bao giờ dùng biến `DATAFLOW_INTEGRATION_*` để trỏ tới ClickHouse/PG đang vận hành của người dùng. Với benchmark hoặc seed, tên mọi bảng phải có prefix `qa_` hoặc mô hình `dft_`/`dfd_` trong **QA database**; dọn bằng allowlist, không dùng `docker system prune`, `DROP DATABASE` của hệ thống cũ.
5. **TÁCH QUYỀN:** MSSQL app user read-only, seed admin chỉ vào fixture QA; PostgreSQL metadata user riêng; ClickHouse read-only cho preview/DQ và writer riêng chỉ được phép ghi `scratch`; dbt worker không có secret MSSQL; API token ngẫu nhiên. Không log/in nguyên mật khẩu, API token hay DSN có password. `.env.qa.local` phải được Git ignore.
6. **FEATURE FLAGS:** tất cả cờ nguy hiểm mặc định `false`. Chỉ bật trong compose/project QA và chỉ sau khi preflight/permissions PASS. Không bật `DATAFLOW_ALLOW_TEST_LOAD`, `DATAFLOW_ENABLE_INCREMENTAL_TEST`, `DATAFLOW_ENABLE_SAFE_TABLE_TEST` trên hệ thống thực.
7. **KHÔNG THAY TEST CHO XANH:** không bỏ/skips test, bỏ assertion, nâng timeout tùy tiện, ép pass, fake logs hay sửa expected result chỉ để CI xanh. Nếu không chạy được phải ghi `BLOCKED`, lý do, log. Tách rõ `PASS synthetic`, `PASS Epicor TEST`, `NOT TESTED` và `FAIL`.
8. **HARD STOP:** dừng ngay các thao tác có khả năng thay đổi ERP thật, target không phải QA, credential không giới hạn đúng mục đích, sai database, hay nguy cơ mất dữ liệu. Ghi P0 và thông báo người dùng trước khi tiếp tục. Không tự ý đụng production.

## 1. Đầu ra cuối cùng bắt buộc

Codex phải tạo folder `qa-results/<RUN_ID>/` trong repo local, ví dụ `qa-results/20261016-qa001/`, Git-ignored, với tối thiểu:

| Tệp | Nội dung / điều kiện |
|---|---|
| `00_EXECUTIVE_SUMMARY.md` | Verdict **PASS / CONDITIONAL PASS / FAIL / BLOCKED**, môi trường chạy, commit, số test pass/fail/blocked, P0/P1, rủi ro và quyết định Go/No-Go |
| `01_ENVIRONMENT.md` | OS/CPU/RAM/Disk (không chứa ID cá nhân), WSL2/Docker/Go/Node/Python/dbt/SQL Server/CH/PG versions, ports, compose project, memory limit, giả định, cờ đã bật |
| `02_TEST_MATRIX.csv` | Mỗi test ID, mô tả, dataset, expected, actual, PASS/FAIL/BLOCKED, run time, evidence file, bug ID |
| `03_DATA_RECONCILIATION.md` | Count/PK/NULL/decimal/datetime/sample hash/aggregate nguồn vs đích; số mismatch có thể tái lập |
| `04_PERFORMANCE.md` và `perf.csv` | Mỗi lần chạy latency, rows/s, MiB/s, CPU/RAM/disk/IO nếu đo được, retries, source impact; ít nhất 3 trial cùng cấu hình; median/range |
| `05_BUG_REGISTER.md` | Bug ID, P0–P3, steps reproduce, expected/actual, log và root cause, file/commit fix, regression test, retest, unresolved |
| `06_SECURITY_AND_RECOVERY.md` | Bằng chứng MSSQL source read-only, isolation, secrets redaction, lease/retry/crash/rollback/cursor safeguards |
| `07_UI_EVIDENCE.md` | Các thao tác UI, ảnh màn hình có timestamp và status, chức năng không kiểm thử được |
| `08_RECOMMENDATIONS.md` | Fix kế tiếp theo thứ tự ưu tiên, blockers đối với Epicor TEST/production, giới hạn CI/bench |
| `09_COMMANDS_AND_VERSIONS.md` | Lệnh từng chạy, output exit code, seed/script version, git diff/commit list (đã redacted) |
| `artifacts/` | stdout/stderr test, run IDs, ảnh, JSON API response (đã xoá secrets), timing, số liệu xác thực, checksum |
| `repro/` | Script môi trường, seed/delta, kiểm tra DB, integration/regression và benchmark để có thể chạy lại |

Thêm **đầu ra có thể giao cho ChatGPT trong chat**: một file `HANDOFF_TO_CHATGPT.md`, nội dung tối đa ~200–400 dòng, chỉ rõ verdict, blockers, top bugs, link/path các file evidence, git commit/branch, benchmark tóm tắt, mismatch và lệnh tái hiện. Nếu không thể đính kèm đầy đủ, gói ZIP `DATAFLOW_LOCAL_QA_RESULTS.zip` chứa các báo cáo/log đã khử bí mật và các script tái hiện. Không chứa `.env` thật, DB dump chứa dữ liệu công ty hoặc password/token.

## 2. Tự dựng hạ tầng (Codex phải thực hiện, không chờ người dùng tự setup)

### 2.1 Preflight máy

- Kiểm tra Docker Desktop/WSL2, Docker Compose v2, Go >=1.23, Node >=22, npm, Git, PowerShell, Python 3.12 (nếu cần), khoảng trống đĩa và RAM thực tế.
- Nếu thiếu runtime, thử cài bằng phương thức đã có trên máy **chỉ khi không yêu cầu quyền administrator ngoài phạm vi đã cấp**. Nếu cần quyền nâng cao/đổi policy hệ thống, báo `BLOCKED-ENV` và hướng dẫn ngắn; không tự thay đổi Windows Firewall/DLP/domain/proxy công ty.
- Đọc `README.md`, `.env.example`, `docker-compose*.yml`, migrations `001..014`, tài liệu `testing/acceptance.md`, `testing/benchmark.md`, `implementation/release-status-v13.md`, và kiểm tra code worker/API trước khi chọn flags.
- Ghi baseline git SHA, `go version`, `node --version`, `docker version`, CPU/RAM/disk/port. Kiểm tra container hiện tại để tránh collision.

### 2.2 Tạo stack riêng hoàn chỉnh

Tạo trong nhánh QA (không sửa cấu hình production):

```
deploy/local-qa/
  compose.qa.yml              # SQL Server fixture + PostgreSQL 16 + ClickHouse 25.3 + dbt-worker
  .env.qa.example             # chỉ placeholder, không chứa secrets
  create-env.ps1              # tạo secrets cục bộ + kiểm tra ports/labels
  up.ps1                      # start/healthcheck/init/migrations/seed
  down.ps1                    # stop đúng project QA; không xóa volume nếu chưa được đồng ý
  seed-fixtures.ps1           # verify container identity rồi seed synthetic MSSQL
  run-smoke.ps1
  run-functional.ps1
  run-faults.ps1
  run-benchmark.ps1
  collect-results.ps1
  sql/                        # only synthetic fixture SQL and oracle queries
  manifests/                  # seed row count, checksums, schema/permission contracts
```

Sử dụng project name đặc thù `dataflow-localqa`, dedicated Docker network + volumes + labels `com.dataflow.role=local-qa`, `com.dataflow.owner=dataflow-localqa`; kiểm tra labels/ID trước khi seed, fault injection, drop hoặc cleanup. Có thể bổ sung một container *API* vào compose hoặc chạy Go trên host (nếu host, tạo local env riêng với `localhost` ports); Web UI có thể chạy qua Vite trên host. Đảm bảo `dbt-worker` thấy PostgreSQL và ClickHouse theo service DNS của cùng network, tránh dùng `localhost` trong container.

Phải **tự triển khai SQL Server fixture** bằng `mcr.microsoft.com/mssql/server:2022-latest` hoặc tag ổn định tương thích của Microsoft; `ACCEPT_EULA=Y`, strong password sinh ngẫu nhiên; Docker Desktop cần đủ RAM để SQL Server Linux khởi động. PostgreSQL 16-alpine, ClickHouse 25.3 (như CI), worker Dockerfile repo với dbt Core==1.11.0, dbt-clickhouse==1.10.3; không nâng ngẫu nhiên phiên bản để né lỗi. SQL Server test nên giới hạn CPU/RAM riêng; không làm nghẽn dịch vụ đang chạy. Bắt buộc có readiness/healthcheck thực sự (SQL query), migrations tuần tự với cơ chế tránh áp dụng lặp khi volume cũ. ClickHouse database `scratch` phải là `Atomic`.

**Port gợi ý (phải kiểm tra rảnh):** `127.0.0.1:11433→1433` (SQL Server fixture), `127.0.0.1:15433→5432` (PostgreSQL), `127.0.0.1:18123→8123` (ClickHouse), `127.0.0.1:18080` (Go API), `127.0.0.1:15173` (Vite). Nếu port xung đột thì tự chọn cổng rảnh và ghi báo cáo; **không stop container hiện hữu để lấy port**.

Môi trường cần có ba vai trò rõ ràng:

1. SQL Server `DataFlowFixtureQA` có schema `Erp` và seed SQL login `qa_seed_admin` chỉ cho fixture; `qa_erp_reader` chỉ có SELECT, không có quyền DML/DDL. Phần seed dùng quyền riêng và tắt/xóa credential khỏi tiến trình API.
2. ClickHouse `scratch` QA: tài khoản `qa_ro` SELECT và `qa_dbt_writer` được tạo bảng/View/EXCHANGE cần thiết *trong `scratch` QA*, không có ERP credentials. Test Go integration **chỉ trỏ vào instance QA rỗng**; account writer test có thể rộng hơn bên trong container QA disposable, ghi rõ giới hạn.
3. PostgreSQL `dataflow_qa` với metadata, migrations 001–014 theo thứ tự và audit tối thiểu; không chia sẻ với PG thật.

### 2.3 Chặn sai môi trường trước khi chạy mỗi nhóm test

Script preflight phải kiểm tra đồng thời: project/container labels, host URL/port, SQL Server `DB_NAME()='DataFlowFixtureQA'`, database ID test, ClickHouse `currentDatabase`/`system.databases.engine='Atomic'` + hostname/container ID, PG database/user là QA, không có host/DSN dạng `SRVERPTEST`, `ERPDBTEST`, hoặc credentials production khi seed. Không được chỉ tin vào tên biến môi trường. Thử quyền đọc nguồn bằng query nhỏ và xác thực quyền seed account không được truyền cho Go API.

### 2.4 Cờ cho QA (đọc code để xác nhận điều kiện)

- API cần `DATAFLOW_POSTGRES_DSN`, `DATAFLOW_SQLSERVER_DSN`, `DATAFLOW_CH_URL`, `DATAFLOW_API_TOKEN` và `PORT` (mặc định 8080).
- Các cờ test: `DATAFLOW_ALLOW_TEST_LOAD=true`, `DATAFLOW_ENABLE_MANAGED_TEST=true` (chỉ sau khi smoke), `DATAFLOW_ENABLE_SQL_JOBS_TEST=true`, `DATAFLOW_ENABLE_DBT_TEST=true`, `DATAFLOW_ENABLE_SAFE_TABLE_TEST=true`, `DATAFLOW_ENABLE_AUTO_BUILD_TEST=true`, `DATAFLOW_ENABLE_INCREMENTAL_TEST=true`, `DATAFLOW_TEST_TARGET_DATABASE=scratch`.
- Preview/DQ cần `DATAFLOW_CH_READONLY_USER/PASSWORD`; worker cần `DATAFLOW_POSTGRES_DSN`, `DATAFLOW_DBT_CH_HOST/PORT/USER/PASSWORD`, và các flags tương ứng.
- **Bật theo pha** chứ không bật tất cả ngay trước khi migrations/safety checks PASS. Nếu API không boot, ghi startup logs, nguyên nhân và bước sửa; không bỏ qua check schema.
- `DATAFLOW_INTEGRATION_POSTGRES_DSN`/`DATAFLOW_INTEGRATION_PG_ISOLATED=true` + `DATAFLOW_INTEGRATION_CH_URL/USER/PASSWORD` chỉ dùng cùng test cluster QA disposable. Không bao giờ sử dụng DSN nguồn ERP ở các biến có test ghi DDL.

## 3. DỮ LIỆU GIẢ LẬP — Codex tự sinh có oracle định sẵn

### 3.1 SQL Server mô phỏng Epicor

Trong **duy nhất** database giả lập `DataFlowFixtureQA`, tạo schema `Erp`, bảng đủ cho connector đọc metadata/full/rowversion, tối thiểu:

- `Erp.Part`: `Company`, `PartNum`, `PartDescription`, `SysRowID uniqueidentifier`, `SysRevID rowversion`, nullable/unicode.
- `Erp.PartTran`: `Company`, `PartNum`, `TranDate`, `SysDate`, `TranType`, `TranQty decimal(18,6)`, `ExtCost decimal(18,4)`, `SysRowID uniqueidentifier`, `SysRevID rowversion` và vài cột optional. Kiểm tra source metadata thực tế để điều chỉnh (không suy đoán rằng mọi bản Epicor đều giống nhau).
- `Erp.InvcHead` / `Erp.InvcDtl` ở mức tối thiểu để thử Join, GroupBy và mô hình Revenue. Khóa ổn định, khóa ngoại và dữ liệu phù hợp; kiểu Date/Decimal/Nullable/Unicode có cả extreme values hợp lệ.
- `qa.control_events` / `qa.expected_*` là bảng **oracle của test fixture**, không phải bảng Epicor thật. Có danh sách seed ID, expected latest state, deleted keys, expected sum/row counts per scenario.

Seed bằng script deterministic có random seed cố định `20261016`; chọn GUID/PK unique và source `SysRevID` rowversion thật (không giả mạo literal bytes tùy tiện); tránh insert thủ công `SysRevID`. Theo dõi `SysRevID` theo raw binary 8 byte đúng format ứng dụng. Dữ liệu chuẩn gồm: 0/NULL/negative quantities, decimal chính xác, Unicode Tiếng Việt, date backdated/forwarddated, trùng tên Part nhưng khác key, vài hàng có NULL, các ngày cuối tháng/năm, customer/lot dữ liệu hợp lệ.

### 3.2 Quy mô fixture

| Bộ | Số dòng mục tiêu | Dùng để làm gì | Gate |
|---|---:|---|---|
| `SMOKE` | 50–200 | Connect/schema/startup/UI | Bắt buộc |
| `CORRECTNESS` | 10.000 | Full, cursor, aggregate, crash/retry | Bắt buộc |
| `SCALE_S` | 100.000 | Pipeline, worker config, repeatability | Bắt buộc khi máy đủ tài nguyên |
| `SCALE_M` | 1.000.000 | Full throughput, RAM/CPU, disk | Có điều kiện sau resource preflight |
| `SCALE_L` | 6.945.639 hoặc ~7 triệu | So sánh gần `Erp.PartTran` | **Tùy chọn** nếu máy đủ RAM/disk/time và được người dùng chấp thuận; không tự phá máy |

**Không đưa >1 triệu dòng vào Safe Table v1.1/v1.3**, vì chúng có giới hạn TEST; Incremental mỗi cửa sổ delta tối đa 50.000 sự kiện. E/L benchmark có thể dùng nhiều hơn nếu bộ Extract/Load hỗ trợ và tài nguyên cho phép. Nên seed bằng batch có kiểm soát, theo dõi SQL Server tempdb/log size, tránh log growth vô hạn. Không seed toàn bộ scale trước khi PASS 10k.

### 3.3 Các sự kiện kiểm thử phải sinh được

- Baseline: bộ PK duy nhất, expected `COUNT(*)`, `COUNT(DISTINCT PK)`, date buckets, tổng quantity/cost/revenue ở precision chuẩn.
- Insert mới: +10, +100, +1.000 record (tương ứng với fixture) có rowversion lớn hơn checkpoint.
- Update: sửa Amount, Qty, Nullable, Unicode; sửa **cùng key nhiều lần** trước khi sync; update trong lúc pagination thử isolation.
- Delete: **chỉ ở fixture synthetic**, tạo tombstone `_df_deleted=1` trong ClickHouse delta; và thử xóa vật lý trên SQL Server synthetic để chứng minh rowversion **không tự phát hiện DELETE**. Đánh dấu đó là giới hạn sản phẩm, không tự nhận PASS real CDC.
- Ordering: late arrival, equal version (phải FAIL closed), gap version, out-of-order commit/long transaction; các trường hợp không có bảo đảm từ source phải ghi rõ BLOCKED/KNOWN LIMITATION thay vì đoán.
- Data shape: duplicate key trong delta, NULL key (reject), duplicate global `_df_version` (reject), out-of-range event limit >50k (reject), all-deleted target → valid empty staging, no-change job.
- Concurrency/fault: duplicate job submissions, API restart, worker stop/restart, ClickHouse unavailable, PG unavailable, revoke scratch writer role, timeout/cancel, staged DQ fail; tạo fault chỉ bằng container và tài khoản QA.

### 3.4 Các oracle dữ liệu bắt buộc

Codex phải tạo `fixtures_manifest.json` có: seed version, source table, schema types, expected row counts, PK set hash, per-bucket aggregate values, 20–100 stable sample keys, expected after each delta window, expected after rollback. Không so sánh số liệu chỉ bằng `COUNT(*)`: thêm phép đối soát theo khóa, min/max cursor, null count, decimal exact, sample record fields, sums grouped by Company/PartNum/TranDate và checksum **canonicalized** (định nghĩa một lần cho SQL Server và ClickHouse, lưu cách normalize). Nếu không thể định nghĩa checksum tương đương cross-DB do timezone/collation, so sánh dữ liệu canonical trên client, nêu rõ quy tắc; không tuyên bố checksum bằng nhau khi chỉ hash format khác nhau.

## 4. MA TRẬN TEST BẮT BUỘC VÀ KẾT QUẢ MONG ĐỢI

Mỗi ID phải có actual evidence; **P0/P1 chưa sửa = NO-GO**. Tham khảo chi tiết `TEST_CASES.csv` đi kèm.

### A. Env/Git/Security (`ENV-01..08`)

- `ENV-01` Git branch, SHA, dirty status, divergence: phát hiện và không hủy thay đổi.
- `ENV-02` stack QA mới, container labels/network/volumes, ports localhost-only; health xanh.
- `ENV-03` migrations 001–014 chạy đúng thứ tự, kiểm tra idempotence/khởi động lại; không tự chạy migration trên DB hệ thống.
- `ENV-04` Go API `/healthz` 200 và SQL Server/PG/CH kết nối thực sự; health endpoint **không thay thế** connectivity proof.
- `ENV-05` React build + UI load + API token; unauthorized API 401, flags OFF 403.
- `ENV-06` SQL Server app login có quyền SELECT nhưng không có DML/DDL; dùng metadata/grants read-only. Việc chủ động thử UPDATE chỉ được phép trên fixture QA với record sentinel và transaction rollback.
- `ENV-07` lỗi bật flags/thiếu migrations fail rõ ràng, không rơi vào silent success.
- `ENV-08` secrets không xuất hiện trong logs/artifacts; không làm lộ nội dung ERP qua báo cáo.

### B. Extract/Load – MSSQL → ClickHouse (`EL-01..12`)

- Full refresh 10k: exact count, unique PK, sample values, decimal/date/null/unicode đúng; đếm mismatch **0**.
- Lặp full: không nhân đôi logical rows; atomic_replace giữ target cũ nếu fail trước Publish.
- Rowversion insert/update: kiểm tra upper exclusive, binary cursor, thứ tự/phân trang SysRowID/PK; không bỏ sót/double logic rows.
- Managed Sync: retry/checkpoint/contiguous ACK; no-change zero new events; source permission SELECT-only.
- DELETE vật lý: nhận diện giới hạn rowversion; nếu app không có giải pháp thì **KNOWN GAP** chứ không tuyên bố CDC DELETE PASS.
- Snapshot isolation: nếu SQL Server synthetic bật được và code hỗ trợ, thử snapshot consistent; trên Epicor thật chỉ đọc trạng thái cấu hình, không bật qua DataFlow.
- Fault: kill only QA worker, block test network, ClickHouse INSERT fail; checkpoint không tiến khi write fail, stale worker không chốt status, recovery không làm sai latest logical state.
- Scalability: 10k/100k/1m; không dùng PartTran 7m nếu vượt ngưỡng đã thống nhất.

### C. Transform, dbt, Lineage, Auto Build (`T-01..09`)

- SQL transform Visual: Select, Filter, Rename/Calculate/Aggregate/Limit trong phạm vi code thực sự hỗ trợ; Preview trả expected; bất kỳ loại step nào không hỗ trợ phải ghi GAP.
- SQL editor: SELECT-only; chặn DDL/DML/Jinja injection theo validation hiện tại; không mở rộng quyền test.
- dbt View: tạo `scratch.dfv_*`, trả run_results/status persisted và SELECT output đúng.
- Lineage: source→A→B→C, Impact descendants đúng, cycle A→B→C→A bị từ chối atomic transaction.
- Auto Build: bấm **chỉ C**, chạy A→B→C theo thứ tự; DQ count ở từng bước, không mở C khi B fail, SQL ancestor đổi giữa chừng phải STOP.
- API restart giữa A/B: chạy tiếp từ PostgreSQL không queue child lần hai.
- Kiểm tra **giới hạn View**: View có thể nhìn thấy trước DQ, không hứa multi-model atomic release.

### D. Safe Table (`TAB-01..07`)

- Build staging trong `scratch` không đổi bảng đang được đọc; chỉ sau kiểm tra mới status `ready`.
- DQ nonempty, max 1m rows, optional unique key; lỗi DQ không Publish.
- Publish đòi confirm, latest saved SQL không drift; EXCHANGE trong Atomic DB.
- Thử hai phiên bản 100→200; replay sau HTTP timeout/crash KHÔNG swap ngược; old physical UUID còn ở stage để rollback.
- Rollback trả dữ liệu cũ, sai UUID hoặc publication mới hơn thì từ chối.
- Không tự publish các bảng trong Auto Build View v1.2.

### E. Safe Incremental (`INC-01..10`)

- Delta chuẩn là payload target + `_df_version UInt64`, `_df_deleted UInt8`; global version tăng và duy nhất.
- A update nhiều lần → bản mới nhất; B tombstone → xóa; D insert → một row duy nhất.
- Hai window `0→4` và `4→7`: trước publish checkpoint không tăng; sau publish checkpoint đúng; không có đổi dữ liệu target trước Publish.
- Replay build, retry publish và restart: không nhân dòng, không swap ngược.
- Rollback version hai: phục hồi chính xác bảng version một **và cursor 7→4**; xây lại window 4→7 không trùng.
- Bad key, duplicate version, late event thấp hơn checkpoint, invalid flags, count >50k, changed baseline UUID, no_changes, all-deletes.
- **Gap về late arrivals/SQL Server physical DELETE phải ghi rõ**. Không báo PASS native Epicor CDC chỉ vì fixture có tombstone.

### F. UI / UX (`UI-01..06`)

- Trên browser thật nếu Codex có browser automation: Source browser, Create/Preview/Save Transform, node drag-drop, Run History, Lineage node click, Auto Build C once, Safe Table Approve/Rollback, Incremental status.
- Theo dõi realtime/status refresh, clear errors, responsive width và labels tiếng Việt; chụp screenshot.
- Không có browser automation thì chạy build + API integration và ghi UI cases **BLOCKED-BROWSER**, không tự nhận PASS UI.

### G. Recovery / Security (`REC-01..07`)

- Crash ở trước/sau ACK ClickHouse, trước/sau PG checkpoint, trước/sau Atomic EXCHANGE; khôi phục checkpoint đúng và old table giữ nguyên khi chưa publish.
- Partial commit và duplicate attempts: chứng minh logical latest data không sai, lease fencing từ chối stale worker; log run ID và snapshot.
- Postgres crash và CH outage: hệ thống báo failed/queued có thể phục hồi, không giả success, không làm cursor tiến.
- Sai quyền writer hoặc wrong database: fail closed; không spill secrets trong error.
- Cleanup chỉ run-owned QA resources, không xóa backup khi chưa ghi evidence.

### H. Hiệu năng & so sánh (`PERF-01..07`)

- Warm/no-change, cold start, full 10k/100k/1m; delta 1k/10k/50k, retry sau crash; đo end-to-end với cùng query projection, batch size, workers, CPU/RAM/memory limits.
- Mỗi workload ít nhất **3 lần** sau warm-up, ghi từng trial rồi median/range, peak RSS, CPU, SQL Server/CH server impact. Nếu không đo được CPU/IO thì ghi `N/A` và lý do.
- Airbyte baseline chỉ khi người dùng có Airbyte TEST hoạt động và cùng nguồn/dích; không cài Airbyte lên instance đang chạy, không dùng thông số nghe kể, không bịa speedup.
- Cần thêm correctness: performance PASS chỉ khi mismatch=0 và recovery hợp lệ. Một lượt nhanh nhưng mất record là FAIL.
- Đối với mức ~7m, trước tiên đo memory/disk 1m và xin phép người dùng nếu benchmark có thể làm chậm máy; không tự chạy khi nguồn ERP TEST thiếu consent hoặc tài nguyên không đủ.

## 5. Cách xử lý BUG — Codex tự sửa trong nhánh QA và retest

**P0 Critical:** viết nhầm vào ERP/DB ngoài QA, mất bảng dữ liệu thật, tiến cursor khi publish thất bại, sai UUID swap làm hỏng published target, lộ secrets → dừng ngay, cô lập QA và báo người dùng; không tự merge.

**P1 High:** mất/nhân/trùng dòng logical, sai decimal/date/PK, skip delta, DELETE không đúng với contract fixture, rollback sai checkpoint, crash báo success giả, data lineage chạy sai thứ tự → block acceptance. Viết test đỏ tái hiện, fix nguyên nhân, chạy lại test riêng + full regression + đối soát 0 mismatch.

**P2 Medium:** API/UI workflow lỗi không làm sai data, lỗi thông báo, retry chưa tốt, memory leak vừa phải, chậm hơn target đo được → fix nếu đủ thời gian, nếu không cần ghi workaround/repro và ưu tiên.

**P3 Low:** UX/cosmetic/log clarity → ghi và có thể defer.

Với MỖI bug, Codex phải:

1. Ghi `BUG-xxx`, severity, môi trường, precondition, seed, câu lệnh ngắn nhất, expected/actual, failing log có timestamp/trace/run ID.
2. Khoanh vùng code/file, cơ chế gây lỗi (root cause) dựa trên bằng chứng, không đoán mò. **Thêm regression test thất bại trước khi sửa** (nếu tái lập được).
3. Sửa trên **nhánh local test**, không thay data oracle/expected để hợp thức hóa code sai. Hạn chế diff nhỏ, giữ backward compatibility và flags test-only.
4. Chạy lại failing test → module tests → `go test -race ./...`, `go vet ./...`, `go build ./cmd/...`, `npm run build` → E2E affected → so sánh count/hash/cursor; log commit SHA.
5. Chụp evidence trước/sau, cập nhật bug `FIXED + VERIFIED` hoặc `OPEN/REPRODUCIBLE/BLOCKED`. Không xóa log thất bại gốc.
6. Nếu fix làm mất khả năng rollback/vi phạm quyền source, revert fix cục bộ rồi báo. Không merge/push main khi chưa được user đồng ý.

## 6. Test Epicor TEST thật — TÙY CHỌN, CHỈ ĐỌC

Chỉ thực hiện **sau synthetic suite PASS** và khi người dùng xác nhận có quyền thử tải đối với `SRVERPTEST` / `ERPDBTEST`. Tự kiểm tra DSN thực tế; không giả định host là localhost.

1. Chỉ dùng login SQL Server read-only. Lấy metadata, cột và PK/cursor của `Erp.PartTran` và bảng mẫu khác; kiểm tra `SysRowID`, `SysRevID`, `TranDate`, `SysDate`, `TranType`, decimal & nullable, snapshot isolation bằng `SELECT` vào `sys.databases`.
2. Read-only preview 10/100 rows rồi `COUNT(*)`/window read có giới hạn. Bắt đầu 1k → 10k → 100k, tách ClickHouse QA output; không tạo staging hay temp table bên SQL Server ERP.
3. Kiểm tra query plan/impact SQL Server theo quyền đọc có sẵn; không chạy `UPDATE STATISTICS`, `ALTER INDEX`, DBCC hay DML.
4. Full 7m chỉ khi có thời gian phù hợp, source impact chấp nhận được, tài nguyên máy đủ và người dùng đã chấp thuận. Báo thời gian, số dòng, checkpoint, mismatch, throughput — **không suy từ synthetic benchmark**.
5. SysRevID không thay cho nguồn tombstone DELETE; nếu không có CDC upstream, ghi rõ hạn chế và hướng giải quyết đề xuất (snapshot reconciliation/SQL CDC nếu DBA cho phép).
6. Nếu access bị DLP/gateway/firewall chặn, ghi `BLOCKED` và evidence; không thay đổi corporate policy, không lách bảo mật.

## 7. Tiêu chí nghiệm thu và phán quyết

**PASS (synthetic local)** khi: nguồn/đích cô lập, metadata/migrations/permissions đúng, unit/integration/E2E chạy xanh hoặc documented nonapplicable, **mọi scenario core 10k exact match** (count, PK, decimal, state), dbt/Auto Build/DQ/Safe Table/Incremental approve+rollback hoạt động như contract, crash/retry không làm nhảy cursor/mất bảng cũ, P0/P1 = 0, report và evidence đầy đủ.

**CONDITIONAL PASS** khi các core correctness/safety PASS nhưng PERF lớn, UI browser hoặc Epicor TEST thật chưa chạy; phải ghi rõ phần chưa chạy và điều kiện trước production.

**FAIL** khi bất kỳ mismatch logical/financial, P0/P1 open, invalid source write permission, Cursor/Publish/Rollback không an toàn, test fixture/lệnh không tái lập được. **BLOCKED** khi thiếu quyền, Docker tài nguyên hoặc corporate network, và phải báo phần nào đã PASS.

**PRODUCTION READY = NO** cho dù synthetic PASS; muốn production phải có Epicor TEST real acceptance, CDC/deletes plan, benchmark, audit/RBAC, bảo trì backup, staging/publish isolation, xác nhận của DBA/IT Owner.

## 8. Lệnh kiểm thử chuẩn (Codex tự xác thực port và shell)

```powershell
# Run trong nhánh QA, KHÔNG sử dụng DSN ngoài disposable QA containers.
git status --short
git fetch origin
git rev-parse HEAD
go mod tidy
go test -race ./...
go vet ./...
go build ./cmd/...
cd web
npm ci
npm run build
cd ..

# Full integration test runner phải tạo service QA, seed, export vars QA,
# rồi chạy các case CI có dependency. CI reference: .github/workflows/ci.yml
# Nếu Go race có hạn chế môi trường Windows, chạy trong WSL2/container Go 1.23+.
```

Codex phải cung cấp lệnh **một nút** `./deploy/local-qa/up.ps1`, `./deploy/local-qa/run-functional.ps1` và `./deploy/local-qa/collect-results.ps1` (có thể thay tên nhưng phải ghi README). Scripts phải exit nonzero khi hard gate thất bại, xuất report máy-đọc được, và dừng sạch đúng project QA.

---

**Final instruction to Codex:** Tiến hành tự động tới mức có thể trên local: preflight → dựng toàn bộ QA sandbox → seed dữ liệu có oracle → run tests → phân tích/fix bugs trên test branch → retest → benchmark có giới hạn → xuất ZIP + `HANDOFF_TO_CHATGPT.md`. Không hứa hẹn hay báo PASS nếu chưa thực sự chạy. Chỉ đặt câu hỏi cho user nếu cần cấp quyền elevated hoặc muốn kết nối ERP TEST thật; nếu không, chủ động chạy synthetic suite.