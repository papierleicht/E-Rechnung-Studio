package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"papierleicht.de/e-rechnung-studio/internal/engine"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/pdfinvoice"
	"papierleicht.de/e-rechnung-studio/internal/validation"
)

type generateInput struct {
	Company model.Company      `json:"company"`
	Invoice model.InvoiceDraft `json:"invoice"`
}

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 1
	}
	switch args[0] {
	case "validate":
		return validateCommand(args[1:])
	case "generate":
		return generateCommand(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", args[0])
		usage()
		return 1
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: e-rechnung-studio-cli validate <invoice.xml>")
	fmt.Fprintln(os.Stderr, "       e-rechnung-studio-cli generate -input invoice.json -output invoice.xml|invoice.pdf")
}

func newValidator() (*validation.Validator, func(), error) {
	root, err := os.MkdirTemp("", "e-rechnung-studio-cli-")
	if err != nil {
		return nil, nil, err
	}
	v, err := validation.New(root)
	if err != nil {
		_ = os.RemoveAll(root)
		return nil, nil, err
	}
	return v, func() { _ = os.RemoveAll(root) }, nil
}

func printJSON(value any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func validateCommand(args []string) int {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		usage()
		return 1
	}
	data, err := os.ReadFile(flags.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	v, cleanup, err := newValidator()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cleanup()
	result := v.Validate(data)
	if err = printJSON(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if result.State == "valid" {
		return 0
	}
	if result.State == "error" {
		return 1
	}
	return 2
}

func generateCommand(args []string) int {
	flags := flag.NewFlagSet("generate", flag.ContinueOnError)
	input := flags.String("input", "", "JSON file containing company and invoice")
	output := flags.String("output", "", "destination XML or PDF file")
	if err := flags.Parse(args); err != nil || *input == "" || *output == "" {
		usage()
		return 1
	}
	extension := strings.ToLower(filepath.Ext(*output))
	if extension != ".xml" && extension != ".pdf" {
		fmt.Fprintln(os.Stderr, "output must end in .xml or .pdf")
		return 1
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var request generateInput
	if err = json.Unmarshal(raw, &request); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if issues := validation.InputIssues(request.Invoice, request.Company); len(issues) > 0 {
		_ = printJSON(model.ValidationResult{State: "invalid", Issues: issues})
		return 2
	}
	invoiceEngine := engine.New()
	inv, err := invoiceEngine.Build(request.Invoice, request.Company)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	xmlData, err := invoiceEngine.XML(inv)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	v, cleanup, err := newValidator()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cleanup()
	result := v.Validate(xmlData)
	if result.State != "valid" {
		_ = printJSON(result)
		return 2
	}
	data := xmlData
	if extension == ".pdf" {
		data, err = pdfinvoice.RenderWithSnapshot(xmlData, &request.Company, &request.Invoice.Customer)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	dir := filepath.Dir(*output)
	if dir != "." {
		if err = os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	if err = os.WriteFile(*output, data, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	_ = printJSON(map[string]any{"state": "valid", "output": *output, "totals": engine.Totals(inv)})
	return 0
}
