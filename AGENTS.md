# Agent Rules

## Language

- All source code, comments, log messages, and commit messages must be written in **English**.
- Russian is allowed only in user-facing output (e.g., CSV content that mirrors receipt data).

## Code Style

- Follow idiomatic Go conventions.
- Format all code with `gofmt` before committing.
- Run `go vet ./...` to catch common mistakes.
- Maximum line length: 120 characters.
- No tab characters for indentation in non-Go files (use spaces).
- Comments must answer **"Why?"**, not **"What?"**.

## Project Structure

```
ozon-bill-parser/
├── cmd/parser/main.go          # CLI entry point
├── internal/parser/
│   ├── parser.go               # core parsing logic
│   └── parser_test.go          # unit tests
└── plans/                      # architecture plans (Markdown)
```

## Dependencies

- Manage dependencies with Go modules: `go mod tidy` after any change.
- Do not vendor dependencies unless explicitly requested.
- PDF parsing: `github.com/ledongthuc/pdf`.

## Testing

Run all tests:
```bash
go test ./...
```

Run with verbose output:
```bash
go test -v ./...
```

Tests live in `internal/parser/parser_test.go`. Each new parsing feature must have a corresponding test.

## Building

```bash
go build -o bin/parser ./cmd/parser
```

## Running

```bash
./bin/parser -input "Чеки озон май" -output result.csv
# or
go run ./cmd/parser -input "Чеки озон май" -output result.csv
```

## What NOT to commit

- `result.csv` and any `*.csv` output files (covered by `.gitignore`).
- Compiled binaries in `bin/`.
- `vendor/` directory.
