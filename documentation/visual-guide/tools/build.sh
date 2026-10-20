#!/usr/bin/env bash
# Bundle src/main.js and Three.js into dist/world.js, one classic script that
# also works from file://. Needs rolldown and three@0.186.0: run `npm install`
# in this folder, or point ROLLDOWN and THREE_DIR at existing installs.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
rolldown="${ROLLDOWN:-$here/node_modules/.bin/rolldown}"
"$rolldown" -c "$here/rolldown.config.mjs"
ls -l "$here/../dist/world.js"
