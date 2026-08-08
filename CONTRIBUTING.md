# Contributing to Muster

Thanks for contributing. Here's how.

## Setup

```bash
git clone https://github.com/wojons/muster.git
cd muster
go build ./...
```

## Making changes

1. Fork the repo
2. Create a branch: `git checkout -b feature/your-thing`
3. Make your changes
4. Build: `go build ./...`
5. Run the demo: `./demo/run-all.sh`
6. Commit: `git commit -m "feat: your thing"`
7. Push: `git push origin feature/your-thing`
8. Open a PR

## Code standards

- `go fmt ./...` before committing
- `go vet ./...` passes
- `go build ./...` passes
- `./demo/run-all.sh` passes (23/23)

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
