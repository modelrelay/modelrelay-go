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

// MessageSendRequest contains fields to send a message.
type MessageSendRequest = generated.MessageSendRequest

// MessageResponse represents a message payload.
type MessageResponse = generated.MessageResponse

// MessageListResponse represents a paginated list of messages.
type MessageListResponse = generated.MessageListResponse

// MaxMessageListLimit caps list pagination.
const MaxMessageListLimit int32 = 200

// MessageListOptions controls inbox listing.
type MessageListOptions struct {
	To       string
	ThreadID *uuid.UUID
	Unread   *bool
	Limit    int32
	Offset   int32
}

// MessagesClient provides access to messaging endpoints.
type MessagesClient struct {
	client *Client
}

func (c *MessagesClient) ensureInitialized() error {
	if c == nil || c.client == nil {
		return fmt.Errorf("sdk: messages client not initialized")
	}
	return nil
}

// Send sends a message to the specified address.
func (c *MessagesClient) Send(ctx context.Context, req MessageSendRequest) (MessageResponse, error) {
	if err := c.ensureInitialized(); err != nil {
		return MessageResponse{}, err
	}
	if strings.TrimSpace(req.To) == "" {
		return MessageResponse{}, fmt.Errorf("sdk: to is required")
	}
	if strings.TrimSpace(req.Subject) == "" {
		return MessageResponse{}, fmt.Errorf("sdk: subject is required")
	}
	if req.Body == nil {
		return MessageResponse{}, fmt.Errorf("sdk: body is required")
	}
	var resp MessageResponse
	if err := c.client.sendAndDecode(ctx, http.MethodPost, "/messages", req, &resp); err != nil {
		return MessageResponse{}, err
	}
	return resp, nil
}

// List returns messages for a mailbox address or thread.
func (c *MessagesClient) List(ctx context.Context, opts MessageListOptions) (MessageListResponse, error) {
	if err := c.ensureInitialized(); err != nil {
		return MessageListResponse{}, err
	}
	if strings.TrimSpace(opts.To) == "" && opts.ThreadID == nil {
		return MessageListResponse{}, fmt.Errorf("sdk: to or thread_id required")
	}
	if opts.Limit < 0 || opts.Offset < 0 {
		return MessageListResponse{}, fmt.Errorf("sdk: limit and offset must be non-negative")
	}
	if opts.Limit > MaxMessageListLimit {
		return MessageListResponse{}, fmt.Errorf("sdk: limit exceeds maximum (%d)", MaxMessageListLimit)
	}

	params := url.Values{}
	if strings.TrimSpace(opts.To) != "" {
		params.Set("to", strings.TrimSpace(opts.To))
	}
	if opts.ThreadID != nil {
		params.Set("thread_id", opts.ThreadID.String())
	}
	if opts.Unread != nil {
		params.Set("unread", strconv.FormatBool(*opts.Unread))
	}
	if opts.Limit > 0 {
		params.Set("limit", strconv.FormatInt(int64(opts.Limit), 10))
	}
	if opts.Offset > 0 {
		params.Set("offset", strconv.FormatInt(int64(opts.Offset), 10))
	}
	path := "/messages"
	if encoded := params.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var resp MessageListResponse
	if err := c.client.sendAndDecode(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return MessageListResponse{}, err
	}
	return resp, nil
}

// Get fetches a message by ID.
func (c *MessagesClient) Get(ctx context.Context, id uuid.UUID) (MessageResponse, error) {
	if err := c.ensureInitialized(); err != nil {
		return MessageResponse{}, err
	}
	if id == uuid.Nil {
		return MessageResponse{}, fmt.Errorf("sdk: message_id is required")
	}
	var resp MessageResponse
	if err := c.client.sendAndDecode(ctx, http.MethodGet, "/messages/"+id.String(), nil, &resp); err != nil {
		return MessageResponse{}, err
	}
	return resp, nil
}

// MarkRead marks a message as read.
func (c *MessagesClient) MarkRead(ctx context.Context, id uuid.UUID) error {
	if err := c.ensureInitialized(); err != nil {
		return err
	}
	if id == uuid.Nil {
		return fmt.Errorf("sdk: message_id is required")
	}
	path := "/messages/" + id.String() + "/read"
	return c.client.sendAndDecode(ctx, http.MethodPost, path, nil, nil)
}
