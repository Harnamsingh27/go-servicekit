# Contributing to go-servicekit

This project follows an inner-source model: anyone within the organisation can contribute. The guidelines below keep the review process fast and the codebase consistent.

---

## Workflow

1. **Create a branch** from `main`.
   ```bash
   git checkout -b feat/my-feature
   ```

2. **Write or update tests first.** Every PR must maintain or improve coverage. Target ≥ 80% statement coverage per package.

3. **Run the full check suite locally** before opening a PR:
   ```bash
   make vet lint test-race
   ```

4. **Open a pull request** against `main`. The PR description should answer:
   - What problem does this solve?
   - Why is this the right approach?
   - What did you test and how?

5. **One approval** from a codeowner is required to merge.

---

## Commit messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <short summary>

[optional body]
```

Types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`, `ci`.

Examples:
```
feat(httpx): add PATCH route helper
fix(config): handle empty env var correctly
docs: add grpcx usage example to README
```

---

## Code style

- `gofmt` and `goimports` are enforced by CI. Run `make lint` before pushing.
- Public APIs must have a doc comment. The linter enforces this via `revive`.
- No `panic` in library code — return errors instead. Use `PanicRecovery` middleware/interceptors at the boundary.
- Keep exported types free of stuttering (`auth.Verifier`, not `auth.AuthVerifier`).

---

## Adding a new package

1. Create `yourpkg/doc.go` with a package-level comment describing the contract.
2. Export only what callers need. Start with the minimal surface area.
3. Add tests in `yourpkg/yourpkg_test.go` (external test package `yourpkg_test`).
4. Add a row to the package table in `README.md`.
5. If there is a non-obvious design decision, add a design doc under `docs/design/`.

---

## Design docs

Place Architecture Decision Records (ADRs) in `docs/design/` following the naming convention `NNNN-short-title.md`. Use the template in `docs/design/0001-config-loading.md` as a reference.

---

## Reporting issues

Open a GitHub issue with a minimal reproduction and the output of `go env`.