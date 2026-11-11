# syntax=docker/dockerfile:1
FROM golang:1.27.3-bookworm@sha256:d9b6a1beb9474adc2a1ce842573639b44b2a0dd49b6669f51d3821baf9ec37c9 AS build-base
WORKDIR /src
ENV CGO_ENABLED=0 GOMAXPROCS=2

FROM build-base AS build
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go build -p 2 -trimpath -tags timetzdata -ldflags="-s -w" -o /out/ ./cmd/...

FROM build-base AS goose-build
# Keep only the PostgreSQL driver; this tool is not an application dependency.
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go install -p 2 -tags 'no_clickhouse no_libsql no_mssql no_mysql no_sqlite3 no_vertica no_ydb' \
    github.com/pressly/goose/v3/cmd/goose@v3.28.0

FROM scratch AS runtime
COPY --from=build-base /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
USER 65532:65532
WORKDIR /app

FROM runtime AS app
COPY --from=build /out/ /app/
EXPOSE 8080 8082
CMD ["/app/api"]

# Explicit, one-off migration command. API and worker never migrate on startup.
FROM runtime AS migrate
COPY --from=goose-build /go/bin/goose /app/goose
COPY db/migrations/ /app/migrations/
ENTRYPOINT ["/app/goose", "-dir", "/app/migrations"]
CMD ["status"]
