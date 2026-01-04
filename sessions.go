package sdk

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/modelrelay/modelrelay/sdk/go/generated"
)

// Session represents a session in the ModelRelay API.
type Session = generated.SessionResponse

// SessionWithMessages represents a session with its full message history.
type SessionWithMessages = generated.SessionWithMessagesResponse

// SessionMessage represents a single message in a session.
type SessionMessage = generated.SessionMessageResponse

// SessionCreateRequest contains the fields to create a session.
type SessionCreateRequest = generated.SessionCreateRequest

// SessionMessageCreateRequest contains the fields to add a message to a session.
type SessionMessageCreateRequest = generated.SessionMessageCreateRequest

// SessionListResponse is a paginated list of sessions.
type SessionListResponse = generated.SessionListResponse

// SessionsClient provides methods for managing sessions.
//
// Sessions enable multi-turn conversations with persistent history. Use sessions to:
// - Track conversation context across multiple runs
// - Store and retrieve message history
// - Associate runs with conversation sessions
//
// Example:
//
//	// Create a new session
//	session, err := client.Sessions.Create(ctx, sdk.SessionCreateRequest{
//	    Metadata: map[string]any{"name": "Feature discussion"},
//	})
//
//	// Get session with messages
//	sessionWithMessages, err := client.Sessions.Get(ctx, session.Id)
//
//	// Add a message to the session
//	message, err := client.Sessions.AddMessage(ctx, session.Id, sdk.SessionMessageCreateRequest{
//	    Role:    "user",
//	    Content: []map[string]any{{"type": "text", "text": "Hello!"}},
//	})
type SessionsClient struct {
	client *Client
}

// ensureInitialized returns an error if the client is not properly initialized.
func (c *SessionsClient) ensureInitialized() error {
	if c == nil || c.client == nil {
		return fmt.Errorf("sdk: sessions client not initialized")
	}
	return nil
}

// Create creates a new session.
//
// Sessions are project-scoped and can optionally be associated with an customer.
//
// Example:
//
//	session, err := client.Sessions.Create(ctx, sdk.SessionCreateRequest{
//	    CustomerId: &customerID,
//	    Metadata: map[string]any{"project": "my-app"},
//	})
func (c *SessionsClient) Create(ctx context.Context, req SessionCreateRequest) (Session, error) {
	if err := c.ensureInitialized(); err != nil {
		return Session{}, err
	}

	var resp Session
	if err := c.client.sendAndDecode(ctx, http.MethodPost, "/sessions", req, &resp); err != nil {
		return Session{}, err
	}
	return resp, nil
}

// ListOptions contains options for listing sessions.
type ListOptions struct {
	// Limit is the maximum number of sessions to return (default: 50, max: 100).
	Limit int
	// Offset is the number of sessions to skip (for pagination).
	Offset int
	// Cursor is the pagination cursor from a previous response's NextCursor field.
	Cursor string
	// CustomerID filters sessions by customer ID.
	CustomerID *uuid.UUID
}

// List returns a paginated list of sessions.
//
// Use the NextCursor field in the response for pagination.
//
// Example:
//
//	resp, err := client.Sessions.List(ctx, sdk.ListOptions{Limit: 10})
//	for _, session := range resp.Sessions {
//	    fmt.Printf("Session %s: %d messages\n", session.Id, session.MessageCount)
//	}
//
//	// Fetch next page using cursor
//	if resp.NextCursor != nil {
//	    nextPage, err := client.Sessions.List(ctx, sdk.ListOptions{Cursor: *resp.NextCursor})
//	}
func (c *SessionsClient) List(ctx context.Context, opts ListOptions) (SessionListResponse, error) {
	if err := c.ensureInitialized(); err != nil {
		return SessionListResponse{}, err
	}

	path := "/sessions"
	params := url.Values{}
	if opts.Limit > 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Offset > 0 {
		params.Set("offset", strconv.Itoa(opts.Offset))
	}
	if opts.Cursor != "" {
		params.Set("cursor", opts.Cursor)
	}
	if opts.CustomerID != nil {
		params.Set("customer_id", opts.CustomerID.String())
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var resp SessionListResponse
	if err := c.client.sendAndDecode(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return SessionListResponse{}, err
	}
	return resp, nil
}

// Get retrieves a session by ID, including its full message history.
//
// Example:
//
//	session, err := client.Sessions.Get(ctx, sessionID)
//	for _, msg := range session.Messages {
//	    fmt.Printf("[%s] %v\n", msg.Role, msg.Content)
//	}
func (c *SessionsClient) Get(ctx context.Context, sessionID uuid.UUID) (SessionWithMessages, error) {
	if err := c.ensureInitialized(); err != nil {
		return SessionWithMessages{}, err
	}
	if sessionID == uuid.Nil {
		return SessionWithMessages{}, fmt.Errorf("sdk: session_id is required")
	}

	path := fmt.Sprintf("/sessions/%s", sessionID.String())
	var resp SessionWithMessages
	if err := c.client.sendAndDecode(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return SessionWithMessages{}, err
	}
	return resp, nil
}

// Delete deletes a session by ID.
//
// Requires a secret key (not publishable key).
//
// Example:
//
//	err := client.Sessions.Delete(ctx, sessionID)
func (c *SessionsClient) Delete(ctx context.Context, sessionID uuid.UUID) error {
	if err := c.ensureInitialized(); err != nil {
		return err
	}
	if sessionID == uuid.Nil {
		return fmt.Errorf("sdk: session_id is required")
	}

	path := fmt.Sprintf("/sessions/%s", sessionID.String())
	return c.client.sendAndDecode(ctx, http.MethodDelete, path, nil, nil)
}

// AddMessage appends a message to a session.
//
// Messages can be user, assistant, or tool messages. Assistant messages
// can optionally include a run_id to link them to a workflow run.
//
// Example:
//
//	msg, err := client.Sessions.AddMessage(ctx, sessionID, sdk.SessionMessageCreateRequest{
//	    Role: "user",
//	    Content: []map[string]any{{"type": "text", "text": "Hello!"}},
//	})
func (c *SessionsClient) AddMessage(ctx context.Context, sessionID uuid.UUID, req SessionMessageCreateRequest) (SessionMessage, error) {
	if err := c.ensureInitialized(); err != nil {
		return SessionMessage{}, err
	}
	if sessionID == uuid.Nil {
		return SessionMessage{}, fmt.Errorf("sdk: session_id is required")
	}

	role := strings.TrimSpace(req.Role)
	if role == "" {
		return SessionMessage{}, fmt.Errorf("sdk: role is required")
	}
	switch role {
	case "user", "assistant", "tool":
		// valid
	default:
		return SessionMessage{}, fmt.Errorf("sdk: invalid role %q (must be user, assistant, or tool)", role)
	}

	if len(req.Content) == 0 {
		return SessionMessage{}, fmt.Errorf("sdk: content is required")
	}

	// Use trimmed role in request to normalize whitespace
	normalizedReq := SessionMessageCreateRequest{
		Role:    role,
		Content: req.Content,
		RunId:   req.RunId,
	}

	path := fmt.Sprintf("/sessions/%s/messages", sessionID.String())
	var resp SessionMessage
	if err := c.client.sendAndDecode(ctx, http.MethodPost, path, normalizedReq, &resp); err != nil {
		return SessionMessage{}, err
	}
	return resp, nil
}
