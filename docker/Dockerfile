# syntax=docker/dockerfile:1.7
# cellp platform image: cellpd + celld + offshoot (+ esbuild for celld deploy)
#
# CI (docker-publish.yml): celld is built natively on amd64/arm64 runners and
# copied into docker/prebuilt/<arch>/celld before build — avoids QEMU Rust compiles
# (~10-15 min saved on arm64). Local `docker build` compiles celld in-image when
# prebuilt binaries are absent.

ARG GO_VERSION=1.25
ARG RUST_VERSION=1.97.1
ARG CELLD_PROFILE=release
ARG ESBUILD_VERSION=0.28.2

# --- celld (Rust submodule; skipped when docker/prebuilt/<arch>/celld exists) ---
FROM rust:${RUST_VERSION}-bookworm AS build-celld
ARG TARGETARCH
ARG CELLD_PROFILE
WORKDIR /src/celld
COPY docker/prebuilt/ /prebuilt/
COPY celld/Cargo.toml celld/Cargo.lock ./
COPY celld/crates ./crates
RUN --mount=type=cache,id=cellp-celld-cargo-registry,target=/usr/local/cargo/registry,sharing=locked \
    --mount=type=cache,id=cellp-celld-cargo-git,target=/usr/local/cargo/git,sharing=locked \
    --mount=type=cache,id=cellp-celld-target-${TARGETARCH},target=/src/celld/target,sharing=locked \
    mkdir -p /out && \
    if [ -f "/prebuilt/${TARGETARCH}/celld" ]; then \
      install -m 755 "/prebuilt/${TARGETARCH}/celld" /out/celld; \
    else \
      cargo build --profile "${CELLD_PROFILE}" --locked -p celld && \
      install -m 755 "target/${CELLD_PROFILE}/celld" /out/celld; \
    fi

# --- offshoot CLI ---
FROM golang:${GO_VERSION}-bookworm AS build-offshoot
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    CGO_ENABLED=0 GOBIN=/out go install github.com/sricola/offshoot/cmd/offshoot@v0.2.0

# --- cellpd ---
FROM golang:${GO_VERSION}-bookworm AS build-cellpd
WORKDIR /src/cellp
COPY cellp/go.mod cellp/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod download
COPY cellp/ ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/cellpd ./cmd/cellpd && \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/cellp ./cmd/cellp

# --- esbuild (celld deploy bundling; prebuilt npm tarball, no node install) ---
FROM debian:bookworm-slim AS build-esbuild
ARG TARGETARCH
ARG ESBUILD_VERSION
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates curl && \
    case "${TARGETARCH}" in \
      amd64) pkg=linux-x64 ;; \
      arm64) pkg=linux-arm64 ;; \
      *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; \
    esac && \
    curl -fsSL "https://registry.npmjs.org/@esbuild/${pkg}/-/${pkg}-${ESBUILD_VERSION}.tgz" \
      | tar -xz -C /tmp && \
    mkdir -p /out && \
    install -m 755 /tmp/package/bin/esbuild /out/esbuild && \
    rm -rf /var/lib/apt/lists/* /tmp/package

# --- runtime ---
FROM debian:bookworm-slim AS runtime
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates curl sqlite3 && \
    rm -rf /var/lib/apt/lists/*

COPY --from=build-celld /out/celld /usr/local/bin/celld
COPY --from=build-cellpd /out/cellpd /usr/local/bin/cellpd
COPY --from=build-cellpd /out/cellp /usr/local/bin/cellp
COPY --from=build-offshoot /out/offshoot /usr/local/bin/offshoot
COPY --from=build-esbuild /out/esbuild /usr/local/bin/esbuild

WORKDIR /data
EXPOSE 8787 8790

HEALTHCHECK --interval=10s --timeout=5s --start-period=20s --retries=6 \
    CMD curl -fsS http://127.0.0.1:8790/v1/health && curl -fsS http://127.0.0.1:8787/health

ENTRYPOINT ["cellpd"]
