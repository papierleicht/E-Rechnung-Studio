package pdfinvoice

import "strings"

// headingRunes is about what fits on one line of the 24pt seller heading.
const headingRunes = 32

// sellerHeading is the seller name as set at the top of a document.
type sellerHeading struct {
	Title, Subtitle string
	// Compact sets a long name without a natural break in a smaller heading.
	Compact bool
}

// splitSellerName keeps short names in the large heading unchanged. A long
// name with a spaced dash ("Atelier Lindner – Zweigniederlassung Köln") keeps
// its first part as heading and moves the rest to a smaller subheading. A very
// long name without such a break is set smaller as a whole. The full legal
// name still appears in the return address and the footer.
func splitSellerName(name string) sellerHeading {
	name = strings.TrimSpace(name)
	if len([]rune(name)) <= headingRunes {
		return sellerHeading{Title: name}
	}
	for _, dash := range []string{" – ", " — ", " - "} {
		head, rest, found := strings.Cut(name, dash)
		head, rest = strings.TrimSpace(head), strings.TrimSpace(rest)
		if found && head != "" && rest != "" && len([]rune(head)) <= headingRunes {
			return sellerHeading{Title: head, Subtitle: rest}
		}
	}
	return sellerHeading{Title: name, Compact: len([]rune(name)) > 2*headingRunes}
}
