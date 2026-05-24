# ozon-bill-parser

CLI utility that parses Ozon PDF receipts and produces a single CSV file with one row per purchased item.

## Output format

```
datetime;name;price;operation
2026-05-01T19:01:00;Комод с ящиками для одежды ГУД ЛАКК Сага, 92х51х100 см, белый;12977.00;расход
2026-05-23T17:30:00;Парафиновая смазка для цепи велосипеда MAX WAX Chain Wax 30мл;225.00;расход
2026-05-10T12:00:00;Наушники беспроводные;1490.00;доход
```

| Field       | Format                     | Notes                                                      |
| ----------- | -------------------------- | ---------------------------------------------------------- |
| `datetime`  | `2006-01-02T15:04:05`      | Naive (no timezone), as printed on receipt                 |
| `name`      | UTF-8 string               | Full item name, multi-line names joined                    |
| `price`     | Decimal with `.` separator | Delivery cost distributed proportionally                   |
| `operation` | `расход` / `доход` / `проверка` | Payment direction; see [Operation types](#operation-types) |

- Delimiter: `;`
- Encoding: UTF-8 with BOM (compatible with Microsoft Excel)
- Rows sorted by `datetime` ascending

## Operation types

| Value       | Meaning                                                                                   |
| ----------- | ----------------------------------------------------------------------------------------- |
| `расход`    | Regular purchase (`Приход` receipt) — money spent                                        |
| `доход`     | Refund (`Возврат прихода` receipt) — money returned                                      |
| `проверка`  | Partial prepayment offset — total is only partially covered by a prior prepayment; **requires manual review** |

## Receipt deduplication

Ozon sometimes issues multiple receipts for a single order when a prepayment is involved. The parser automatically skips duplicate receipts to avoid double-counting:

| Receipt type                    | Action  | Reason                                                                 |
| ------------------------------- | ------- | ---------------------------------------------------------------------- |
| `Зачет предварительной оплаты` only | **skipped** | Pure offset receipt — goods are already recorded in the Prepayment receipt |
| `Полный расчет` + full offset amount == total | **skipped** | Settlement document — goods are already recorded in the Prepayment receipt |
| `Предварительная оплата`        | emitted | Prepayment receipt — goods recorded here                               |
| `Полный расчет` with partial offset | emitted as `проверка` | Partial prepayment — needs manual review                  |

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

### Drag and drop (no terminal needed)

Drag the folder containing your PDF receipts and drop it onto the `parser` binary
(or `parser.exe` on Windows). The program will:

1. Read all `*.pdf` files inside the dropped folder.
2. Write `result.csv` **next to** the dropped folder (in its parent directory).

```
Before:
  ~/Downloads/
  └── Чеки озон май/
      ├── cheque_001.pdf
      └── cheque_002.pdf

After drop:
  ~/Downloads/
  ├── Чеки озон май/
  │   ├── cheque_001.pdf
  │   └── cheque_002.pdf
  └── result.csv          ← generated here
```

### Command line

```bash
./bin/parser -input "Чеки озон май" -output result.csv
# or without building:
go run ./cmd/parser -input "Чеки озон май" -output result.csv
```

| Flag       | Description                                                    | Default                              |
| ---------- | -------------------------------------------------------------- | ------------------------------------ |
| `-input`   | Directory containing PDF receipts                              | *(required, or drag-and-drop)*       |
| `-output`  | Path to the output CSV file                                    | `result.csv` next to the input dir   |
| `-version` | Print version and exit                                         |                                      |

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
