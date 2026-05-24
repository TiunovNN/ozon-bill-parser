# ozon-bill-parser

CLI utility that parses Ozon PDF receipts and produces a single CSV file with one row per purchased item.

## Output format

```
datetime;name;price
2026-05-01T19:01:00;Комод с ящиками для одежды ГУД ЛАКК Сага, 92х51х100 см, белый;12977.00
2026-05-23T17:30:00;Парафиновая смазка для цепи велосипеда MAX WAX Chain Wax 30мл;225.00
```

| Field      | Format                  | Notes                                      |
| ---------- | ----------------------- | ------------------------------------------ |
| `datetime` | `2006-01-02T15:04:05`   | Naive (no timezone), as printed on receipt |
| `name`     | UTF-8 string            | Full item name, multi-line names joined    |
| `price`    | Decimal with `.` separator | Delivery cost distributed proportionally |

- Delimiter: `;`
- Encoding: UTF-8 with BOM (compatible with Microsoft Excel)
- Rows sorted by `datetime` ascending

## Delivery cost distribution

Service line items (`Доставка`, `Курьерская доставка`, `Обработка заказа в пункте выдачи`) are **not** emitted as separate rows. Their cost is distributed proportionally across regular goods in the same receipt:

```
addon_i = round(price_i / subtotal * deliveryTotal, 2)
```

The rounding residual is added to the last item.

## Requirements

- Go 1.25+

## Build

```bash
go build -o bin/parser ./cmd/parser
```

## Run

```bash
./bin/parser -input "Чеки озон май" -output result.csv
# or without building:
go run ./cmd/parser -input "Чеки озон май" -output result.csv
```

| Flag       | Description                        | Default      |
| ---------- | ---------------------------------- | ------------ |
| `-input`   | Directory containing PDF receipts  | *(required)* |
| `-output`  | Path to the output CSV file        | `result.csv` |
| `-version` | Print version and exit             |              |

Unreadable or malformed PDFs are logged to stderr and skipped; processing continues.

## Test

```bash
go test ./...
# verbose:
go test -v ./...
```

## Releases

Pre-built binaries for every tagged release are available on the
[Releases page](../../releases).

### Download and install

**Linux (x86_64)**
```bash
VERSION=v0.1.0
curl -L "https://github.com/<owner>/ozon-bill-parser/releases/download/${VERSION}/ozon-bill-parser_${VERSION}_linux_amd64.tar.gz" \
  | tar -xz
./parser -version
```

**macOS (Apple Silicon)**
```bash
VERSION=v0.1.0
curl -L "https://github.com/<owner>/ozon-bill-parser/releases/download/${VERSION}/ozon-bill-parser_${VERSION}_darwin_arm64.tar.gz" \
  | tar -xz
./parser -version
```

**Windows (x86_64)**

Download `ozon-bill-parser_<version>_windows_amd64.zip` from the Releases page,
extract it, and run `parser.exe` from a terminal.

### Verify checksums

```bash
sha256sum -c checksums.txt
```

### Creating a release

```bash
git tag v0.1.0
git push origin v0.1.0
```

The `release` GitHub Actions workflow triggers automatically, builds all three
targets, and publishes the release with archives and `checksums.txt`.
Tags containing `-rc`, `-beta`, or `-alpha` are published as pre-releases.

## Project structure

```
ozon-bill-parser/
├── .github/workflows/
│   ├── ci.yml              # lint, vet, test on push/PR
│   └── release.yml         # cross-compile and publish on v* tag
├── cmd/parser/main.go      # CLI entry point
├── internal/parser/
│   ├── parser.go           # PDF extraction, receipt parsing, CSV writing
│   └── parser_test.go      # unit tests
└── plans/                  # architecture plans
```
