# Contributing to godrain

Thanks for helping improve godrain. This project follows
[Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) and aims
to stay close to the [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md).

## Development setup

Requirements:

- Go 1.26+ (required by `github.com/google/pprof`; CI also exercises stable)
- `golangci-lint` (optional locally; required in CI)

```bash
git clone https://github.com/gonnafaraway/godrain.git
cd godrain
make test
make lint
make build
```

## Project layout

- `cmd/godrain` — CLI entrypoint
- `internal/*` — sampling, analysis, reporting
- `testdata/*` — demo leaky / cached services
- `configs/` — example allowlist

## Making changes

1. Create a focused branch from `main`
2. Add/adjust tests for behavior changes
3. Run:

```bash
make check
```

4. Open a PR with a Conventional Commit style title, e.g. `fix(analyze): ...`

### Commit messages

```text
<type>[optional scope]: <description>
```

Common types: `feat`, `fix`, `docs`, `refactor`, `test`, `ci`, `chore`.

## Demos

```bash
make demo-leaky   # unbounded map growth
make demo-cached  # bounded LRU plateau
```

Then in another terminal:

```bash
./godrain watch --url http://127.0.0.1:6061 --duration 45s --interval 5s --warmup 5s
./godrain watch --url http://127.0.0.1:6062 --duration 45s --interval 5s --warmup 5s
```

## Code of Conduct

By participating, you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).

## Security

See [SECURITY.md](SECURITY.md) for private vulnerability reporting. Do not file
public issues for security-sensitive reports.
