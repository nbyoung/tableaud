#!/bin/sh
# Fetch the pinned HTMX release into internal/web/static/htmx.
#
# Usage: scripts/vendor-htmx.sh
# Edit HTMX_VERSION to refresh, run the script, and commit the result.
set -eu

HTMX_VERSION=2.0.6

dir="$(dirname "$0")/../internal/web/static/htmx"
base="https://raw.githubusercontent.com/bigskysoftware/htmx/v${HTMX_VERSION}"

curl -sSfL "${base}/dist/htmx.min.js" -o "${dir}/htmx.min.js"
curl -sSfL "${base}/LICENSE" -o "${dir}/LICENSE"

echo "htmx ${HTMX_VERSION}"
sha256sum "${dir}/htmx.min.js"
