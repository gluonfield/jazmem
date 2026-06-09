# Engineering Rules

- Act like a senior Go backend engineer operating at a Google-grade review bar: correctness, maintainability, and clear ownership matter more than cleverness.
- No 1,000-line source files. If a change would push a non-generated file near or past 1,000 lines, split it by responsibility before committing.
- Treat 700-800 lines as a warning zone for code files. Look for natural package, type, handler, test-helper, or adapter boundaries instead of letting the file sprawl.
- Generated, vendored, fixture, or machine-owned files may exceed the limit, but handwritten application code should not.
- Keep interfaces small and owned by the consumer. Add an abstraction only when it removes real coupling or protects a concrete boundary.
- Prefer focused packages and boring control flow. Do not add generic utility layers, framework wrappers, or broad configuration surfaces unless the current behavior needs them.
- Keep public APIs compact. Every field, flag, endpoint, and response property must fight for existence.
- Preserve markdown as jazmem's source of truth. SQLite is an index/cache and must not contain canonical facts that cannot be rebuilt.
- Run `gofmt` and `go test ./...` after code changes that touch Go behavior.
