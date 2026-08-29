# syntax=docker/dockerfile:1.7
# cellp platform image: cellpd + celld + offshoot (+ esbuild for celld deploy)

ARG GO_VERSION=1.25
ARG RUST_VERSION=1.97.1
ARG CELLD_PROFILE=release

# --- celld (Rust submodule) ---
FROM rust:${RUST_VERSION}-bookworm AS build-celld
ARG TARGETARCH
ARG CELLD_PROFILE
WORKDIR /src/celld
COPY celld/Cargo.toml celld/Cargo.lock ./
COPY celld/crates ./crates
RUN --mount=type=cache,id=cellp-celld-cargo-registry,target=/usr/local/cargo/registry,sharing=locked \
    --mount=type=cache,id=cellp-celld-cargo-git,target=/usr/local/cargo/git,sharing=locked \
    --mount=type=cache,id=cellp-celld-target-${TARGETARCH},target=/src/celld/target,sharing=locked \
    mkdir -p /out && \
    cargo build --profile "${CELLD_PROFILE}" --locked -p celld && \
    install -m 755 "target/${CELLD_PROFILE}/celld" /out/celld

# --- offshoot CLI ---
FROM golang:${GO_VERSION}-bookworm AS build-offshoot
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    CGO_ENABLED=0 GOBIN=/out go install github.com/sricola/offshoot/cmd/offshoot@latest

# --- cellpd ---
FROM golang:${GO_VERSION}-bookworm AS build-cellpd
WORKDIR /src/cellp
COPY cellp/go.mod cellp/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod download
COPY cellp/ ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/cellpd ./cmd/cellpd

# --- esbuild (celld deploy bundling) ---
FROM node:22-bookworm-slim AS build-esbuild
RUN npm install -g esbuild

# --- runtime ---
FROM debian:bookworm-slim AS runtime
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates curl sqlite3 && \
    rm -rf /var/lib/apt/lists/*

COPY --from=build-celld /out/celld /usr/local/bin/celld
COPY --from=build-cellpd /out/cellpd /usr/local/bin/cellpd
COPY --from=build-offshoot /out/offshoot /usr/local/bin/offshoot
COPY --from=build-esbuild /usr/local/bin/esbuild /usr/local/bin/esbuild

WORKDIR /data
EXPOSE 8787 8790

HEALTHCHECK --interval=10s --timeout=5s --start-period=20s --retries=6 \
    CMD curl -fsS http://127.0.0.1:8790/v1/health && curl -fsS http://127.0.0.1:8787/health

ENTRYPOINT ["cellpd"]
