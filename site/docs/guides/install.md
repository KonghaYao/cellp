# Install

One binary CLI (`cellp`) plus `celld`, `offshoot`, and `esbuild`. No Docker required for local `cellp dev`.

## One-liner (macOS / Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/KonghaYao/cellp/main/scripts/install.sh | sh
```

Puts `cellp`, `cellpd`, `celld`, `offshoot`, and `esbuild` in `~/.local/bin`. Then:

```bash
export PATH="$HOME/.local/bin:$PATH"
cellp doctor
cellp dev
```

Override install location: `CELLP_INSTALL_DIR=/usr/local/bin curl -fsSL … | sh`

Pin a release: `CELLP_VERSION=v0.1.0 curl -fsSL … | sh`

If the GitHub API rate-limits you, set `GH_TOKEN` (a fine-grained or classic token with `contents: read`).

## What you get

| Binary | Role |
|--------|------|
| **cellp** | CLI: `dev` (local platform), `serve` (env-based cellpd), `doctor` |
| **celld** | Workers runtime (spawned per version) |
| **offshoot** | SQLite copy-on-write for App + Data |
| **cellpd** | Same process as `cellp serve` (Compose / systemd) |
| **esbuild** | Bundler used by `celld deploy` (included in the release tarball) |

## GitHub Releases

Cross-platform archives are published on version tags (`v*`):

https://github.com/KonghaYao/cellp/releases

Names: `cellp_<tag>_<os>_<arch>.tar.gz` (Windows `.zip`).

## Docker

Production-shaped stack (RustFS + cellpd image): [Self-hosting](/guides/self-hosting) · `ghcr.io/konghayo/cellp`.

## From source (contributors)

```bash
git clone https://github.com/KonghaYao/cellp.git && cd cellp
git submodule update --init celld
cd cellp && go build -o cellp ./cmd/cellp
cd ../celld && cargo build -p celld --profile lab
```

The old Docker-based laptop stack is still `./dev/scripts/up.sh`.
