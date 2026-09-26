# Catalog Performance

Catalog discovery is designed for progressive disclosure. Search should operate on compact metadata and return a bounded result set without requiring clients to enumerate or load every Tool schema.

## Scale fixture

The SQLite Catalog integration suite contains a deterministic 10,000-Tool fixture.

It verifies:

- an unrestricted search returns only the requested bounded result count,
- a unique lexical token resolves the expected Tool,
- profile-style allow-list search never returns a Tool outside membership,
- the same FTS5/BM25 path used by production is exercised.

Run the correctness test:

```bash
go test ./backend/internal/catalog/infrastructure/sqlite \
  -run TestCatalogSearchAtTenThousandTools \
  -count=1
```

## Benchmarks

Run:

```bash
pnpm go:bench:catalog
```

or directly:

```bash
go test ./backend/internal/catalog/infrastructure/sqlite \
  -run '^$' \
  -bench 'BenchmarkCatalog(Search|ScopedSearch)10K' \
  -benchmem
```

The benchmarks intentionally do not enforce a CI wall-clock threshold. Host CPU, filesystem, SQLite runtime, and Go version materially affect absolute time.

The useful signals are:

- trend in ns/op,
- bytes allocated per operation,
- allocations per operation,
- large regressions in scoped versus unrestricted discovery.

## Phase 8 baseline

Captured on 2026-09-26 on Apple M4 / darwin arm64 after scoped-search loading was hardened:

| Benchmark | Approx. time | Bytes/op | Allocs/op |
| --- | ---: | ---: | ---: |
| `BenchmarkCatalogSearch10K` | 24.4 ms | 85 KB | 1.8k |
| `BenchmarkCatalogScopedSearch10K` | 21.4 ms | 280 KB | 8.4k |

The scoped benchmark uses a 100-Tool allow-list distributed across a 10,000-Tool Catalog.

Before the Phase 8 adapter optimization, the same scoped workload was approximately 42 ms, 7.3 MB/op, and 167k allocations/op because disallowed FTS candidates were being fully rehydrated, including schemas, before membership filtering.

The optimized path first scans ranked Tool IDs, filters membership, then loads full canonical Tool records only for the bounded visible result set.

## Interpretation

These numbers are a development baseline, not an SLA.

A future performance gate should use controlled benchmark infrastructure and statistically evaluated historical comparisons rather than a fixed millisecond threshold in ordinary CI.
