# ADR 0001: Multi-source configuration loading

**Status:** Accepted  
**Date:** 2026-06-24

---

## Context

Services need configuration from several sources simultaneously:

- Static defaults embedded in the struct
- A checked-in YAML file for non-secret values
- A `.env` file for local development secrets
- Real environment variables in CI and production
- In-process overrides for testing

Without a clear precedence model, callers are forced to write their own merging logic, leading to subtle bugs where, for example, a test override is silently ignored because a system env var takes precedence.

---

## Decision

`config.Load[T]` applies sources in strict ascending precedence order:

```
struct defaults < YAML file < .env file < environment variables < explicit overrides
```

The implementation uses two internal helpers to enforce this:

- **`applyEnvVars`** — applies `.env` file values first, then re-checks `os.Getenv` to let real env vars win over `.env`. This handles the `.env file < env vars` boundary.
- **`applyMapOnly`** — applies the explicit override map *without* any `os.Getenv` fallback. This ensures overrides always win, regardless of what system env vars are set. Using `applyEnvVars` for overrides would have been incorrect: `os.Getenv` would re-overwrite an override with the system env var.

Schema is driven entirely by struct tags (`yaml:`, `env:`, `default:`, `validate:"required"`). There is no separate schema file and no reflection on JSON tags; only the tags listed above are consumed.

---

## Consequences

**Positive**
- Predictable, documented precedence — callers can reason about which source wins.
- Testing is straightforward: pass `WithOverrides(map[string]string{...})` and env vars are irrelevant.
- Generic (`Load[T any]`) — zero code generation required.

**Negative / Trade-offs**
- Reflection-based field traversal. Deeply nested pointer structs are not supported (only value-type nested structs).
- `validate:"required"` only checks zero-value; more complex validation must live in `Validator.Validate()`.
- No watch/hot-reload. Callers that need dynamic config must re-call `Load` or bring their own watcher.

---

## Alternatives considered

| Alternative | Reason rejected |
|---|---|
| `viper` | Large dependency surface; ties callers to Viper's global state model |
| Code generation (e.g. `envconfig`) | Requires build tooling; struct tags are sufficient for the current requirements |
| Single env-var source | Teams already ship YAML config files; removing that source would require migration |