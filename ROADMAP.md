# Roadmap

This is a living document. Status labels: **now**, **next**, **later**.

## now

- [x] Heap retention watch via pprof (`watch` / `run` / `diff`)
- [x] Slope / plateau / allowlist / load correlation
- [x] Public OSS packaging (license, CI, contributing docs)
- [ ] Expand integration coverage and CI signal quality

## next

- [ ] Goroutine-leak mode using the same sampling pipeline
- [ ] Better first-run UX (richer pprof connectivity errors, suggested flags)
- [ ] HTML / Markdown report output for CI artifacts
- [ ] Kubernetes recipe (port-forward / sidecar pattern) in docs
- [ ] Tunable profiles for short staging windows vs long soak tests

## later

- [ ] Continuous mode with rolling windows and alerting hooks
- [ ] Optional eBPF / runtime metrics backends (beyond HTTP pprof)
- [ ] Library API for embedding classification in custom tools
- [ ] Guided allowlist generation from known cache packages

## Contributing ideas

Open an issue with the `enhancement` template, or grab an issue labeled
`good first issue` once they appear. See [CONTRIBUTING.md](CONTRIBUTING.md).
