# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Go library that extracts FAA NASR (National Airspace System Resources) 28-day subscription CSV data into a SQLite database. Single public API: `Extract(nasrSubscription, sqliteDatabase string) error`. Targets CSV format only (legacy `.txt` format is being sunsetted Dec 2026).

## Build & Test

```bash
go build ./...                          # build
go test -v -count=1 ./...               # run all tests (~2s with real data)
go test -run TestConvertValue ./...     # run a single test
go test -coverprofile=c.out ./...       # coverage (82%)
```

Integration tests require `28DaySubscription_Effective_*.zip` in the project root (252MB, not committed). Tests skip gracefully when the file is absent. `TestMain` runs `Extract` once and shares the resulting DB across all integration tests.

## Architecture

**Schema-driven**: Table schemas are parsed at runtime from `*_CSV_DATA_STRUCTURE.csv` files inside the zip, not hardcoded. This makes the code resilient to FAA format changes across the 63 tables.

### Pipeline (`Extract` in nasr.go)

```
outer zip (memory-mapped) → inner CSV zip (in-memory) → parse schemas → CREATE TABLE → load CSVs → FK check
```

### File Responsibilities

- **zip.go** — `openInnerCSVZip`: Opens outer subscription zip, finds `CSV_Data/*_CSV.zip` (excluding delta zips with hyphens), reads into memory, returns `*zip.Reader` + raw `[]byte` (backing array must stay alive for reader reuse)
- **schema.go** — `parseSchemas`: Reads DATA_STRUCTURE CSVs, groups by table name, normalizes columns (spaces→underscores, VARCHAR→TEXT, NUMBER→REAL). `generateDDL`: Produces CREATE TABLE statements with FK clauses. `normalizeCR`: Fixes bare CR line endings (FSS edge case)
- **loader.go** — `loadAllCSVs`/`loadCSV`: One transaction per table, prepared INSERT, BOM stripping. `convertValue`: empty+nullable→nil, REAL→float64, else string
- **foreignkeys.go** — `foreignKeyDefs`: 38 intra-group FK relationships (cross-group FKs omitted intentionally)
- **nasr.go** — `Extract`: Orchestrates the pipeline. Creates a fresh `zip.Reader` from the raw bytes after schema parsing (the first reader is consumed)

### Key Design Decisions

- **Two zip.Reader passes**: `parseSchemas` consumes the reader iterating over DATA_STRUCTURE files. `loadAllCSVs` needs a fresh reader from the same backing `[]byte`.
- **FK enforcement OFF**: FKs are in the schema for documentation. `PRAGMA foreign_key_check` runs post-load and logs warnings (WXL_SVC→WXL_BASE has known data mismatches).
- **SQLite driver**: `modernc.org/sqlite` (pure Go, no CGo). Driver name is `"sqlite"`, not `"sqlite3"`.
- **Bare CR handling**: `FSS_CSV_DATA_STRUCTURE.csv` uses `\r`-only line endings. `normalizeCR()` replaces bare `\r` with `\n` before csv.Reader parsing.
- **BOM handling**: Some NASR CSVs (e.g., PJA_BASE.csv) start with UTF-8 BOM (`\xef\xbb\xbf`), stripped in `loadCSV`.

## NASR Subscription Zip Structure

```
28DaySubscription_Effective_YYYY-MM-DD.zip   (252 MB)
  └── CSV_Data/
      └── DD_Mon_YYYY_CSV.zip              (~22 MB, the one without a hyphen)
          ├── 24 *_CSV_DATA_STRUCTURE.csv   (schema definitions)
          └── 63 *.csv                      (data files)
```
