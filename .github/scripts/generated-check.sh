#!/usr/bin/env bash
# Checks that web/'s generated artifacts are up to date: the console's
# stylesheet (Tailwind) and the React app's build.
#
# A stale app.css once passed in silence: the 0.4.0 image shipped stamped
# `-dirty` because the committed CSS was out of date, and `make generate`
# dirtied the tree in the middle of publishing.
#
# Tailwind is PINNED by the variable below: `releases/latest` made two
# developers generate different CSS, and a gate that compares against a
# generator that changes on its own could never be reproducible.
set -euo pipefail

TAILWIND_VERSION="${TAILWIND_VERSION:-v4.3.3}"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"

echo "→ tailwind $TAILWIND_VERSION"

mkdir -p bin
if [ ! -x bin/tailwindcss ] || ! ./bin/tailwindcss --help 2>&1 | head -1 | grep -q "${TAILWIND_VERSION#v}"; then
  arch="$(uname -m | sed 's/x86_64/x64/;s/aarch64/arm64/')"
  # darwin -> macos: the release asset does not use the kernel name.
  os="$(uname -s | tr 'A-Z' 'a-z' | sed 's/darwin/macos/')"
  curl -sSLf -o bin/tailwindcss \
    "https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/tailwindcss-${os}-${arch}"
  chmod +x bin/tailwindcss
fi

./bin/tailwindcss -i web/assets/app.src.css -o web/assets/app.css --minify

# The React app: `npm ci` installs exactly the lockfile, so the build is the
# same on every machine and the diff below means something.
(cd web/spa && npm ci --silent && npm run build --silent)

if ! git diff --exit-code -- web/; then
  echo "::error::the generated artifacts are out of date: run 'make generate' and commit web/"
  exit 1
fi
echo "✅ generated artifacts are up to date (CSS and the React app)"
