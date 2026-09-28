# Contributing to Muster

Thanks for contributing. Here's how.

## Setup

```bash
git clone https://github.com/wojons/muster.git
cd muster
make hooks           # one-time: enables the repo's pre-commit guard (.githooks/pre-commit)
make build-cli       # -> bin/openapi-cli
```

`make hooks` sets `core.hooksPath` to `.githooks` in your LOCAL git config. Until you run it,
commits print no guard banner (no secrets/build check) — that is expected, not a failure.

## Making changes

1. Fork the repo
2. Create a branch: `git checkout -b feature/your-thing`
3. Make your changes
4. Build: `make build-cli` (produces `bin/openapi-cli` — plain `go build ./...` in this multi-main layout compiles but writes no binaries)
5. Run the demo: `./demo/run-all.sh`
6. Commit: `git commit -m "feat: your thing"`
7. Push: `git push origin feature/your-thing`
8. Open a PR

## Code standards

- `go fmt ./...` before committing
- `go vet ./...` passes
- `go build ./...` passes
- `./demo/run-all.sh` passes (23/23)

## CI status

GitHub Actions is currently **billing-blocked** (external — needs org billing access), so
no automated checks run on pushes. Until it is restored, the **local pre-push gate is the
CI substitute** — run it before every push:

```bash
make validate        # unit + integration + security + lint + vet
./demo/run-all.sh    # real end-to-end gate (23+ checks)
```

## Documentation changes

- Markdown sources live in `docs/` (guides/, api/, architecture/, contributing/).
- After editing any `docs/**/*.md`, regenerate the HTML build so `docs/_build/`
  stays in sync with the sources:
  ```bash
  python3 docs/scripts/generate-docs.py --source docs --output docs/_build
  ```
  then commit the regenerated `docs/_build/` files in the same change.
- Sanity check before committing: `grep -r 'TODO\|doctor\|spec validate' docs/_build/`
  should return nothing (dead/aspirational commands leak into HTML builds).

## What to work on

Check `.coding-hermes/tasks.md` for pending work. Good first issues are tagged in GitHub.

## License

MIT. By contributing, you agree your code will be licensed under MIT.
