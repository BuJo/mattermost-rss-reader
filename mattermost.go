package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"

	"github.com/apex/log"
)

const oneMegabyte = 1 << (10 * 2)

// The FeedItem hold information for a single feed update.
type FeedItem struct {
	feedUpdate
	FeedConfig
}

// The MattermostMessage for talking to the Mattermost API.
type MattermostMessage struct {
	Channel     string                 `json:"channel,omitempty"`
	Username    string                 `json:"username,omitempty"`
	Icon        string                 `json:"icon_url,omitempty"`
	Message     string                 `json:"text,omitempty"`
	Attachments []MattermostAttachment `json:"attachments,omitempty"`
}

// The MattermostAttachment enables posting richer content to the Mattermost API.
type MattermostAttachment struct {
	Fallback   string `json:"fallback"`
	Color      string `json:"color,omitempty"`
	Title      string `json:"title,omitempty"`
	TitleLink  string `json:"title_link,omitempty"`
	Text       string `json:"text,omitempty"`
	AuthorName string `json:"author_name,omitempty"`
	ThumbURL   string `json:"thumb_url,omitempty"`
}

// NewFeedItem encapsulates a feed item to be published to Mattermost.
func NewFeedItem(sub *Subscription, item feedUpdate) FeedItem {
	return FeedItem{item, sub.config}
}

// itemToSimpleMessage formats a feed to be able to present it in Mattermost.
func itemToSimpleMessage(config *Config, item FeedItem) MattermostMessage {
	var message string

	if item.Image != nil {
		message = fmt.Sprintf("[%s](%s)\n%s", item.Title, item.Link, item.Image.URL)
	} else {
		message = fmt.Sprintf("[%s](%s)", item.Title, item.Link)
	}

	message = config.sanitizer.Sanitize(message)

	return MattermostMessage{Channel: item.Channel, Username: item.Username, Icon: item.IconURL, Message: message}
}

// itemToDetailedMessage formats a feed to be able to present it in Mattermost.
func itemToDetailedMessage(config *Config, item FeedItem) MattermostMessage {
	attachment := MattermostAttachment{
		Fallback:  config.sanitizer.Sanitize(item.Title),
		Title:     config.sanitizer.Sanitize(item.Title),
		TitleLink: item.Link,
		Text:      config.sanitizer.Sanitize(item.Description),
	}

	if item.Description != "" {
		attachment.Text = config.sanitizer.Sanitize(item.Description)
	} else if item.Content != "" {
		attachment.Text = config.sanitizer.Sanitize(item.Content)
	}

	if item.Author != nil {
		attachment.AuthorName = item.Author.Name
	}

	if item.Image != nil {
		attachment.ThumbURL = item.Image.URL
	}

	return MattermostMessage{Channel: item.Channel, Username: item.Username, Icon: item.IconURL, Attachments: []MattermostAttachment{attachment}}
}

// toMattermost sends a message to mattermost.
func toMattermost(config *Config, item FeedItem) (err error) {

	var msg MattermostMessage

	if item.Detailed || config.Detailed {
		msg = itemToDetailedMessage(config, item)
	} else {
		msg = itemToSimpleMessage(config, item)
	}

	if msg.Channel == "" {
		msg.Channel = config.Channel
	}
	if msg.Username == "" {
		msg.Username = config.Username
	}
	if msg.Icon == "" {
		msg.Icon = config.IconURL
	}

	ctx := config.ctx.WithField("channel", msg.Channel).WithField("user", msg.Username)
	defer ctx.Trace("Posting to Mattermost").Stop(&err)

	buff := new(bytes.Buffer)
	json.NewEncoder(buff).Encode(msg)
	var response *http.Response
	response, err = http.Post(config.WebhookURL, "application/json;charset=utf-8", buff)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode == 200 {
		// success
		return
	}

	data, err := ioutil.ReadAll(io.LimitReader(response.Body, oneMegabyte))
	if err != nil {
		ctx.WithError(err).Error("Failed reading Mattermost error message")
		return
	}
	ctx.Warnf("Mattermost response: %s", string(data))
	return nil
}

// feedCommandHandler handles HTTP requests according to the slash command
// documentation from mattermost.
// See https://docs.mattermost.com/developer/slash-commands.html fore more
// documentation.
func feedCommandHandler(cfg *Config) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		var err error
		token := r.PostFormValue("token")

		if token != cfg.Token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		username := r.PostFormValue("user_name")
		channel := r.PostFormValue("channel_name")

		ctx := cfg.ctx.WithFields(log.Fields{
			"user":    username,
			"channel": channel,
		})

		w.Header().Set("Content-Type", "application/json")

		text := r.PostFormValue("text")

		whitespace := regexp.MustCompile(`\s+`)
		tokens := whitespace.Split(text, -1)

		action := tokens[0]
		switch action {
		case "add":
			if len(tokens) < 3 {
				j, _ := json.Marshal(MattermostMessage{Message: "Usage: add <name> <url> [iconURL] [options]*"})
				w.Write(j)
				return
			}

			name := tokens[1]
			url := tokens[2]
			iconURL := ""
			detailed := false
			displayname := ""

			if len(tokens) >= 4 {
				iconURL = tokens[3]
			}

			for _, f := range cfg.Feeds {
				if f.Name == name {
					ctx.WithField("feed", name).Info("Feed already exists")

					j, _ := json.Marshal(MattermostMessage{Message: "Feed already exists, delete it first."})
					w.Write(j)
					return
				}
			}

			for _, option := range tokens[4:] {
				if o := strings.SplitN(option, "=", 2); len(o) == 2 {
					opt, val := o[0], o[1]
					switch opt {
					case "icon":
						iconURL = val
					case "channel":
						channel = val
					case "detailed", "detail":
						if val == "t" || val == "true" || val == "yes" || val == "detailed" {
							detailed = true
						} else {
							detailed = false
						}
					case "user", "username":
						displayname = val
					}
				}
			}

			cfg.Feeds = append(cfg.Feeds, FeedConfig{
				Name:     name,
				URL:      url,
				IconURL:  iconURL,
				Channel:  channel,
				Detailed: detailed,
				Username: displayname,
			})

			defer ctx.WithFields(log.Fields{
				"feed": name,
				"url":  url,
			}).Trace("Fedd added").Stop(&err)

			cfg.SaveFeeds()

			j, _ := json.Marshal(MattermostMessage{Message: "Added feed."})
			w.Write(j)
		case "remove":
			name := tokens[1]
			newlist := make([]FeedConfig, 0, len(cfg.Feeds)-1)
			for _, f := range cfg.Feeds {
				if f.Name != name {
					newlist = append(newlist, f)
				}
			}
			cfg.Feeds = newlist
			cfg.SaveFeeds()

			defer ctx.WithField("feed", name).Trace("Feed deleted").Stop(&err)

			j, _ := json.Marshal(MattermostMessage{Message: "Removed feed."})
			w.Write(j)
		case "list":
			str := ""
			for _, f := range cfg.Feeds {
				str += "* [" + f.Channel + "] " + f.Name + " (" + f.URL + ")\n"
			}

			defer ctx.Trace("Feed listing").Stop(&err)

			j, _ := json.Marshal(MattermostMessage{Message: str})
			w.Write(j)
		default:
			defer ctx.Trace("Unknown command").Stop(&err)

			j, _ := json.Marshal(MattermostMessage{Message: "Unknown command"})
			w.Write(j)
		}

	}
}
