package bots

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"strings"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
)

// Hot is the sign for a hot story, either because it has high score or it has
// large number of discussions.
const Hot = "🔥"

// Story is a struct represents an item stored in datastore.
// Part of the fields will be saved to datastore.
type Story struct {
	ID                  int64     `json:"id"`
	URL                 string    `json:"url"`
	Title               string    `json:"title"`
	Descendants         int64     `json:"descendants"`
	Score               int64     `json:"score"`
	Timestamp           string    `json:"ts"`
	LastSave            time.Time `json:"-"`
	Type                string    `json:"type"`
	missingFieldsLoaded bool
}

// NewFromDatastore create a Story from datastore.
func NewFromDatastore(ctx context.Context, id int64) (Story, error) {
	var story Story
	if err := datastore.Get(ctx, GetKey(ctx, id), &story); err != nil {
		return story, errors.WithStack(err)
	}
	return story, nil
}

// Load implements the PropertyLoadSaver interface.
func (s *Story) Load(ps []datastore.Property) error {
	return datastore.LoadStruct(s, ps)
}

// Save implements the PropertyLoadSaver interface.
func (s *Story) Save() ([]datastore.Property, error) {
	return []datastore.Property{
		{
			Name:  "Timestamp",
			Value: s.Timestamp,
		},
		{
			Name:  "ID",
			Value: s.ID,
		},
		{
			Name:  "LastSave",
			Value: time.Now(),
		},
	}, nil
}

// FillMissingFields is used to fill the missing story data from HN API.
func (s *Story) FillMissingFields(ctx context.Context) error {
	resp, err := myHTTPClient(ctx).Get(ItemURL(s.ID))
	if err != nil {
		return errors.WithStack(err)
	}
	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(s)
	if err != nil {
		return errors.WithStack(err)
	}
	s.missingFieldsLoaded = true
	return nil
}

// ShouldIgnore is a filter for story.
func (s *Story) ShouldIgnore() bool {
	return s.Type != "story" ||
		s.Score < ScoreThreshold ||
		s.Descendants < NumCommentsThreshold ||
		s.URL == ""
}

func (s *Story) ToSendMessageAttachments() []SlackMessageAttachments {
	var (
		scoreSuffix   string
		commentSuffix string
	)
	if s.Score > 100 {
		scoreSuffix = " " + Hot
	}
	if s.Descendants > 100 {
		commentSuffix = " " + Hot
	}
	return []SlackMessageAttachments{
		{
			Fallback:  s.Title,
			Color:     "#ff6633",
			Title:     s.Title,
			TitleLink: s.URL,
			Fields: []*SlackMessageAttachmentField{
				{
					Title: "Score",
					Value: fmt.Sprintf("%d+%s", s.Score, scoreSuffix),
					Short: true,
				},
				{
					Title: "Comments",
					Value: fmt.Sprintf("<%s|%d+%s>", NewsURL(s.ID), s.Descendants, commentSuffix),
					Short: true,
				},
			},
		},
	}
}

// GetReplyMarkup will return the markup for the story.
func (s *Story) GetReplyMarkup() InlineKeyboardMarkup {
	var scoreSuffix, commentSuffix string
	if s.Score > 100 {
		scoreSuffix = " " + Hot
	}
	if s.Descendants > 100 {
		commentSuffix = " " + Hot
	}
	return InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{
					Text: fmt.Sprintf("Score: %d+%s", s.Score, scoreSuffix),
					URL:  s.URL,
				},
				{
					Text: fmt.Sprintf("Comments: %d+%s", s.Descendants, commentSuffix),
					URL:  NewsURL(s.ID),
				},
			},
		},
	}
}

// EditMessage send a request to edit a message.
func (s *Story) EditMessage(ctx context.Context) error {
	if !s.missingFieldsLoaded {
		if err := s.FillMissingFields(ctx); err != nil {
			return errors.WithStack(err)
		}
	}
	if s.ShouldIgnore() {
		return errors.WithStack(ErrIgnoredItem)
	}

	attchments := s.ToSendMessageAttachments()
	jsonBytes, err := json.Marshal(attchments)
	if err != nil {
		return errors.WithStack(err)
	}

	resp, err := myHTTPClient(ctx).PostForm("https://slack.com/api/chat.update",
		url.Values{
			"token":        {SlackToken()},
			"channel":      {ChannelID()},
			"text":         {fmt.Sprintf("<%s>", strings.Replace(s.URL, "https", "http", 1))},
			"ts":           {s.Timestamp},
			"unfurl_links": {"true"},
			"attachments":  {string(jsonBytes)},
		},
	)
	if err != nil {
		log.Errorf(ctx, "story %d: %s could not be edit: %#v", s.ID, s.Title, err)
		return errors.WithStack(err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	log.Infof(ctx, "edited story %d: %s: %s", s.ID, s.Title, string(body))
	return nil
}

// InDatastore checks if the story is already in datastore.
func (s *Story) InDatastore(ctx context.Context) bool {
	log.Infof(ctx, "calling InDatastore")
	key := GetKey(ctx, s.ID)
	q := datastore.NewQuery("Story").Filter("__key__ =", key).KeysOnly()
	keys, _ := q.GetAll(ctx, nil)
	return len(keys) != 0
}

// SendMessage send a request to send a new message.
func (s *Story) SendMessage(ctx context.Context) error {
	if !s.missingFieldsLoaded {
		if err := s.FillMissingFields(ctx); err != nil {
			return errors.WithStack(err)
		}
	}

	if s.ShouldIgnore() {
		return ErrIgnoredItem
	} else if s.InDatastore(ctx) {
		return errors.WithStack(fmt.Errorf("story already posted: %#v", s))
	}
	attchments := s.ToSendMessageAttachments()
	jsonBytes, err := json.Marshal(attchments)
	if err != nil {
		return errors.WithStack(err)
	}

	resp, err := myHTTPClient(ctx).PostForm("https://slack.com/api/chat.postMessage",
		url.Values{
			"token":        {SlackToken()},
			"channel":      {ChannelID()},
			"attachments":  {string(jsonBytes)},
			"unfurl_links": {"true"},
			"text":         {fmt.Sprintf("<%s>", strings.Replace(s.URL, "https", "http", 1))}},
	)
	if err != nil {
		log.Errorf(ctx, "story %d: %s could not be sent: %#v", s.ID, s.Title, err)
		return errors.WithStack(err)
	}
	defer resp.Body.Close()

	var response SlackMessageResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return errors.WithStack(err)
	}
	s.Timestamp = response.Timestamp
	log.Infof(ctx, "sent story %d: %s", s.ID, s.Title)
	return nil
}

// DeleteMessage delete a message from datastore.
func (s *Story) DeleteMessage(ctx context.Context) error {
	key := GetKey(ctx, s.ID)
	if err := datastore.Delete(ctx, key); err != nil {
		return errors.WithStack(err)
	}
	log.Infof(ctx, "Story %d deleted", s.ID)
	return nil
}
