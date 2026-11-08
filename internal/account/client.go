package account

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// APIError is a non-2xx answer of the license server. Message is the
// server's German text if it sent one.
type APIError struct {
	Status  int
	Message string
	// Plan is the server's X-Papierleicht-Plan header: a plan the answer
	// does not cover, e.g. "support" on the 402 of the entitlement route.
	Plan string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("Der Kontoserver antwortete mit Status %d.", e.Status)
}

// StatusOf returns the HTTP status of an APIError, or 0 for network errors.
func StatusOf(err error) int {
	var api *APIError
	if errors.As(err, &api) {
		return api.Status
	}
	return 0
}

// ErrOffline wraps transport errors so callers can keep cached state.
var ErrOffline = errors.New("Der Kontoserver ist nicht erreichbar. Prüfe deine Internetverbindung.")

type client struct {
	base string
	http *http.Client
}

// maxResponse bounds JSON answers; backups use their own limit.
const maxResponse = 1 << 20

func (c *client) do(ctx context.Context, method, path, auth string, body io.Reader, header map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return nil, err
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	res, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w (%v)", ErrOffline, err)
	}
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return res, nil
	}
	defer res.Body.Close()
	var msg struct {
		Message string `json:"message"`
	}
	raw, _ := io.ReadAll(io.LimitReader(res.Body, maxResponse))
	_ = json.Unmarshal(raw, &msg)
	if res.StatusCode >= 500 && res.StatusCode != http.StatusInsufficientStorage {
		// Server failures count as temporarily unavailable.
		return nil, fmt.Errorf("%w (HTTP %d)", ErrOffline, res.StatusCode)
	}
	return nil, &APIError{Status: res.StatusCode, Message: strings.TrimSpace(msg.Message), Plan: res.Header.Get("X-Papierleicht-Plan")}
}

func (c *client) json(ctx context.Context, method, path, auth string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	res, err := c.do(ctx, method, path, auth, body, nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxResponse)).Decode(out); err != nil {
		return errors.New("Die Antwort des Kontoservers ist ungültig.")
	}
	return nil
}

type authResponse struct {
	Token  string `json:"token"`
	Record struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	} `json:"record"`
}

func (c *client) requestOTP(ctx context.Context, email string) (string, error) {
	var out struct {
		OTPID string `json:"otpId"`
	}
	err := c.json(ctx, "POST", "/api/collections/users/request-otp", "", map[string]string{"email": email}, &out)
	if err == nil && out.OTPID == "" {
		err = errors.New("Die Antwort des Kontoservers ist ungültig.")
	}
	return out.OTPID, err
}

func (c *client) authWithOTP(ctx context.Context, otpID, code string) (authResponse, error) {
	var out authResponse
	err := c.json(ctx, "POST", "/api/collections/users/auth-with-otp", "", map[string]string{"otpId": otpID, "password": code}, &out)
	if err == nil && out.Token == "" {
		err = errors.New("Die Antwort des Kontoservers ist ungültig.")
	}
	return out, err
}

func (c *client) authRefresh(ctx context.Context, token string) (authResponse, error) {
	var out authResponse
	err := c.json(ctx, "POST", "/api/collections/users/auth-refresh", token, nil, &out)
	if err == nil && out.Token == "" {
		err = errors.New("Die Antwort des Kontoservers ist ungültig.")
	}
	return out, err
}

type entitlementResponse struct {
	Token        string    `json:"token"`
	ProductID    string    `json:"productId"`
	ExpiresAt    time.Time `json:"expiresAt"`
	RefreshAfter time.Time `json:"refreshAfter"`
}

func (c *client) entitlement(ctx context.Context, token, deviceID, deviceName string) (entitlementResponse, error) {
	var out entitlementResponse
	err := c.json(ctx, "POST", "/api/license/entitlement", token, map[string]string{"deviceId": deviceID, "deviceName": deviceName}, &out)
	return out, err
}

// Device is one activated installation of the customer.
type Device struct {
	ID       string `json:"id"`
	DeviceID string `json:"deviceId"`
	Name     string `json:"name"`
	LastSeen string `json:"lastSeen"`
	Current  bool   `json:"current"`
}

func (c *client) devices(ctx context.Context, token string) ([]Device, error) {
	var out struct {
		Items []struct {
			ID       string `json:"id"`
			DeviceID string `json:"device_id"`
			Name     string `json:"name"`
			LastSeen string `json:"last_seen"`
		} `json:"items"`
	}
	q := url.Values{"perPage": {"100"}, "sort": {"-last_seen"}, "skipTotal": {"1"}}
	if err := c.json(ctx, "GET", "/api/collections/devices/records?"+q.Encode(), token, nil, &out); err != nil {
		return nil, err
	}
	devices := make([]Device, 0, len(out.Items))
	for _, d := range out.Items {
		devices = append(devices, Device{ID: d.ID, DeviceID: d.DeviceID, Name: d.Name, LastSeen: d.LastSeen})
	}
	return devices, nil
}

func (c *client) deleteDevice(ctx context.Context, token, id string) error {
	return c.json(ctx, "DELETE", "/api/collections/devices/records/"+url.PathEscape(id), token, nil, nil)
}

// signOutEverywhere ends every session of the account and removes all of
// its devices on the server.
func (c *client) signOutEverywhere(ctx context.Context, token string) error {
	return c.json(ctx, "POST", "/api/license/sign-out-everywhere", token, nil, nil)
}

// BackupVersion is the server-side metadata of one encrypted backup.
type BackupVersion struct {
	ID            string `json:"id"`
	DeviceID      string `json:"deviceId"`
	DeviceName    string `json:"deviceName"`
	Size          int64  `json:"size"`
	FormatVersion int    `json:"formatVersion"`
	SHA256        string `json:"sha256"`
	// Objects and LogicalSize describe the stored documents of this version.
	Objects     int       `json:"objects"`
	LogicalSize int64     `json:"logicalSize"`
	Created     time.Time `json:"created"`
}

// BackupState is what the server stores for a customer and what they may do.
type BackupState struct {
	// Access is "active", "read-only" or "none".
	Access        string     `json:"access"`
	ReadableUntil *time.Time `json:"readableUntil,omitempty"`
	DeletionAt    *time.Time `json:"deletionAt,omitempty"`
	UsedBytes     int64      `json:"usedBytes"`
	QuotaBytes    int64      `json:"quotaBytes"`
	Versions      int        `json:"versions"`
	Objects       int        `json:"objects"`
}

func (c *client) backupState(ctx context.Context, token string) (BackupState, error) {
	var out BackupState
	return out, c.json(ctx, "GET", "/api/backup/status", token, nil, &out)
}

// missingChunk is the server's limit for one query.
const missingChunk = 5000

func (c *client) missingObjects(ctx context.Context, token string, ids []string) ([]string, error) {
	missing := []string{}
	for batch := range slices.Chunk(ids, missingChunk) {
		var out struct {
			Missing []string `json:"missing"`
		}
		if err := c.json(ctx, "POST", "/api/backup/objects/missing", token, map[string][]string{"ids": batch}, &out); err != nil {
			return nil, err
		}
		missing = append(missing, out.Missing...)
	}
	return missing, nil
}

func (c *client) uploadObject(ctx context.Context, token, id string, data []byte) error {
	sum := sha256.Sum256(data)
	res, err := c.do(ctx, "PUT", "/api/backup/objects/"+url.PathEscape(id), token, bytes.NewReader(data), map[string]string{
		"Content-Type":    "application/octet-stream",
		"X-Backup-SHA256": hex.EncodeToString(sum[:]),
	})
	if err != nil {
		return err
	}
	return res.Body.Close()
}

func (c *client) downloadObject(ctx context.Context, token, id string) ([]byte, error) {
	return c.download(ctx, "/api/backup/objects/"+url.PathEscape(id), token, MaxBackupObject)
}

func (c *client) requestBackupDeletion(ctx context.Context, token string) (time.Time, error) {
	var out struct {
		DeletionAt time.Time `json:"deletionAt"`
	}
	return out.DeletionAt, c.json(ctx, "POST", "/api/backup/deletion", token, nil, &out)
}

// AuditReceipt is the server's signed note about the change log.
type AuditReceipt struct {
	Receipt string    `json:"receipt"`
	At      time.Time `json:"at"`
}

func (c *client) anchorAudit(ctx context.Context, token, device string, seq int64, hash string) (AuditReceipt, error) {
	var out AuditReceipt
	err := c.json(ctx, "POST", "/api/audit/anchors", token, map[string]any{"deviceId": device, "seq": seq, "hash": hash}, &out)
	if err == nil && out.Receipt == "" {
		err = errors.New("Die Antwort des Kontoservers ist ungültig.")
	}
	return out, err
}

func (c *client) auditReceipts(ctx context.Context, token string) ([]StoredReceipt, error) {
	var out struct {
		Items []StoredReceipt `json:"items"`
	}
	err := c.json(ctx, "GET", "/api/audit/anchors", token, nil, &out)
	return out.Items, err
}

func (c *client) cancelBackupDeletion(ctx context.Context, token string) error {
	return c.json(ctx, "DELETE", "/api/backup/deletion", token, nil, nil)
}

func (c *client) listBackups(ctx context.Context, token string) ([]BackupVersion, error) {
	var out struct {
		Items []BackupVersion `json:"items"`
	}
	if err := c.json(ctx, "GET", "/api/backup/versions", token, nil, &out); err != nil {
		return nil, err
	}
	if out.Items == nil {
		out.Items = []BackupVersion{}
	}
	return out.Items, nil
}

func (c *client) uploadBackup(ctx context.Context, token, deviceID, deviceName string, format int, manifest []byte, objects []string, logicalSize int64) (BackupVersion, error) {
	sum := sha256.Sum256(manifest)
	body, err := json.Marshal(map[string]any{"manifest": manifest, "objects": objects, "logicalSize": logicalSize})
	if err != nil {
		return BackupVersion{}, err
	}
	res, err := c.do(ctx, "POST", "/api/backup/versions", token, bytes.NewReader(body), map[string]string{
		"X-Backup-Device-Id":   deviceID,
		"X-Backup-Device-Name": headerSafe(deviceName),
		"X-Backup-Format":      strconv.Itoa(format),
		"X-Backup-SHA256":      hex.EncodeToString(sum[:]),
	})
	var v BackupVersion
	if err != nil {
		return v, err
	}
	defer res.Body.Close()
	if err := json.NewDecoder(io.LimitReader(res.Body, maxResponse)).Decode(&v); err != nil {
		return v, errors.New("Die Antwort des Kontoservers ist ungültig.")
	}
	return v, nil
}

// download returns stored ciphertext after checking its SHA-256 against the
// value the server stored at upload time.
func (c *client) download(ctx context.Context, path, token string, limit int64) ([]byte, error) {
	res, err := c.do(ctx, "GET", path, token, nil, nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%w (%v)", ErrOffline, err)
	}
	if int64(len(data)) > limit {
		return nil, errors.New("Die Sicherung ist größer als erwartet.")
	}
	sum := sha256.Sum256(data)
	if want := res.Header.Get("X-Backup-SHA256"); want != "" && !strings.EqualFold(want, hex.EncodeToString(sum[:])) {
		return nil, errors.New("Die heruntergeladene Sicherung ist unvollständig. Versuche es noch einmal.")
	}
	return data, nil
}

func (c *client) downloadBackup(ctx context.Context, token, id string, limit int64) ([]byte, error) {
	return c.download(ctx, "/api/backup/versions/"+url.PathEscape(id), token, limit)
}

// headerSafe keeps a device name within visible ASCII for an HTTP header.
func headerSafe(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 0x20 && r < 0x7f:
			b.WriteRune(r)
		case r == 'ä':
			b.WriteString("ae")
		case r == 'ö':
			b.WriteString("oe")
		case r == 'ü':
			b.WriteString("ue")
		case r == 'ß':
			b.WriteString("ss")
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
