// Package shops normalizes read-only shop orders for local invoicing.
package shops

import (
	"context"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/model"
)

type Connection struct {
	ID               string `json:"id"`
	Provider         string `json:"provider"`
	Label            string `json:"label"`
	URL              string `json:"url"`
	SiteID           string `json:"siteId,omitempty"`
	Enabled          bool   `json:"enabled"`
	AutoCreateDrafts bool   `json:"autoCreateDrafts"`
	ImportFrom       string `json:"importFrom"`
	LastSync         string `json:"lastSync"`
	LastError        string `json:"lastError"`
	HasCredentials   bool   `json:"hasCredentials"`
}

// Credentials are stored only in the OS keychain, never in connection metadata.
type Credentials struct {
	APIKey         string `json:"apiKey,omitempty"`
	Username       string `json:"username,omitempty"`
	Password       string `json:"password,omitempty"`
	AccessToken    string `json:"accessToken"`
	ClientID       string `json:"clientId"`
	ClientSecret   string `json:"clientSecret"`
	ConsumerKey    string `json:"consumerKey"`
	ConsumerSecret string `json:"consumerSecret"`
}

type Order struct {
	ID                 string         `json:"id"`
	ConnectionID       string         `json:"connectionId"`
	ExternalID         string         `json:"externalId"`
	Number             string         `json:"number"`
	CustomerExternalID string         `json:"customerExternalId"`
	Customer           model.Customer `json:"customer"`
	Currency           string         `json:"currency"`
	Gross              string         `json:"gross"`
	CreatedAt          string         `json:"createdAt"`
	UpdatedAt          string         `json:"updatedAt"`
	Status             string         `json:"status"`
	Paid               bool           `json:"paid"`
	// PaidAt is when the shop received the payment, if it reports that.
	PaidAt      string                    `json:"paidAt,omitempty"`
	Lines       []model.InvoiceLine       `json:"lines"`
	Adjustments []model.InvoiceAdjustment `json:"adjustments"`
	// SourceTaxes hold the shop's VAT per rate where its line rounding differs.
	SourceTaxes     []model.SourceTax `json:"sourceTaxes,omitempty"`
	ReviewReason    string            `json:"reviewReason"`
	ConversionError string            `json:"conversionError,omitempty"`
	InvoiceID       string            `json:"invoiceId"`
	ImportedAt      string            `json:"importedAt"`
}

type Overview struct {
	Connections []Connection `json:"connections"`
	Orders      []Order      `json:"orders"`
	Syncing     bool         `json:"syncing"`
}

type SyncResult struct {
	Imported int `json:"imported"`
	Drafts   int `json:"drafts"`
	Review   int `json:"review"`
}

// FetchOrders must exhaust pagination or return an error; partial results are
// never checkpointed. since is an inclusive lower bound on order modifications.
type Fetcher interface {
	FetchOrders(context.Context, Connection, Credentials, time.Time) ([]Order, error)
}
