# syntax=docker/dockerfile:1.7
FROM node:22-alpine AS web
WORKDIR /src
COPY web/package.json web/package-lock.json ./web/
RUN cd web && npm ci --no-audit --no-fund
COPY internal/i18n/locales ./internal/i18n/locales
COPY web ./web
RUN cd web && npm run build

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/out ./internal/webui/dist
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}" \
    -o /out/lip ./cmd/lip

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S lip \
    && adduser -S -G lip -h /app lip \
    && mkdir -p /app /data/imports \
    && chown -R lip:lip /app /data
COPY --from=build /out/lip /usr/local/bin/lip
USER lip
WORKDIR /app
ENV LIP_ADDR=:8080 \
    LIP_IMPORT_ROOT=/data/imports
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=20s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/api/v1/healthz >/dev/null || exit 1
ENTRYPOINT ["/usr/local/bin/lip"]
