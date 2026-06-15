# Contributing to Symaira Fetch

Thank you for your interest in contributing to Symaira Fetch!

## Development Setup

1. **Prerequisites**: Go 1.26.4 or later.
2. **Clone the repo**:
   ```bash
   git clone https://github.com/danieljustus/symaira-fetch.git
   cd symaira-fetch
   ```
3. **Build**:
   ```bash
   make build
   ```
4. **Run Tests**:
   ```bash
   make test
   ```

## Pull Request Process

1. Fork the repository and create your branch from `main`.
2. Ensure all tests and linters pass before submitting:
   - `go vet ./...`
   - `go test -race ./...`
3. Document any new features or CLI options in `README.md` if applicable.
4. Open a draft Pull Request and request review.
