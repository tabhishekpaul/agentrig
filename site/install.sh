#!/bin/sh
# Agentrig installer for macOS and Linux:
#   curl -fsSL https://tabhishekpaul.github.io/agentrig/install.sh | sh
# Installs arig to ~/.arig/bin (override with ARIG_INSTALL=/some/dir), adds it
# to PATH and starts the always-on dashboard. Set ARIG_NO_START=1 to skip that.
set -eu

REPO="tabhishekpaul/agentrig"
DIR="${ARIG_INSTALL:-$HOME/.arig}/bin"

red() { printf '\033[31m%s\033[0m\n' "$1" >&2; }
fail() { red "error: $1"; exit 1; }

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) fail "unsupported OS $(uname -s). On Windows run: irm https://tabhishekpaul.github.io/agentrig/install.ps1 | iex" ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) fail "unsupported CPU $(uname -m)" ;;
esac
# A shell running under Rosetta reports x86_64 on Apple silicon: install the native build.
if [ "$os" = darwin ] && [ "$arch" = amd64 ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" = 1 ]; then
  arch=arm64
fi

command -v curl >/dev/null || fail "curl is required"
command -v tar >/dev/null || fail "tar is required"

asset="arig_${os}_${arch}.tar.gz"
base="${ARIG_DOWNLOAD:-https://github.com/$REPO/releases/latest/download}"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

printf 'Downloading arig for %s/%s…\n' "$os" "$arch"
curl -fsSL "$base/$asset" -o "$tmp/$asset" || fail "download failed: $base/$asset"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt" || fail "download failed: checksums.txt"

# Never install something that doesn't match the published checksum.
want=$(awk -v f="$asset" '$2 == f || $2 == "*"f { print $1 }' "$tmp/checksums.txt")
[ -n "$want" ] || fail "$asset is not listed in checksums.txt"
if command -v sha256sum >/dev/null; then
  got=$(sha256sum "$tmp/$asset" | awk '{print $1}')
else
  got=$(shasum -a 256 "$tmp/$asset" | awk '{print $1}')
fi
[ "$want" = "$got" ] || fail "checksum mismatch for $asset (expected $want, got $got)"

tar -xzf "$tmp/$asset" -C "$tmp"
mkdir -p "$DIR"
mv -f "$tmp/arig_${os}_${arch}/arig" "$DIR/arig"
chmod +x "$DIR/arig"

# Put ~/.arig/bin on PATH for future shells (once).
case ":$PATH:" in
  *":$DIR:"*) ;;
  *)
    line="export PATH=\"$DIR:\$PATH\""
    case "$(basename "${SHELL:-sh}")" in
      zsh) rc="$HOME/.zshrc" ;;
      bash) if [ "$os" = darwin ]; then rc="$HOME/.bash_profile"; else rc="$HOME/.bashrc"; fi ;;
      fish)
        rc="$HOME/.config/fish/conf.d/arig.fish"
        line="fish_add_path \"$DIR\""
        mkdir -p "$(dirname "$rc")"
        ;;
      *) rc="$HOME/.profile" ;;
    esac
    if ! grep -qsF "$DIR" "$rc"; then
      printf '\n# Agentrig\n%s\n' "$line" >> "$rc"
      printf 'Added %s to PATH in %s\n' "$DIR" "$rc"
    fi
    ;;
esac

printf '\n\033[1marig %s installed\033[0m to %s/arig\n' "$("$DIR/arig" version | awk '{print $2}')" "$DIR"

if [ "${ARIG_NO_START:-}" = 1 ]; then
  printf 'Run arig to start the dashboard.\n'
else
  # Starts the background server (and at every login from now on), then opens the dashboard.
  "$DIR/arig" || red "arig installed, but the dashboard didn't start; run: $DIR/arig"
fi
printf 'Dashboard: http://arig.localhost:7377 (open a new terminal to use the arig command)\n'
