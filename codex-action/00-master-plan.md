# Edge Proxy Manager Master Plan

## Goal
Build an MVP edge proxy manager that lets an operator provide:
- domain
- target IP
- target port

The system should then:
- persist the route definition
- generate reverse proxy configuration automatically
- support HTTPS automation through the proxy layer

## Product Positioning
`reproxy` is not the reverse proxy itself. It is the control layer that manages route definitions and renders proxy configuration for a dedicated edge proxy on a well-connected ingress machine.

## MVP Scope
- Provide a small HTTP API for route management.
- Persist route definitions in a local JSON file.
- Render an Nginx site config from the stored routes.
- Support optional automatic certificate issuance by executing a configured certificate command template.
- Support optional automatic reload by executing a configured reload command after config sync.
- Expose a health endpoint for operational checks.

## Explicit Non-Goals For MVP
- Multi-user auth and RBAC
- Database-backed state
- Full TLS certificate lifecycle management inside the manager
- Advanced routing features such as path rewrite, header manipulation, canary, and load balancing
- Multi-node orchestration

## Architecture Decision
### Decision 1: Use Go for the manager
Reason:
- single static binary is lightweight to deploy
- good standard library for HTTP, file IO, and validation
- easy to maintain without framework overhead

### Decision 2: Use Nginx as the proxy runtime target
Reason:
- Nginx is a common and predictable edge proxy choice
- generated server blocks are easy to inspect and operate
- keeps the manager focused on config generation while delegating certificate issuance to external tooling

### Decision 3: Use file-based JSON state in MVP
Reason:
- lowest operational overhead
- transparent and easy to inspect
- adequate until route volume and concurrency become more demanding

### Decision 4: Generated Nginx config is a derived artifact
Reason:
- JSON route state is the primary source of truth
- proxy config can be re-rendered on startup or after writes
- keeps recovery logic simple

### Decision 5: HTTPS automation is hook-based in MVP
Reason:
- embedded ACME logic would make the manager heavier
- external tools such as `certbot` or `acme.sh` can be adopted per environment
- the manager only needs to trigger certificate issuance and detect whether certificate files exist

### Decision 6: Validate before reload when an explicit validation command is configured
Reason:
- Nginx syntax errors should be caught before reload
- validation must remain optional so local development can stay lightweight
- rollback of the generated config keeps the running proxy safer when validation fails

### Decision 7: Expose sync and TLS readiness through the API
Reason:
- operators need to know whether generated routes are already HTTPS-ready
- sync failures must be queryable without reading process logs
- the manager should provide enough state for light automation and monitoring

### Decision 8: Provide a GitHub bootstrap script for one-command installation
Reason:
- deployment should not require the operator to manually clone the repository first
- the repo should remain installable through a raw GitHub script URL
- bootstrap should install dependencies, fetch source, run the installer, and optionally start the service

## Runtime Flow
1. Operator starts `reproxy`.
2. Service loads local route state from disk.
3. Service checks whether TLS material already exists for each domain.
4. Service renders `deployments/nginx/reproxy.conf`.
5. Optional reload command is executed if configured.
6. Operator creates or updates a route through the HTTP API.
7. Service validates the input, persists JSON state, optionally runs a certificate command, regenerates Nginx config, and optionally reloads Nginx.

## Planned Project Structure
- `cmd/reproxy`: application entrypoint
- `internal/runtime`: environment-based runtime configuration
- `internal/app`: route validation and orchestration logic
- `internal/store`: file-backed route persistence
- `internal/nginx`: Nginx config renderer and TLS hook syncer
- `internal/httpapi`: HTTP handlers and server wiring
- `deployments/nginx`: generated Nginx config target
- `data`: local route state for MVP
- `codex-action`: plan and progress tracking

## MVP API Shape
- `GET /healthz`
- `GET /routes`
- `POST /routes`

`POST /routes` uses domain as the stable key and behaves as upsert for MVP simplicity and retry safety.

## Risks And Mitigations
- Risk: config reload fails after state is persisted.
  Mitigation: keep JSON state as source of truth and re-sync config on next startup or next write.
- Risk: Nginx or the ACME tool is not installed on target machine.
  Mitigation: make reload optional and document a manual or external provisioning path.
- Risk: TLS material is not ready when a route is first created.
  Mitigation: render HTTP service immediately, expose ACME challenge path, and add TLS blocks once certificate files exist.
- Risk: route collisions or invalid inputs.
  Mitigation: validate domain, IP, and port before persisting.

## Phase Plan
### Phase 1
- Create repo conventions and planning docs.

### Phase 2
- Scaffold minimal Go service and Nginx renderer.

### Phase 3
- Write README covering positioning, architecture, startup, and MVP constraints.

### Phase 4
- Verify build and record next steps.

### Phase 5
- Add route deletion, pre-reload validation, and deployment examples.

### Phase 6
- Add route status visibility, service status endpoint, graceful shutdown, and installation tooling.

### Phase 7
- Add GitHub bootstrap installation flow and publish the one-command installation path in the README.
