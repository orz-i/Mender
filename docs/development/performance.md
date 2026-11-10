# Catalog performance — canonical v3

Mender discovers tools through the canonical **Typed SourceRef Catalog**. Search uses FTS5/BM25 metadata indexes and bounded ToolID rehydration; it does not activate Providers, start Agents, open Sandboxes or inject every Tool schema into the caller's context.

## Current 10,000-Tool fixture

`backend/internal/catalog/infrastructure/sourcecatalog/search_v3_regression_test.go` uses the **same pinned v3 schema image** as the official Runtime (not an isolated Provider-v2 table). The fixture inserts 10,000 externally sourced Tool definitions in one independent Provider and preserves the nine immutable native Tool definitions. It verifies:

- bounded common-query search results;
- an exact unique-token hit in the large Catalog;
- Profile-style ToolID allowlists before returning results;
- typed Provider SourceRef filtering, including absent/other sources;
- safe handling of FTS-special syntax without exposing unselected tools.

Correctness test:

```bash
go test ./backend/internal/catalog/infrastructure/sourcecatalog \
  -run '^TestTypedCatalogFTSScopedSearchAtTenThousandTools$' -count=1
```

Benchmark (no fixed wall-clock threshold):

```bash
pnpm go:bench:catalog
```

This runs `BenchmarkTypedCatalogSearch10K` against the current v3 Catalog. Track `ns/op`, `B/op`, allocation counts, and scoped-query behavior under the same machine, Go and SQLite versions. Pre-v3 performance figures are not an acceptance baseline for the current SourceRef-aware query and should not be used to justify a regression or an incorrect schema fixture.
