# Engineering Rules

- Act like a senior Go backend engineer operating at a Google-grade review bar: correctness, maintainability, and clear ownership matter more than cleverness.
- No 1,000-line source files. If a change would push a non-generated file near or past 1,000 lines, split it by responsibility before committing.
- Treat 700-800 lines as a warning zone for code files. Look for natural package, type, handler, test-helper, or adapter boundaries instead of letting the file sprawl.
- Generated, vendored, fixture, or machine-owned files may exceed the limit, but handwritten application code should not.
- Keep interfaces small and owned by the consumer. Add an abstraction only when it removes real coupling or protects a concrete boundary.
- Prefer focused packages and boring control flow. Do not add generic utility layers, framework wrappers, or broad configuration surfaces unless the current behavior needs them.
- Keep public APIs compact. Every field, flag, endpoint, and response property must fight for existence.
- Preserve markdown as jazmem's source of truth. SQLite is an index/cache and must not contain canonical facts that cannot be rebuilt.
- SQLite schema changes live in Goose migrations under `internal/store/sqlite/migrations`. Do not put DDL back into Go strings.
- SQL queries that are static and stable should use SQLC. Keep query groups separated under `internal/store/sqlite/queries/<concern>` and generate to `internal/store/sqlite/generated/<concern>db` with `emit_interface`.
- Do not create one generated "everything database" package. Each feature should depend only on the generated interface for the concern it actually needs.
- Generated files under `internal/store/sqlite/generated` are machine-owned. Edit the SQL query files or migrations, then run `go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate`.
- Handwritten dynamic SQL is acceptable when SQLC would require awkward query construction, especially graph expansion with dynamic seed sets or link-type predicates.
- Agent-facing response payloads (search results, citations, MCP outputs) must be token-efficient: no field that duplicates information already present (e.g. a `type` field when the slug prefix encodes the lane), and no field that does not change agent behavior. Every response field must earn its tokens.
- Keep the reading path generous where it matters: chunks target the research-backed 100-400 token range (pack limit 1400 chars in the indexer) and search snippets must carry enough of the chunk to act on (600 chars), because truncated snippets, not chunk size, are the usual cause of a weak reading side.
- Run `gofmt` and `go test ./...` after code changes that touch Go behavior.
