# Universidad App

PWA universitaria full-stack: backend Go (binario único) + frontend Svelte + PostgreSQL.
Documentación completa de setup y despliegue: ver `deploy/` y ADRs (pendientes, fase final).

## Desarrollo rápido (laptop Fedora)

    cp .env.example .env          # y editar JWT_SECRET
    docker compose up -d db       # PostgreSQL 16 local
    cd backend && go run ./cmd/server

## Convenciones

- Migraciones versionadas en `backend/internal/migrations` (golang-migrate, up/down).
- Tests por módulo; ningún código se entrega sin su test en verde.
- Ramas: `feature/<modulo>-<descripcion>`. Commits en español, imperativo.
