package mail

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Template is an editable subject/body pair with {Platzhalter} fields.
type Template struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// Document types sharing this package. Other features add their own type,
// default template and placeholder list here.
const (
	DocumentInvoice = "invoice"
	// Reminders carry their own texts from the reminder settings, so they have
	// no mail template here.
	DocumentReminder = "reminder"
	DocumentQuote    = "quote"
)

// DefaultTemplates are used until the user saves an own template.
var DefaultTemplates = map[string]Template{
	DocumentInvoice: {
		Subject: "Rechnung {Rechnungsnummer} von {Firma}",
		Body: "Guten Tag,\n\nanbei erhalten Sie unsere Rechnung {Rechnungsnummer} über {Betrag}.\n" +
			"Fällig: {Fälligkeit}\n\nBei Fragen antworten Sie gern auf diese E-Mail.\n\n" +
			"Mit freundlichen Grüßen\n{Firma}",
	},
	DocumentQuote: {
		Subject: "Angebot {Angebotsnummer} von {Firma}",
		Body: "Guten Tag,\n\nanbei erhalten Sie unser Angebot {Angebotsnummer} über {Betrag}.\n" +
			"Das Angebot gilt bis {Gültig bis}.\n\nBei Fragen antworten Sie gern auf diese E-Mail.\n\n" +
			"Mit freundlichen Grüßen\n{Firma}",
	},
}

// Placeholders lists the fields accepted in each document type's template.
var Placeholders = map[string][]string{
	DocumentInvoice: {"Rechnungsnummer", "Betrag", "Fälligkeit", "Kunde", "Firma"},
	DocumentQuote:   {"Angebotsnummer", "Betrag", "Gültig bis", "Kunde", "Firma"},
}

var placeholderPattern = regexp.MustCompile(`\{([^{}\r\n]{1,40})\}`)

// Render replaces {Name} with values[Name]. Unknown placeholders are an error
// so that a typo is never sent to a customer.
func Render(text string, values map[string]string) (string, error) {
	var unknown []string
	out := placeholderPattern.ReplaceAllStringFunc(text, func(match string) string {
		name := strings.TrimSpace(match[1 : len(match)-1])
		value, ok := values[name]
		if !ok {
			unknown = append(unknown, match)
			return match
		}
		return value
	})
	if len(unknown) > 0 {
		return out, fmt.Errorf("Den Platzhalter %s gibt es nicht. Verfügbar sind: %s.", unknown[0], available(values))
	}
	return out, nil
}

func available(values map[string]string) string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, "{"+name+"}")
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// ValidateTemplate checks a template against the placeholders of documentType.
func ValidateTemplate(documentType string, t Template) error {
	names, ok := Placeholders[documentType]
	if !ok {
		return fmt.Errorf("Die Vorlage „%s“ gibt es nicht.", documentType)
	}
	values := map[string]string{}
	for _, name := range names {
		values[name] = name
	}
	if strings.TrimSpace(t.Subject) == "" {
		return fmt.Errorf("Gib einen Betreff für die Vorlage ein.")
	}
	if _, err := Render(t.Subject, values); err != nil {
		return fmt.Errorf("Betreff: %w", err)
	}
	if _, err := Render(t.Body, values); err != nil {
		return fmt.Errorf("Text: %w", err)
	}
	return nil
}
