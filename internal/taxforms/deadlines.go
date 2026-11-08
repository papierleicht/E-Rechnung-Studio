package taxforms

import (
	"fmt"
	"time"
)

// Period is one Voranmeldungszeitraum.
type Period struct {
	ID    string `json:"id"` // 2026-01 or 2026-Q1
	Label string `json:"label"`
	From  string `json:"from"`
	To    string `json:"to"`
	// Due is the transmission deadline: the 10th after the period, one month
	// later with Dauerfristverlängerung, moved to the next weekday (§ 108 Abs. 3
	// AO). Public holidays are not considered.
	Due string `json:"due"`
	// Last marks December or the fourth quarter, where the
	// Sondervorauszahlung is deducted (Kz 39).
	Last bool `json:"last"`
}

var monthNames = []string{"Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"}

// Periods lists the Voranmeldungszeiträume of a year for "monthly" or
// "quarterly"; other values return none.
func Periods(year int, frequency string, extension bool) []Period {
	var out []Period
	add := func(id, label string, first, last time.Month, isLast bool) {
		from := time.Date(year, first, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(year, last+1, 0, 0, 0, 0, 0, time.UTC)
		due := time.Date(year, last+1, 10, 0, 0, 0, 0, time.UTC)
		if extension {
			due = due.AddDate(0, 1, 0)
		}
		out = append(out, Period{ID: id, Label: label, From: from.Format(time.DateOnly), To: to.Format(time.DateOnly), Due: NextWeekday(due).Format(time.DateOnly), Last: isLast})
	}
	switch frequency {
	case "monthly":
		for m := time.January; m <= time.December; m++ {
			add(fmt.Sprintf("%d-%02d", year, m), fmt.Sprintf("%s %d", monthNames[m-1], year), m, m, m == time.December)
		}
	case "quarterly":
		for q := 1; q <= 4; q++ {
			first := time.Month(3*q - 2)
			add(fmt.Sprintf("%d-Q%d", year, q), fmt.Sprintf("%d. Quartal %d", q, year), first, first+2, q == 4)
		}
	}
	return out
}

// ZMPeriods lists the Meldezeiträume of the Zusammenfassende Meldung: the
// calendar quarter, or its months for quarters marked in monthly because
// deliveries of goods exceeded 50,000 € (§ 18a Abs. 1 UStG). The report is
// due on the 25th after the period; a Dauerfristverlängerung does not apply.
func ZMPeriods(year int, monthly map[int]bool) []Period {
	var out []Period
	add := func(id, label string, first, last time.Month) {
		from := time.Date(year, first, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(year, last+1, 0, 0, 0, 0, 0, time.UTC)
		due := time.Date(year, last+1, 25, 0, 0, 0, 0, time.UTC)
		out = append(out, Period{ID: id, Label: label, From: from.Format(time.DateOnly), To: to.Format(time.DateOnly), Due: NextWeekday(due).Format(time.DateOnly)})
	}
	for q := 1; q <= 4; q++ {
		first := time.Month(3*q - 2)
		if !monthly[q] {
			add(fmt.Sprintf("%d-Q%d", year, q), fmt.Sprintf("%d. Quartal %d", q, year), first, first+2)
			continue
		}
		for m := first; m <= first+2; m++ {
			add(fmt.Sprintf("%d-%02d", year, m), fmt.Sprintf("%s %d", monthNames[m-1], year), m, m)
		}
	}
	return out
}

// QuarterOf returns the calendar quarter (1–4) of a date like 2026-08-05.
func QuarterOf(date string) int {
	var y, m int
	if _, err := fmt.Sscanf(date, "%4d-%2d", &y, &m); err != nil || m < 1 || m > 12 {
		return 0
	}
	return (m + 2) / 3
}

// PeriodByID finds a period across years.
func PeriodByID(id, frequency string, extension bool) (Period, bool) {
	var year int
	if _, err := fmt.Sscanf(id, "%4d", &year); err != nil {
		return Period{}, false
	}
	for _, p := range Periods(year, frequency, extension) {
		if p.ID == id {
			return p, true
		}
	}
	return Period{}, false
}

// NextWeekday moves Saturday and Sunday to the following Monday.
func NextWeekday(t time.Time) time.Time {
	switch t.Weekday() {
	case time.Saturday:
		return t.AddDate(0, 0, 2)
	case time.Sunday:
		return t.AddDate(0, 0, 1)
	}
	return t
}

// AnnualDeadline is the filing deadline for the Einkommensteuer-,
// Umsatzsteuer- and EÜR-Erklärung of year without a tax advisor: 31 July of
// the following year (§ 149 Abs. 2 AO), moved to the next weekday.
func AnnualDeadline(year int) string {
	return NextWeekday(time.Date(year+1, time.July, 31, 0, 0, 0, 0, time.UTC)).Format(time.DateOnly)
}

// AdvisorDeadline is the deadline when a tax advisor prepares the returns:
// the end of February of the second following year (§ 149 Abs. 3 AO); for
// 2024 it was extended to 30 April 2026.
func AdvisorDeadline(year int) string {
	if year == 2024 {
		return "2026-04-30"
	}
	return NextWeekday(time.Date(year+2, time.March, 0, 0, 0, 0, 0, time.UTC)).Format(time.DateOnly)
}
