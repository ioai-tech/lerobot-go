# Contributing

Thanks for helping improve lerobot-go.

## Development setup

```bash
git clone https://github.com/ioai-tech/lerobot-go.git
cd lerobot-go
go test ./... -count=1
```

Requires Go 1.26+ (see `go.mod`).

## Before opening a PR

1. Run tests: `make test`
2. Run linter: `make lint` — install the [official binary](https://golangci-lint.run/welcome/install/) **v2.9.0+** (must be built with Go ≥ `go.mod`; do not use `go install`)
3. Keep changes focused; match existing naming and package layout
4. Public API changes belong in `lerobot/`; CLI wiring in `internal/cli/`

## Commit messages

Use clear, imperative subjects. Examples:

- `fix merge stats aggregation for v3 episode meta`
- `docs: add create command to CLI reference`

## End-to-end tests

Heavy dataset tests are optional locally:

```bash
make e2e
```

CI runs unit tests only.

## Official LeRobot compatibility smoke checks

For higher-confidence stability validation against the official Python loader:

1. Install the official Python runtime into an isolated prefix (one workable approach on this VM was `pip --prefix`, since `uv` was unavailable).
2. Download authoritative sample datasets such as:
   - `lerobot/pusht`
   - `lerobot/pusht_image`
   - optionally `nvidia/LIBERO_LeRobot_v3` (for example `libero_10/**`)
3. Run:

```bash
make official-stability
```

The script `scripts/run_official_stability_checks.sh`:
- loads official datasets with the official `lerobot` package,
- exercises Go convert v3→v2.1→v3 round-trips,
- exercises Go merge on official sample datasets,
- re-loads the resulting v3 outputs with the official `lerobot` package.

Useful environment overrides:

```bash
export LEROBOT_OFFICIAL_PY_PREFIX=/path/to/prefix/local
export LEROBOT_OFFICIAL_DATA_ROOT=/path/to/downloaded/datasets
export LEROBOT_OFFICIAL_WORK_ROOT=/tmp/lerobot-go-official-checks
```

## Questions

Open a [GitHub issue](https://github.com/ioai-tech/lerobot-go/issues) for bugs or feature requests.
