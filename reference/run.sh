#!/usr/bin/env bash
# Calibrates the frozen development and control corpus inside the reference
# image. /src is the checkout, /corpus holds the four bare clones, and /out
# receives the reports, sidecars, selection, and evidence index.
set -euo pipefail

runner="${REFERENCE_RUNNER:-linux/amd64 container, 4 CPUs, 8 GiB, no network, Git 2.55.0, Go 1.27.0}"

CGO_ENABLED=0 go build -trimpath -o /tmp/tacita ./cmd/tacita
CGO_ENABLED=0 go build -trimpath -o /tmp/tacita-harness ./cmd/tacita-harness
git --version
sha256sum /tmp/tacita

exec /tmp/tacita-harness \
  --tacita /tmp/tacita \
  --runner "$runner" \
  --out /out \
  --golang-tools /corpus/tools.git \
  --kubernetes /corpus/kubernetes.git \
  --go-starter /corpus/go-starter.git \
  --proficiency /corpus/proficiency.git
