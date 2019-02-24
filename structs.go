package bots

import (
	"errors"
	"strings"
)

// ErrIgnoredItem is returned when the story should be ignored.
var ErrIgnoredItem = errors.New("item ignored")

// SlackMessageResponse is a struct that maps to the response returned from slack.com/api/chat.postMessage
type SlackMessageResponse struct {
	OK          bool                       `json:"ok"`
	Channel     string                     `json:"channel"`
	Timestamp   string                     `json:"ts"`
	Attachments []*SlackMessageAttachments `json:"attachments"`
}

// SlackMessageAttachments is a struct that maps to the message attachment
type SlackMessageAttachments struct {
	Fallback   string                         `json:"fallback"`
	AuthorName string                         `json:"author_name"`
	AuthorIcon string                         `json:"author_icon"`
	Color      string                         `json:"color"`
	Title      string                         `json:"title"`
	TitleLink  string                         `json:"title_link"`
	Fields     []*SlackMessageAttachmentField `json:"fields"`
	ThumbURL   string                         `json:"thumb_url"`
	Text       string                         `json:"text"`
}

// SlackMessageAttachmentField is a struct that maps to the message attachment field
type SlackMessageAttachmentField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// InlineKeyboardMarkup type.
type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard,omitempty"`
}

// InlineKeyboardButton type.
type InlineKeyboardButton struct {
	Text string `json:"text,omitempty"`
	URL  string `json:"url,omitempty"`
}

// SendMessageResponse is the response from sendMessage request.
type SendMessageResponse struct {
	OK     bool   `json:"ok"`
	Result Result `json:"result"`
}

// Result is a submessage in SendMessageResponse. We only care the MessageID for now.
type Result struct {
	MessageID int64 `json:"message_id"`
}

// EditMessageTextRequest is the request to editMessageText method.
type EditMessageTextRequest struct {
	ChatID      string               `json:"chat_id"`
	MessageID   int64                `json:"message_id"`
	Text        string               `json:"text"`
	ParseMode   string               `json:"parse_mode,omitempty"`
	ReplyMarkup InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

// DeleteMessageRequest is the request to deleteMessage method.
type DeleteMessageRequest struct {
	ChatID    string `json:"chat_id"`
	MessageID int64  `json:"message_id"`
}

// DeleteMessageResponse is the response to deleteMessage method.
type DeleteMessageResponse struct {
	OK          bool   `json:"ok"`
	ErrorCode   int64  `json:"error_code"`
	Description string `json:"description"`
}

// ShouldIgnoreError return true if the message contains an error but should be ignored.
func (r *DeleteMessageResponse) ShouldIgnoreError() bool {
	return (r.ErrorCode == 400 &&
		// Someone manually deleted the message from the channel
		(strings.Contains(r.Description, "message to delete not found") ||
			// Story was on top 30 list for > 24 hours but Telegram API only allow
			// deleting messages that were posted in <48 hours.
			// It should be fine to just ignore this error, and leave these stories in
			// channel forever.
			strings.Contains(r.Description, "message can't be deleted")))
}
