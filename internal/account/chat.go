package account

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
)

// The community chat and "Angebote & Gesuche" run on the license server for
// customers with Community Support or Plus. The server decides who may take
// part; the app only shows what it answers.

// ChatAccess values of ChatState.Access.
const (
	// ChatSignedOut: the app has no session, the server was not asked.
	ChatSignedOut = "signed-out"
	ChatNone      = "none"
	ChatJoin      = "join"
	ChatMember    = "member"
)

// ChatState tells whether the customer may read and write.
type ChatState struct {
	Access string `json:"access"`
	// Guidelines is the version of the community guidelines the server asks
	// for; the app shows its text of that version.
	Guidelines string           `json:"guidelines"`
	Member     *ChatMemberState `json:"member,omitempty"`
}

type ChatMemberState struct {
	DisplayName string `json:"displayName"`
	// Role is "member" or "team" (papierleicht staff).
	Role       string `json:"role"`
	MutedUntil string `json:"mutedUntil,omitempty"`
}

// ChatPreview are the public numbers of the chat, for people without it.
type ChatPreview struct {
	Members      int                 `json:"members"`
	MessagesWeek int                 `json:"messagesWeek"`
	Online       int                 `json:"online"`
	Offers       int                 `json:"offers"`
	Requests     int                 `json:"requests"`
	Categories   []ChatCategoryCount `json:"categories"`
}

type ChatCategoryCount struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}

type ChatMessage struct {
	ID      string `json:"id"`
	Seq     int    `json:"seq"`
	Author  string `json:"author"`
	Role    string `json:"role"`
	Mine    bool   `json:"mine"`
	Body    string `json:"body"`
	Created string `json:"created"`
	// Removed means: drop the message with this ID.
	Removed bool `json:"removed"`
}

// ChatPage is one answer of the message route; see ChatMessages.
type ChatPage struct {
	Items    []ChatMessage `json:"items"`
	Cursor   int           `json:"cursor"`
	HasOlder bool          `json:"hasOlder"`
	More     bool          `json:"more"`
	Online   int           `json:"online"`
}

// MarketListing is one entry of "Angebote & Gesuche".
type MarketListing struct {
	ID string `json:"id"`
	// Kind is "offer" or "request".
	Kind       string `json:"kind"`
	Category   string `json:"category"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	PostalCode string `json:"postalCode"`
	City       string `json:"city"`
	Remote     bool   `json:"remote"`
	Contact    string `json:"contact"`
	Author     string `json:"author"`
	Role       string `json:"role"`
	Mine       bool   `json:"mine"`
	Created    string `json:"created"`
	ExpiresAt  string `json:"expiresAt"`
}

// MarketDraft is a new listing.
type MarketDraft struct {
	Kind       string `json:"kind"`
	Category   string `json:"category"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	PostalCode string `json:"postalCode"`
	City       string `json:"city"`
	Remote     bool   `json:"remote"`
	Contact    string `json:"contact"`
}

// MarketFilter narrows the board; empty fields do not filter. Area is the
// start of a postal code. Mine lists only the customer's own listings.
// Page counts from 1; 0 means the first page.
type MarketFilter struct {
	Kind     string `json:"kind"`
	Category string `json:"category"`
	Area     string `json:"area"`
	Mine     bool   `json:"mine"`
	Page     int    `json:"page"`
}

// MarketPage is one page of the board. Page is the page the server
// answered with: a page past the end gives the last one.
type MarketPage struct {
	Items    []MarketListing `json:"items"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"pageSize"`
}

var errChatOffline = errors.New("Gerade keine Verbindung zur Community. Prüfe deine Internetverbindung, die App versucht es gleich wieder.")

// chatError keeps the server's German message and explains the rest.
func chatError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrOffline), errors.Is(err, context.DeadlineExceeded):
		return errChatOffline
	case StatusOf(err) == http.StatusUnauthorized:
		return errors.New("Deine Anmeldung ist abgelaufen. Melde dich unter Einstellungen → Konto & Abo noch einmal an.")
	}
	return err
}

// ChatPreview needs no account.
func (m *Manager) ChatPreview(ctx context.Context) (ChatPreview, error) {
	var p ChatPreview
	if m.cfg.ServerURL == "" {
		return p, ErrNotConfigured
	}
	err := m.client.json(ctx, "GET", "/api/chat/preview", "", nil, &p)
	if p.Categories == nil {
		p.Categories = []ChatCategoryCount{}
	}
	return p, chatError(err)
}

// ChatState asks the server; without a session it answers ChatSignedOut.
func (m *Manager) ChatState(ctx context.Context) (ChatState, error) {
	token, err := m.authToken()
	if errors.Is(err, ErrNotConfigured) {
		return ChatState{}, err
	}
	if err != nil {
		return ChatState{Access: ChatSignedOut}, nil
	}
	var st ChatState
	err = m.client.json(ctx, "GET", "/api/chat/status", token, nil, &st)
	if StatusOf(err) == http.StatusUnauthorized {
		return ChatState{Access: ChatSignedOut}, nil
	}
	return st, chatError(err)
}

// JoinChat accepts the guidelines of the given version under a display name;
// it also changes the name later.
func (m *Manager) JoinChat(ctx context.Context, displayName, guidelines string) (ChatState, error) {
	token, err := m.authToken()
	if err != nil {
		return ChatState{}, err
	}
	var st ChatState
	err = m.client.json(ctx, "POST", "/api/chat/join", token, map[string]string{"displayName": displayName, "guidelines": guidelines}, &st)
	return st, chatError(err)
}

// ChatMessages returns the latest page (after and before 0), older messages
// (before = the smallest seq shown) or the changes since cursor (after).
func (m *Manager) ChatMessages(ctx context.Context, after, before int) (ChatPage, error) {
	token, err := m.authToken()
	if err != nil {
		return ChatPage{}, err
	}
	q := url.Values{}
	if after > 0 {
		q.Set("after", strconv.Itoa(after))
	}
	if before > 0 {
		q.Set("before", strconv.Itoa(before))
	}
	path := "/api/chat/messages"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var page ChatPage
	err = m.client.json(ctx, "GET", path, token, nil, &page)
	if page.Items == nil {
		page.Items = []ChatMessage{}
	}
	return page, chatError(err)
}

// ChatUnread is the number for the menu badge.
type ChatUnread struct {
	// Count of messages by others after the seq the app showed last, at most 99.
	Count int `json:"count"`
	// Latest is the newest seq; the app keeps it as its mark.
	Latest int `json:"latest"`
}

// ChatUnread counts new messages without counting as a visit.
func (m *Manager) ChatUnread(ctx context.Context, since int) (ChatUnread, error) {
	token, err := m.authToken()
	if err != nil {
		return ChatUnread{}, err
	}
	var u ChatUnread
	err = m.client.json(ctx, "GET", "/api/chat/unread?since="+strconv.Itoa(since), token, nil, &u)
	return u, chatError(err)
}

func (m *Manager) SendChatMessage(ctx context.Context, body string) (ChatMessage, error) {
	token, err := m.authToken()
	if err != nil {
		return ChatMessage{}, err
	}
	var msg ChatMessage
	err = m.client.json(ctx, "POST", "/api/chat/messages", token, map[string]string{"body": body}, &msg)
	return msg, chatError(err)
}

func (m *Manager) DeleteChatMessage(ctx context.Context, id string) error {
	token, err := m.authToken()
	if err != nil {
		return err
	}
	return chatError(m.client.json(ctx, "DELETE", "/api/chat/messages/"+url.PathEscape(id), token, nil, nil))
}

func (m *Manager) ReportChatMessage(ctx context.Context, id, reason string) error {
	token, err := m.authToken()
	if err != nil {
		return err
	}
	return chatError(m.client.json(ctx, "POST", "/api/chat/messages/"+url.PathEscape(id)+"/report", token, map[string]string{"reason": reason}, nil))
}

func (m *Manager) MarketListings(ctx context.Context, f MarketFilter) (MarketPage, error) {
	token, err := m.authToken()
	if err != nil {
		return MarketPage{Items: []MarketListing{}}, err
	}
	q := url.Values{}
	if f.Mine {
		q.Set("mine", "1")
	}
	for k, v := range map[string]string{"kind": f.Kind, "category": f.Category, "area": f.Area} {
		if v != "" {
			q.Set(k, v)
		}
	}
	if f.Page > 1 {
		q.Set("page", strconv.Itoa(f.Page))
	}
	path := "/api/market/listings"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var page MarketPage
	err = m.client.json(ctx, "GET", path, token, nil, &page)
	if page.Items == nil {
		page.Items = []MarketListing{}
	}
	return page, chatError(err)
}

func (m *Manager) CreateListing(ctx context.Context, d MarketDraft) (MarketListing, error) {
	token, err := m.authToken()
	if err != nil {
		return MarketListing{}, err
	}
	var l MarketListing
	err = m.client.json(ctx, "POST", "/api/market/listings", token, d, &l)
	return l, chatError(err)
}

func (m *Manager) RenewListing(ctx context.Context, id string) (MarketListing, error) {
	token, err := m.authToken()
	if err != nil {
		return MarketListing{}, err
	}
	var l MarketListing
	err = m.client.json(ctx, "POST", "/api/market/listings/"+url.PathEscape(id)+"/renew", token, nil, &l)
	return l, chatError(err)
}

func (m *Manager) DeleteListing(ctx context.Context, id string) error {
	token, err := m.authToken()
	if err != nil {
		return err
	}
	return chatError(m.client.json(ctx, "DELETE", "/api/market/listings/"+url.PathEscape(id), token, nil, nil))
}

// ContactListing sends an inquiry by e-mail to the author of a listing; the
// answer goes to the customer's account address, which the customer agreed to
// share.
func (m *Manager) ContactListing(ctx context.Context, id, message string) error {
	token, err := m.authToken()
	if err != nil {
		return err
	}
	return chatError(m.client.json(ctx, "POST", "/api/market/listings/"+url.PathEscape(id)+"/contact", token, map[string]any{"message": message, "shareEmail": true}, nil))
}

func (m *Manager) ReportListing(ctx context.Context, id, reason string) error {
	token, err := m.authToken()
	if err != nil {
		return err
	}
	return chatError(m.client.json(ctx, "POST", "/api/market/listings/"+url.PathEscape(id)+"/report", token, map[string]string{"reason": reason}, nil))
}
