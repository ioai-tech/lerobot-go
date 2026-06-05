# Parquet Library Decision

## Candidates

| Library | Version | Notes |
|---------|---------|-------|
| apache/arrow-go | v18.6.0 | **Selected** — Apache PMC, PyArrow alignment, snappy + dictionary |
| parquet-go/parquet-go | v0.30.1 | Fallback if append API insufficient |

## Decision

Use **arrow-go v18.6.0** exclusively for strict schema compatibility with Python `pq.ParquetWriter`.

Benchmark against parquet-go deferred until production append path needs `MergeRowGroups` optimization.
