package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"

	"ozon-bill-parser/internal/parser"
)

// version is set at build time via -ldflags "-X main.version=<tag>".
var version = "dev"

func main() {
	inputDir := flag.String("input", "", "Directory containing Ozon PDF receipts (required)")
	outputFile := flag.String("output", "", "Path to the output CSV file (default: result.csv next to input dir)")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	// Support drag-and-drop: when the binary receives a directory as a positional
	// argument (no flags), treat it as the input directory and place result.csv
	// next to it so the user finds the output in the same location.
	if *inputDir == "" && flag.NArg() == 1 {
		*inputDir = flag.Arg(0)
	}

	if *inputDir == "" {
		fmt.Fprintln(os.Stderr, "error: -input flag is required")
		flag.Usage()
		os.Exit(1)
	}

	// Resolve the default output path: result.csv placed next to the input directory.
	if *outputFile == "" {
		absInput, err := filepath.Abs(*inputDir)
		if err != nil {
			log.Fatalf("resolve input path: %v", err)
		}
		*outputFile = filepath.Join(filepath.Dir(absInput), "result.csv")
	}

	pattern := filepath.Join(*inputDir, "*.pdf")
	files, err := filepath.Glob(pattern)
	if err != nil {
		log.Fatalf("glob %q: %v", pattern, err)
	}
	if len(files) == 0 {
		log.Fatalf("no PDF files found in %q", *inputDir)
	}

	var allRows []parser.Row

	for _, f := range files {
		rows, err := processFile(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skip %q: %v\n", f, err)
			continue
		}
		allRows = append(allRows, rows...)
	}

	// Sort all rows by date/time ascending.
	sort.Slice(allRows, func(i, j int) bool {
		return allRows[i].DateTime.Before(allRows[j].DateTime)
	})

	if err := parser.WriteCSV(allRows, *outputFile); err != nil {
		log.Fatalf("write csv: %v", err)
	}

	fmt.Printf("Written %d rows to %s\n", len(allRows), *outputFile)
}

// processFile extracts text from a PDF, parses the receipt, and returns final rows.
// Returns nil rows (no error) for receipts that must be skipped to avoid duplicates.
func processFile(path string) ([]parser.Row, error) {
	text, err := parser.ExtractText(path)
	if err != nil {
		return nil, fmt.Errorf("extract text: %w", err)
	}

	receipt, err := parser.ParseReceipt(text)
	if err != nil {
		return nil, fmt.Errorf("parse receipt: %w", err)
	}

	// Pure prepayment-offset receipts duplicate goods already in a Prepayment receipt.
	if receipt.Type == parser.ReceiptTypePrepaymentOffset {
		fmt.Fprintf(os.Stderr, "info: skip prepayment-offset receipt %q\n", path)
		return nil, nil
	}

	// A receipt whose "Зачет предварительной оплаты" amount fully covers the total
	// is a settlement document — the goods are already recorded in the corresponding
	// Prepayment receipt, so this one is a duplicate.
	if receipt.HasPrepaymentOffset && roundCents(receipt.PrepaymentOffsetAmount) == roundCents(receipt.TotalAmount) {
		fmt.Fprintf(os.Stderr, "info: skip full-prepayment-offset receipt (duplicate of prepayment) %q\n", path)
		return nil, nil
	}

	rows := parser.DistributeDelivery(receipt.Items, receipt.Operation)
	for i := range rows {
		rows[i].DateTime = receipt.DateTime
	}
	return rows, nil
}

// roundCents rounds a float64 to 2 decimal places.
func roundCents(v float64) float64 {
	return math.Round(v*100) / 100
}
