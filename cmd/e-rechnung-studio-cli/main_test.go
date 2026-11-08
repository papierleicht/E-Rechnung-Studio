package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	pdf "github.com/speedata/pdfdisassembler"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

func TestGeneratePDFWithValidatedInvoiceAttachment(t *testing.T) {
	root := t.TempDir()
	d := model.NewDraft()
	d.Number = "CLI-42"
	d.Customer = model.Customer{Name: "Kunde", Address: model.Address{Street: "Testweg 1", Postcode: "10115", City: "Berlin", Country: "DE"}}
	d.Lines = []model.InvoiceLine{{Name: "Leistung", Quantity: "1", Unit: "C62", UnitPrice: "100", TaxRate: "19", TaxCategory: "S"}}
	d.Adjustments = []model.InvoiceAdjustment{{Amount: "10", Reason: "Rabatt", TaxCategory: "S", TaxRate: "19"}}
	c := model.Company{Name: "CLI Firma", VATID: "DE123456789", Address: d.Customer.Address}
	data, err := json.Marshal(generateInput{Company: c, Invoice: d})
	if err != nil {
		t.Fatal(err)
	}
	input, output := filepath.Join(root, "input.json"), filepath.Join(root, "invoice.pdf")
	if err := os.WriteFile(input, data, 0600); err != nil {
		t.Fatal(err)
	}
	if exit := run([]string{"generate", "-input", input, "-output", output}); exit != 0 {
		t.Fatalf("exit: %d", exit)
	}
	r, err := pdf.OpenFile(output)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if count, err := r.PageCount(); err != nil || count == 0 {
		t.Fatal("no PDF pages")
	}
	if len(r.EmbeddedFiles()) != 1 {
		t.Fatal("missing XML attachment")
	}
	if exit := run([]string{"generate", "-input", input, "-output", filepath.Join(root, "wrong.txt")}); exit == 0 {
		t.Fatal("unsupported extension accepted")
	}
}
