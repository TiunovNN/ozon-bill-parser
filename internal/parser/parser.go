package parser

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
)

// serviceItemPrefixes are name prefixes that identify delivery/service line items
// whose cost is redistributed proportionally across regular goods.
var serviceItemPrefixes = []string{
	"Доставка",
	"Курьерская доставка",
	"Обработка заказа в пункте выдачи",
}

// isServiceItem reports whether a line item name represents a delivery or service charge.
func isServiceItem(name string) bool {
	for _, prefix := range serviceItemPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

var (
	// dateRe matches "DD.MM.YYYY HH:MM" anywhere in a line.
	dateRe = regexp.MustCompile(`\d{2}\.\d{2}\.\d{4} \d{2}:\d{2}`)

	// itemIndexRe matches a line that is just an item index: "1." or "12."
	// The ledongthuc/pdf library emits the number and dot on a line by itself.
	itemIndexRe = regexp.MustCompile(`^(\d+)\.$`)

	// qtyLineRe matches the quantity×unit line: "1 x 469,00"
	qtyLineRe = regexp.MustCompile(`^\d+ x [\d,]+$`)

	// priceLineRe matches a standalone price line starting with ≡: "≡469,00"
	priceLineRe = regexp.MustCompile(`^≡([\d,]+)$`)

	// taxLineRe matches tax/VAT lines that start with ≡ and follow a price line.
	// These are metadata and must be skipped.
	taxOrMetaRe = regexp.MustCompile(`^(в т\.ч\. НДС|Без НДС|ИНН продавца:|https?://|ФН:|РН ККТ:|ФД:|ФПД:|Сайт ФНС:|Тип операции:|Дата корректируемого|Безналичными|Зачет предварительной|Предварительная оплата|Полный расчет)`)
)

// RawItem holds a parsed line item before delivery redistribution.
type RawItem struct {
	Name    string
	Price   float64
	Service bool // true for delivery / service charges
}

// Receipt is the result of parsing a single PDF receipt.
type Receipt struct {
	DateTime time.Time
	Items    []RawItem
}

// Row is a final output row ready for CSV serialisation.
type Row struct {
	DateTime time.Time
	Name     string
	Price    float64
}

// ExtractText reads a PDF file and returns its plain-text content.
func ExtractText(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", fmt.Errorf("open pdf %q: %w", path, err)
	}
	defer f.Close()

	var sb strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			return "", fmt.Errorf("extract text from page %d of %q: %w", i, path, err)
		}
		sb.WriteString(text)
	}
	return sb.String(), nil
}

// ParseReceipt parses the plain-text content of a single Ozon receipt.
func ParseReceipt(text string) (Receipt, error) {
	lines := splitLines(text)

	dt, err := parseDateTime(lines)
	if err != nil {
		return Receipt{}, fmt.Errorf("parse date: %w", err)
	}

	items, err := parseItems(lines)
	if err != nil {
		return Receipt{}, fmt.Errorf("parse items: %w", err)
	}

	return Receipt{DateTime: dt, Items: items}, nil
}

// DistributeDelivery redistributes service-item costs proportionally across
// regular goods. The rounding residual is added to the last regular item.
// If there are no regular items, service items are returned as-is.
func DistributeDelivery(items []RawItem) []Row {
	var regular, service []RawItem
	for _, it := range items {
		if it.Service {
			service = append(service, it)
		} else {
			regular = append(regular, it)
		}
	}

	// No service charges — return regular items unchanged.
	if len(service) == 0 {
		return toRows(regular, nil)
	}

	// No regular items — return service items as-is (edge case).
	if len(regular) == 0 {
		return toRows(service, nil)
	}

	var deliveryTotal float64
	for _, s := range service {
		deliveryTotal += s.Price
	}

	var subtotal float64
	for _, r := range regular {
		subtotal += r.Price
	}

	addons := make([]float64, len(regular))
	var distributed float64
	for i, r := range regular {
		addon := roundCents(r.Price / subtotal * deliveryTotal)
		addons[i] = addon
		distributed += addon
	}

	// Assign rounding residual to the last item.
	residual := roundCents(deliveryTotal - distributed)
	addons[len(addons)-1] += residual

	return toRows(regular, addons)
}

// WriteCSV writes rows to a semicolon-delimited UTF-8 CSV file with BOM.
func WriteCSV(rows []Row, outPath string) error {
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create csv %q: %w", outPath, err)
	}
	defer f.Close()

	// Write UTF-8 BOM for Excel compatibility.
	if _, err := f.WriteString("\xEF\xBB\xBF"); err != nil {
		return fmt.Errorf("write bom: %w", err)
	}

	w := csv.NewWriter(f)
	w.Comma = ';'

	if err := w.Write([]string{"datetime", "name", "price"}); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	for _, r := range rows {
		record := []string{
			r.DateTime.Format("2006-01-02T15:04:05"),
			r.Name,
			strconv.FormatFloat(r.Price, 'f', 2, 64),
		}
		if err := w.Write(record); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}

	w.Flush()
	return w.Error()
}

// splitLines splits text into non-empty trimmed lines.
func splitLines(text string) []string {
	raw := strings.Split(text, "\n")
	out := make([]string, 0, len(raw))
	for _, l := range raw {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// parseDateTime finds the first date/time pattern in the receipt lines.
func parseDateTime(lines []string) (time.Time, error) {
	for _, l := range lines {
		if m := dateRe.FindString(l); m != "" {
			return time.Parse("02.01.2006 15:04", m)
		}
	}
	return time.Time{}, fmt.Errorf("no date/time found")
}

// parseItems extracts line items from the section between "Приход"/"Возврат прихода" and "ИТОГ".
//
// The ledongthuc/pdf library splits each receipt item across multiple lines:
//
//	"1."           ← item index line (name is empty here)
//	"Item name"    ← one or more name continuation lines
//	"1 x 213,27"   ← quantity × unit price
//	"≡213,27"      ← total price for this item (the value we capture)
func parseItems(lines []string) ([]RawItem, error) {
	// Find the items section start marker.
	start := -1
	for i, l := range lines {
		if l == "Приход" || l == "Возврат прихода" {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil, fmt.Errorf("section marker 'Приход'/'Возврат прихода' not found")
	}

	type parseState int
	const (
		stateIdle      parseState = iota // waiting for next item index line
		stateNameAccum                   // accumulating item name lines
		stateAfterQty                    // saw "N x price" line, expecting "≡total" next
	)

	var items []RawItem
	var currentName strings.Builder
	state := stateIdle

	for i := start; i < len(lines); i++ {
		l := lines[i]

		// Stop at the total line.
		if l == "ИТОГ" {
			break
		}

		switch state {
		case stateIdle:
			if itemIndexRe.MatchString(l) {
				currentName.Reset()
				state = stateNameAccum
			}

		case stateNameAccum:
			if qtyLineRe.MatchString(l) {
				// Quantity line found — name accumulation is complete.
				state = stateAfterQty
				continue
			}
			if itemIndexRe.MatchString(l) {
				// New item started without a price line — discard previous (malformed).
				currentName.Reset()
				state = stateNameAccum
				continue
			}
			// Skip metadata lines that may appear before the quantity line.
			if taxOrMetaRe.MatchString(l) {
				continue
			}
			// Accumulate name lines.
			if currentName.Len() > 0 {
				currentName.WriteString(" ")
			}
			currentName.WriteString(l)

		case stateAfterQty:
			if pm := priceLineRe.FindStringSubmatch(l); pm != nil {
				price, err := parsePrice(pm[1])
				if err != nil {
					return nil, fmt.Errorf("parse price %q: %w", pm[1], err)
				}
				name := strings.TrimSpace(currentName.String())
				items = append(items, RawItem{
					Name:    name,
					Price:   price,
					Service: isServiceItem(name),
				})
				currentName.Reset()
				state = stateIdle
				continue
			}
			// Unexpected line after qty — treat as name continuation (shouldn't happen).
			state = stateNameAccum
			if currentName.Len() > 0 {
				currentName.WriteString(" ")
			}
			currentName.WriteString(l)
		}
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no items found in receipt")
	}
	return items, nil
}

// parsePrice converts a Russian-formatted price string ("469,00") to float64.
func parsePrice(s string) (float64, error) {
	s = strings.ReplaceAll(s, ",", ".")
	return strconv.ParseFloat(s, 64)
}

// roundCents rounds a float to 2 decimal places.
func roundCents(v float64) float64 {
	return math.Round(v*100) / 100
}

// toRows converts RawItems to Rows, optionally adding per-item delivery addons.
func toRows(items []RawItem, addons []float64) []Row {
	rows := make([]Row, len(items))
	for i, it := range items {
		addon := 0.0
		if addons != nil {
			addon = addons[i]
		}
		rows[i] = Row{
			Name:  it.Name,
			Price: roundCents(it.Price + addon),
		}
	}
	return rows
}
