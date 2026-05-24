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

- Go 1.21+

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

| Flag      | Description                        | Default      |
| --------- | ---------------------------------- | ------------ |
| `-input`  | Directory containing PDF receipts  | *(required)* |
| `-output` | Path to the output CSV file        | `result.csv` |

Unreadable or malformed PDFs are logged to stderr and skipped; processing continues.

## Test

```bash
go test ./...
# verbose:
go test -v ./...
```

## Project structure

```
ozon-bill-parser/
├── cmd/parser/main.go          # CLI entry point
├── internal/parser/
│   ├── parser.go               # PDF extraction, receipt parsing, CSV writing
│   └── parser_test.go          # unit tests
└── plans/ozon-bill-parser.md   # architecture plan
```
