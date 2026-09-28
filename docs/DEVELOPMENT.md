# Development Notes

This repository is developed directly in GitHub first to avoid cluttering the operator's local machine.

## First runnable milestone

The first implementation milestone is a foundation stack:

- Go API service
- Go worker service
- MySQL 8
- Redis
- health and readiness endpoints
- initial schema migration
- Docker Compose stack
- `supportctl` CLI scaffold

## Local validation later

When a local or server checkout is available:

```bash
cp .env.example .env
./supportctl up
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
```

## Next implementation slices

1. Real migration runner instead of relying only on MySQL init scripts.
2. `supportctl init-agent`.
3. `supportctl site create/list/code`.
4. Customer XBoard login endpoint.
5. Customer email verification endpoint.
6. Conversation/message/event core.
7. Agent login and `/sync`.
