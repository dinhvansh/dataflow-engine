# Experimental dbt worker — deliberately separate from Go API and ERP connectors.
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN go mod tidy && CGO_ENABLED=0 go build -trimpath -o /out/dataflow-dbt-worker ./cmd/dataflow-dbt-worker

FROM python:3.12-slim
ENV PYTHONDONTWRITEBYTECODE=1 PIP_DISABLE_PIP_VERSION_CHECK=1 DBT_SEND_ANONYMOUS_USAGE_STATS=false
RUN python -m pip install --no-cache-dir dbt-core==1.11.0 dbt-clickhouse==1.10.3 && \
    useradd --create-home --uid 10001 --shell /usr/sbin/nologin dataflow
COPY --from=build /out/dataflow-dbt-worker /usr/local/bin/dataflow-dbt-worker
USER 10001:10001
WORKDIR /home/dataflow
ENTRYPOINT ["/usr/local/bin/dataflow-dbt-worker"]
