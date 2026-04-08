# Progress

## Phase Checklist
- [x] Phase 0: Baseline inspection completed
- [x] Phase 1: Planning docs and initial directories created
- [x] Phase 2: Minimal runnable MVP code skeleton implemented
- [x] Phase 3: README completed
- [x] Phase 4: Verification and file inventory completed
- [x] Phase 5: Route deletion, validation hook, and deployment examples completed
- [x] Phase 6: Status visibility, graceful shutdown, and install tooling completed
- [x] Phase 7: GitHub bootstrap install flow completed
- [x] Phase 8: Embedded panel, explicit update endpoint, and uninstall tooling completed
- [x] Phase 9: Port-listener routes, host upstreams, Cloudflare TLS path, and SSH menu completed

## Activity Log
- 2026-04-08: Checked repository baseline. Existing files were `AGENTS.md`, `README.md`, and `LICENSE`.
- 2026-04-08: Confirmed `codex-action/00-master-plan.md` and `codex-action/progress.md` were missing and created them.
- 2026-04-08: Replaced the initial Caddy plan with an Nginx-first MVP using hook-based HTTPS automation.
- 2026-04-08: Added the minimal Go service skeleton with JSON state, route upsert API, Nginx config rendering, and optional certificate/reload hooks.
- 2026-04-08: Completed `README.md` with project positioning, architecture, startup flow, API examples, and MVP boundaries.
- 2026-04-08: Verified the repository with `go test ./...` and captured the current file inventory for handoff.
- 2026-04-09: Added `DELETE /routes/{domain}`, optional validation-before-reload, and deployment examples for `systemd` plus environment files.
- 2026-04-09: Re-verified the repository with `go test ./...` after the operational hardening changes.
- 2026-04-09: Added `/status`, route-level TLS visibility, graceful shutdown, a basic `Makefile`, and an install script for faster deployment.
- 2026-04-09: Verified `go build` with a writable `GOCACHE` and tested `deployments/install.sh` successfully against `/tmp`.
- 2026-04-09: Added `deployments/bootstrap.sh` so the project can be installed directly from a raw GitHub script URL.
- 2026-04-09: Verified `deployments/bootstrap.sh` locally with a writable `/tmp` target and updated build tooling to use writable Go cache paths by default.
- 2026-04-09: Added the embedded `/panel/` UI, `PUT /routes/{domain}`, and `deployments/uninstall.sh` for full install-to-remove lifecycle coverage.
- 2026-04-09: Re-verified with `go test ./...`, `make build`, local bootstrap install to `/tmp`, and local uninstall with preserved state backup.
- 2026-04-09: Refactored the route model to support domain listeners, port listeners, IP:port upstreams, and host-based HTTP/HTTPS upstreams.
- 2026-04-09: Added built-in Cloudflare DNS certificate support and an SSH menu script for create, update, and delete flows.
- 2026-04-09: Converted the browser panel into a read-only observability surface for routes and runtime status.
- 2026-04-09: Finalized the name-based route API/docs, fixed the bootstrap panel path hint for custom install directories, and re-verified with `go test ./...`, `make build`, local bootstrap install, SSH menu launch, and uninstall.
- 2026-04-09: Improved the SSH menu to print API error bodies directly and added a regression test for domain routes that proxy to an HTTP host upstream.
- 2026-04-09: Changed startup behavior so initial sync failures keep the API online in degraded mode, and clarified the Cloudflare email/plugin configuration in docs and env examples.
