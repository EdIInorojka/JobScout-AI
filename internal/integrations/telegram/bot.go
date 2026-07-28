package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

type Update struct {
	UpdateID      int            `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

type Message struct {
	MessageID int    `json:"message_id"`
	From      *User  `json:"from,omitempty"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text,omitempty"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data,omitempty"`
}

type Chat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Username string `json:"username,omitempty"`
}

type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username,omitempty"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

type apiResponse[T any] struct {
	OK          bool            `json:"ok"`
	Result      T               `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

func NewClient(token string, timeout time.Duration) *Client {
	return &Client{
		token:      token,
		baseURL:    "https://api.telegram.org/bot" + token,
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *Client) GetUpdates(ctx context.Context, offset, timeoutSec int) ([]Update, error) {
	values := url.Values{}
	if offset > 0 {
		values.Set("offset", strconv.Itoa(offset))
	}
	if timeoutSec > 0 {
		values.Set("timeout", strconv.Itoa(timeoutSec))
	}
	var resp apiResponse[[]Update]
	if err := c.postForm(ctx, "/getUpdates", values, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("telegram getUpdates failed: %s", resp.Description)
	}
	return resp.Result, nil
}

func (c *Client) SendMessage(ctx context.Context, chatID int64, text string, markup *InlineKeyboardMarkup) error {
	values := url.Values{}
	values.Set("chat_id", strconv.FormatInt(chatID, 10))
	values.Set("text", text)
	values.Set("parse_mode", "HTML")
	if markup != nil {
		raw, err := json.Marshal(markup)
		if err != nil {
			return err
		}
		values.Set("reply_markup", string(raw))
	}
	var resp apiResponse[Message]
	if err := c.postForm(ctx, "/sendMessage", values, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("telegram sendMessage failed: %s", resp.Description)
	}
	return nil
}

func (c *Client) EditMessageText(ctx context.Context, chatID int64, messageID int, text string, markup *InlineKeyboardMarkup) error {
	values := url.Values{}
	values.Set("chat_id", strconv.FormatInt(chatID, 10))
	values.Set("message_id", strconv.Itoa(messageID))
	values.Set("text", text)
	values.Set("parse_mode", "HTML")
	if markup != nil {
		raw, err := json.Marshal(markup)
		if err != nil {
			return err
		}
		values.Set("reply_markup", string(raw))
	}
	var resp apiResponse[json.RawMessage]
	if err := c.postForm(ctx, "/editMessageText", values, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("telegram editMessageText failed: %s", resp.Description)
	}
	return nil
}

func (c *Client) AnswerCallbackQuery(ctx context.Context, callbackID string, text string) error {
	values := url.Values{}
	values.Set("callback_query_id", callbackID)
	if text != "" {
		values.Set("text", text)
	}
	var resp apiResponse[bool]
	if err := c.postForm(ctx, "/answerCallbackQuery", values, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("telegram answerCallbackQuery failed: %s", resp.Description)
	}
	return nil
}

func (c *Client) postForm(ctx context.Context, method string, values url.Values, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+method, strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram api http %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return json.Unmarshal(body, dest)
}

func (c *Client) GetMe(ctx context.Context) (User, error) {
	var resp apiResponse[User]
	if err := c.getJSON(ctx, "/getMe", nil, &resp); err != nil {
		return User{}, err
	}
	if !resp.OK {
		return User{}, fmt.Errorf("telegram getMe failed: %s", resp.Description)
	}
	return resp.Result, nil
}

func (c *Client) getJSON(ctx context.Context, method string, query url.Values, dest any) error {
	fullURL := c.baseURL + method
	if query != nil {
		fullURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram api http %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return json.Unmarshal(body, dest)
}

func (m InlineKeyboardMarkup) Empty() bool {
	return len(m.InlineKeyboard) == 0
}

func NewInlineKeyboard(rows ...[]InlineKeyboardButton) *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{InlineKeyboard: rows}
}

func Button(label, callbackData string) InlineKeyboardButton {
	return InlineKeyboardButton{Text: label, CallbackData: callbackData}
}

func URLButton(label, target string) InlineKeyboardButton {
	return InlineKeyboardButton{Text: label, URL: target}
}

func (m InlineKeyboardMarkup) MarshalJSON() ([]byte, error) {
	type alias InlineKeyboardMarkup
	return json.Marshal(alias(m))
}

func (m *InlineKeyboardMarkup) String() string {
	raw, _ := json.Marshal(m)
	return string(bytes.TrimSpace(raw))
}
