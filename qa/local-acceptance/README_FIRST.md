# Gói bàn giao Codex Local — CORE final QA và v1.3 (lịch sử)

## Nhiệm vụ Core hiện tại: một lượt test cuối

Dùng **qa/local-acceptance/CODEX_LOCAL_ONE_PASS_PROMPT.md** làm prompt mới nhất. Đọc **CORE_ONE_PASS_QA.md** để chạy tất cả các bài CORE-8B + regression + fault trên QA disposable được xác minh. Sau khi Codex tạo báo cáo, kiểm chứng bằng **verify_core_final_evidence.py --report qa-results/<UTC-RUN-ID>**. Không chạy benchmark 7 triệu dòng trong đợt này. Không tự reset volume nếu chưa xác nhận dữ liệu QA có thể xóa; nếu thiếu tài nguyên thì BLOCKED.

**Trạng thái:** Chỉ mới GitHub-hosted CI/E2E PASS cho mã nguồn; Codex Local QA trên máy người dùng **chưa chạy**. Giữ Production NO-GO.

## Hướng dẫn v1.3 cũ — chỉ dùng cho kiểm thử phạm vi E/L/T mở rộng

Dùng theo thứ tự:

1. Mở **`qa/local-acceptance/START_PROMPT_FOR_CODEX.md`** ngay trong repo và đưa cho Codex local. Không cần tải ZIP hay đính kèm file.
2. Cho Codex đọc **`qa/local-acceptance/CODEX_LOCAL_QA_MISSION.md`**. Đây là bản đặc tả chi tiết: tự dựng Docker QA, SQL Server mô phỏng Epicor, sinh dữ liệu, test E/L/T, benchmark, fix bug, report.
3. Cho Codex đọc **`qa/local-acceptance/TEST_CASE_CATALOG.md`** (68 test cases) và **`qa/local-acceptance/REPORT_TEMPLATE.md`**.
4. Codex phải tự sinh **02_TEST_MATRIX.csv** và các artifact/log/test report trong quá trình chạy. Template không phải kết quả đã chạy.
5. Khi Codex hoàn thành, upload **HANDOFF_TO_CHATGPT.md** và **DATAFLOW_LOCAL_QA_RESULTS.zip** vào ChatGPT để đối soát test và tư vấn fix tiếp.

**Không cần user chuẩn bị source test data**: Codex tự dựng fixture SQL Server + ClickHouse + PostgreSQL QA, tự seed và tính đáp án. Epicor TEST thật là nhánh read-only **tùy chọn**, chỉ làm khi được cấp phép.

**Repo đã đối chiếu:** https://github.com/dinhvansh/dataflow-engine · tham chiếu main SHA b7cfc40d3ca88f0c21ace3eb28adc361f705ce74. Codex phải xác minh lại Git state trước khi tiếp tục.

**An toàn:** SQL Server ERP thật chỉ SELECT; không seed/xóa vào máy ERP; Docker QA project riêng; không xóa container/volumes đang dùng; không ghi secrets vào báo cáo.