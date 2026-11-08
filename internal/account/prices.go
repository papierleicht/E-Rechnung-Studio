package account

import (
	"context"
	"net/http"
)

// Price is the final amount of one plan and interval, as the server charges it.
type Price struct {
	Plan     string `json:"plan"`
	Interval string `json:"interval"`
	Currency string `json:"currency"`
	Value    string `json:"value"`
}

// Prices returns what can be booked, so the app names amounts only as the
// server charges them. Like the newsletter, it needs no sign-in.
func (m *Manager) Prices(ctx context.Context) ([]Price, error) {
	if m.cfg.ServerURL == "" {
		return nil, ErrNotConfigured
	}
	return m.client.prices(ctx)
}

// prices lists what can be booked; the route needs no sign-in.
func (c *client) prices(ctx context.Context) ([]Price, error) {
	var out struct {
		Prices []Price `json:"prices"`
	}
	err := c.json(ctx, http.MethodGet, "/api/billing/mollie/prices", "", nil, &out)
	return out.Prices, err
}
