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

// ReceiptType represents the payment type of a receipt.
type ReceiptType int

const (
	ReceiptTypeUnknown          ReceiptType = iota
	ReceiptTypeFullPayment                  // Полный расчет
	ReceiptTypePrepayment                   // Предварительная оплата
	ReceiptTypePrepaymentOffset             // Зачет предварительной оплаты — duplicate, must be skipped
)

// OperationType indicates the financial direction of a receipt.
type OperationType string

const (
	OperationExpense OperationType = "расход"   // Приход — money spent
	OperationIncome  OperationType = "доход"    // Возврат прихода — money returned
	OperationCheck   OperationType = "проверка" // Partial prepayment offset — needs manual review
)

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
	DateTime  time.Time
	Items     []RawItem
	Type      ReceiptType
	Operation OperationType
	// HasPrepaymentOffset is true when the receipt footer contains
	// "Зачет предварительной оплаты".
	HasPrepaymentOffset bool
	// TotalAmount is the grand total from the "ИТОГ" footer line (≡NNN,NN).
	TotalAmount float64
	// PrepaymentOffsetAmount is the amount on the "Зачет предварительной оплаты"
	// footer line. When it equals TotalAmount the receipt is a pure duplicate of
	// the corresponding Prepayment receipt and must be skipped.
	// When it is less than TotalAmount the receipt has a partial offset and is
	// flagged with OperationCheck for manual review.
	PrepaymentOffsetAmount float64
}

// Row is a final output row ready for CSV serialisation.
type Row struct {
	DateTime  time.Time
	Name      string
	Price     float64
	Operation OperationType
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

	rt, hasPrepOffset := detectReceiptType(lines)
	totalAmt, offsetAmt := detectTotals(lines)

	// Determine operation type.
	op := detectOperation(lines)
	if hasPrepOffset && rt != ReceiptTypePrepaymentOffset {
		if offsetAmt > 0 && roundCents(offsetAmt) == roundCents(totalAmt) {
			// Full prepayment offset: this receipt duplicates the Prepayment receipt.
			// Mark it so the caller can skip it.
			op = OperationExpense // will be skipped; value doesn't matter
		} else {
			// Partial prepayment offset: needs manual review.
			op = OperationCheck
		}
	}

	return Receipt{
		DateTime:               dt,
		Items:                  items,
		Type:                   rt,
		Operation:              op,
		HasPrepaymentOffset:    hasPrepOffset,
		TotalAmount:            totalAmt,
		PrepaymentOffsetAmount: offsetAmt,
	}, nil
}

// detectReceiptType scans lines after "ИТОГ" to determine the payment type and
// whether the receipt contains a prepayment-offset line.
//
// A receipt can be FullPayment AND contain "Зачет предварительной оплаты" — this
// means it is the final settlement of a prior Prepayment receipt. In that case
// hasPrepOffset is true and the paired Prepayment receipt must be skipped.
//
// A receipt whose ONLY payment method is "Зачет предварительной оплаты" (no
// "Полный расчет" / "Предварительная оплата" line) is typed PrepaymentOffset and
// is itself a duplicate that must be skipped.
func detectReceiptType(lines []string) (rt ReceiptType, hasPrepOffset bool) {
	afterTotal := false
	for _, l := range lines {
		if l == "ИТОГ" {
			afterTotal = true
			continue
		}
		if !afterTotal {
			continue
		}
		switch l {
		case "Полный расчет":
			rt = ReceiptTypeFullPayment
		case "Предварительная оплата":
			if rt == ReceiptTypeUnknown {
				rt = ReceiptTypePrepayment
			}
		case "Зачет предварительной оплаты":
			hasPrepOffset = true
			if rt == ReceiptTypeUnknown {
				rt = ReceiptTypePrepaymentOffset
			}
		}
	}
	return rt, hasPrepOffset
}

// detectOperation returns OperationExpense for "Приход" receipts and
// OperationIncome for "Возврат прихода" (return/refund) receipts.
func detectOperation(lines []string) OperationType {
	for _, l := range lines {
		switch l {
		case "Приход":
			return OperationExpense
		case "Возврат прихода":
			return OperationIncome
		}
	}
	return OperationExpense
}

// detectTotals scans the footer (after "ИТОГ") and returns:
//   - totalAmount: the first ≡NNN,NN value immediately after "ИТОГ"
//   - offsetAmount: the ≡NNN,NN value on the line immediately after
//     "Зачет предварительной оплаты"
func detectTotals(lines []string) (totalAmount, offsetAmount float64) {
	afterTotal := false
	expectTotalPrice := false
	expectOffsetPrice := false

	for _, l := range lines {
		if l == "ИТОГ" {
			afterTotal = true
			expectTotalPrice = true
			continue
		}
		if !afterTotal {
			continue
		}

		if expectTotalPrice {
			if pm := priceLineRe.FindStringSubmatch(l); pm != nil {
				if v, err := parsePrice(pm[1]); err == nil {
					totalAmount = v
				}
				expectTotalPrice = false
				continue
			}
			// Non-price line after ИТОГ — stop looking for total price.
			expectTotalPrice = false
		}

		if l == "Зачет предварительной оплаты" {
			expectOffsetPrice = true
			continue
		}

		if expectOffsetPrice {
			if pm := priceLineRe.FindStringSubmatch(l); pm != nil {
				if v, err := parsePrice(pm[1]); err == nil {
					offsetAmount = v
				}
			}
			expectOffsetPrice = false
		}
	}
	return totalAmount, offsetAmount
}

// DistributeDelivery redistributes service-item costs proportionally across
// regular goods. The rounding residual is added to the last regular item.
// If there are no regular items, service items are returned as-is.
func DistributeDelivery(items []RawItem, op OperationType) []Row {
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
		return toRows(regular, nil, op)
	}

	// No regular items — return service items as-is (edge case).
	if len(regular) == 0 {
		return toRows(service, nil, op)
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

	return toRows(regular, addons, op)
}

// WriteCSV writes rows to a semicolon-delimited UTF-8 CSV file with BOM.
// Columns: datetime, name, price, operation.
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

	if err := w.Write([]string{"datetime", "name", "price", "operation"}); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	for _, r := range rows {
		record := []string{
			r.DateTime.Format("2006-01-02T15:04:05"),
			r.Name,
			strconv.FormatFloat(r.Price, 'f', 2, 64),
			string(r.Operation),
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
func toRows(items []RawItem, addons []float64, op OperationType) []Row {
	rows := make([]Row, len(items))
	for i, it := range items {
		addon := 0.0
		if addons != nil {
			addon = addons[i]
		}
		rows[i] = Row{
			Name:      it.Name,
			Price:     roundCents(it.Price + addon),
			Operation: op,
		}
	}
	return rows
}
