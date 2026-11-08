package app

import (
	"papierleicht.de/e-rechnung-studio/internal/account"
)

// ChatService is the page "Chat & Markt": the community chat and
// "Angebote & Gesuche" for Community Support and Plus customers. It uses the
// account session; the server decides who may take part.
type ChatService struct {
	manager *account.Manager
}

func NewChatService(accounts *AccountService) *ChatService {
	return &ChatService{manager: accounts.manager}
}

// Preview returns public numbers of the chat; it needs no account.
func (s *ChatService) Preview() (account.ChatPreview, error) {
	c, cancel := accountContext()
	defer cancel()
	return s.manager.ChatPreview(c)
}

// State tells whether the customer may take part (access "member"), still
// has to accept the guidelines ("join"), has no plan for it ("none") or is
// not signed in ("signed-out").
func (s *ChatService) State() (account.ChatState, error) {
	c, cancel := accountContext()
	defer cancel()
	return s.manager.ChatState(c)
}

// Join accepts the community guidelines of the given version and sets the
// display name; members use it to change their name.
func (s *ChatService) Join(displayName, guidelines string) (account.ChatState, error) {
	c, cancel := accountContext()
	defer cancel()
	return s.manager.JoinChat(c, displayName, guidelines)
}

// Messages returns the latest page (0, 0), older messages (0, smallest seq)
// or the changes since a cursor (cursor, 0).
func (s *ChatService) Messages(after, before int) (account.ChatPage, error) {
	c, cancel := accountContext()
	defer cancel()
	return s.manager.ChatMessages(c, after, before)
}

// Unread counts messages of others after since, the newest seq the page
// showed; it does not mark the customer as online.
func (s *ChatService) Unread(since int) (account.ChatUnread, error) {
	c, cancel := accountContext()
	defer cancel()
	return s.manager.ChatUnread(c, since)
}

func (s *ChatService) Send(body string) (account.ChatMessage, error) {
	c, cancel := accountContext()
	defer cancel()
	return s.manager.SendChatMessage(c, body)
}

func (s *ChatService) Delete(id string) error {
	c, cancel := accountContext()
	defer cancel()
	return s.manager.DeleteChatMessage(c, id)
}

func (s *ChatService) Report(id, reason string) error {
	c, cancel := accountContext()
	defer cancel()
	return s.manager.ReportChatMessage(c, id, reason)
}

// Listings returns one page of the board.
func (s *ChatService) Listings(filter account.MarketFilter) (account.MarketPage, error) {
	c, cancel := accountContext()
	defer cancel()
	return s.manager.MarketListings(c, filter)
}

func (s *ChatService) CreateListing(draft account.MarketDraft) (account.MarketListing, error) {
	c, cancel := accountContext()
	defer cancel()
	return s.manager.CreateListing(c, draft)
}

func (s *ChatService) RenewListing(id string) (account.MarketListing, error) {
	c, cancel := accountContext()
	defer cancel()
	return s.manager.RenewListing(c, id)
}

func (s *ChatService) DeleteListing(id string) error {
	c, cancel := accountContext()
	defer cancel()
	return s.manager.DeleteListing(c, id)
}

// ContactListing forwards an inquiry by e-mail to the author of a listing.
// Calling it means the customer agreed to share the account address.
func (s *ChatService) ContactListing(id, message string) error {
	c, cancel := accountContext()
	defer cancel()
	return s.manager.ContactListing(c, id, message)
}

func (s *ChatService) ReportListing(id, reason string) error {
	c, cancel := accountContext()
	defer cancel()
	return s.manager.ReportListing(c, id, reason)
}
