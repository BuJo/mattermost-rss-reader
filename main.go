package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/apex/log"
	"github.com/apex/log/handlers/graylog"
	"github.com/apex/log/handlers/multi"
	"github.com/apex/log/handlers/text"
	"github.com/microcosm-cc/bluemonday"
	"github.com/mmcdole/gofeed"
)

// A Subscription holds the configuration to fetch updates from a single URL.
type Subscription struct {
	parser  *gofeed.Parser
	config  FeedConfig
	updates []gofeed.Item
}

// The Config holds the configuration and state for this application.
type Config struct {
	file       string
	shownFeeds map[[sha1.Size]byte]bool
	interval   time.Duration
	sanitizer  *bluemonday.Policy
	ctx        *log.Entry

	WebhookURL  string `json:"WebhookURL"`
	Token       string `json:"Token,omitempty"`
	Channel     string `json:"Channel"`
	IconURL     string `json:"IconURL,omitempty"`
	Username    string `json:"Username"`
	SkipInitial bool   `json:"SkipInitial"`
	ShowInitial int    `json:"ShowInitial,omitempty"`
	Interval    string `json:"Interval"`
	Detailed    bool   `json:"Detailed"`

	FeedFile string       `json:"FeedFile"`
	Feeds    []FeedConfig `json:"Feeds"`
}

// The FeedConfig holds information for a single Feed.
type FeedConfig struct {
	Name     string `json:"Name,omitempty"`
	URL      string `json:"URL"`
	IconURL  string `json:"IconUrl,omitempty"`
	Username string `json:"Username,omitempty"`
	Channel  string `json:"Channel,omitempty"`
	Detailed bool   `json:"Detailed"`
}

// The FeedItem hold information for a single feed update.
type FeedItem struct {
	gofeed.Item
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

var cPath = flag.String("config", "./config.json", "Path to the config file.")
var httpBind = flag.String("bind", "127.0.0.1:9090", "HTTP Binding")
var environment = flag.String("environment", "dev", "Runtime environment")
var printVersion = flag.Bool("version", false, "Show Version")
var logLevel = flag.String("loglevel", "info", "Log level (debug, _info_, warn, error, fatal)")
var logGraylog = flag.String("graylog", "", "Optional Graylog host for logging")

// Version of this application.
var Version = "development"

func main() {

	flag.Parse()

	if *printVersion {
		fmt.Println(path.Base(os.Args[0]), "version:", Version)
		return
	}

	cfg := LoadConfig()

	// Set up command server
	go func(ctx *log.Entry) {
		http.HandleFunc("/feeds", feedCommandHandler(cfg))
		ctx.Infof("Listening for commands on http://%s/feeds\n", *httpBind)
		err := http.ListenAndServe(*httpBind, nil)
		if err != nil {
			ctx.WithError(err).Error("Error starting server:")
		}
	}(cfg.ctx)

	//get all of our feeds and process them initially
	subscriptions := make([]Subscription, 0)
	for _, feed := range cfg.Feeds {
		subscriptions = append(subscriptions, NewSubscription(feed))
	}

	feedItems := make(chan FeedItem, 200)
	updateTimer := time.Tick(cfg.interval)

	// Run once at start
	cfg.ctx.WithField("interval", cfg.interval).Info("Ready to fetch feeds")
	run(cfg, subscriptions, feedItems)

	for {
		select {
		case <-updateTimer:
			run(cfg, subscriptions, feedItems)
		case item := <-feedItems:
			toMattermost(cfg, item)
		}
	}
}

// run fetches all new feed items from subscriptions.
func run(cfg *Config, subscriptions []Subscription, ch chan<- FeedItem) {

	initialRun := false
	if cfg.shownFeeds == nil {
		initialRun = true
	}
	shownFeeds := make(map[[sha1.Size]byte]bool, 0)

	for _, subscription := range subscriptions {
		ctx := cfg.ctx.WithField("feed", subscription.config.Name)

		updates, _ := subscription.getUpdates(ctx)
		nr := 1

		ctx = ctx.WithField("count", len(updates))

		for _, update := range updates {
			hsh := sha1.Sum(append([]byte(update.Title), []byte(subscription.config.URL)...))
			ctx = ctx.WithField("hsh", fmt.Sprintf("%x", hsh)).WithField("title", update.Title).WithField("nr", nr)

			shownFeeds[hsh] = true

			if initialRun && cfg.SkipInitial {
				ctx.Debug("Skipping initial run")

				continue
			} else if initialRun && nr > cfg.ShowInitial {
				ctx.Debug("Skipping initial run")

				continue
			} else if _, ok := cfg.shownFeeds[hsh]; ok {
				ctx.Debug("Skipping already published")
				continue
			}

			nr++
			ch <- NewFeedItem(subscription, update)
		}
	}

	cfg.shownFeeds = shownFeeds
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
				str += "* " + f.Name + " (" + f.URL + ")\n"
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

// NewFeedItem encapsulates a feed item to be published to Mattermost.
func NewFeedItem(sub Subscription, item gofeed.Item) FeedItem {
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

	return nil
}

// LoadConfig returns the config from json.
func LoadConfig() *Config {
	var config Config

	log.SetHandler(text.New(os.Stderr))
	log.SetLevelFromString(*logLevel)

	if *logGraylog != "" {
		g, err := graylog.New(*logGraylog)
		if err != nil {
			log.WithError(err).Error("Failed to initialize Graylog logger")
		}
		log.SetHandler(multi.New(text.New(os.Stderr), g))
	}

	config.ctx = log.WithFields(log.Fields{
		"application": path.Base(os.Args[0]),
		"environment": *environment,
		"version":     Version,
	})

	raw, err := ioutil.ReadFile(*cPath)
	if err != nil {
		config.ctx.WithError(err).Fatal("Error reading config file")
	}

	config.file = *cPath
	if err = json.Unmarshal(raw, &config); err != nil {
		config.ctx.WithError(err).Fatal("Error reading config file")
	}

	interval, err := time.ParseDuration(config.Interval)
	if err == nil && interval > 0 {
		config.interval = interval
	} else {
		config.interval = 5 * time.Minute
	}

	if config.FeedFile != "" {
		config.LoadFeeds()
	}

	config.sanitizer = bluemonday.StrictPolicy()

	config.ctx.Debug("Configuration loaded")

	return &config
}

// LoadFeeds will load feeds from a separate feed file.
func (c *Config) LoadFeeds() error {
	raw, err := ioutil.ReadFile(c.FeedFile)
	if err != nil {
		c.ctx.WithError(err).Error("Error reading feed file")
		return err
	}

	if err = json.Unmarshal(raw, &c.Feeds); err != nil {
		c.ctx.WithError(err).Fatal("Error reading feed file")
	}

	// Remove bad feeds
	newlist := make([]FeedConfig, 0, len(c.Feeds))
	for _, f := range c.Feeds {
		if f.URL != "" {
			newlist = append(newlist, f)
		}
	}
	c.Feeds = newlist

	return nil
}

// SaveFeeds will save the current list of feeds.
func (c *Config) SaveFeeds() {
	if c.FeedFile == "" {
		c.ctx.Warn("Not saving feeds, configure `FeedFile`.")
		return
	}

	raw, err := json.MarshalIndent(c.Feeds, "", "  ")
	if err != nil {
		c.ctx.WithError(err).Error("Error serializing feeds")
		return
	}

	tmpfile, err := ioutil.TempFile("", "mamo-rss-reader")
	if err != nil {
		c.ctx.WithError(err).Error("Error opening temporary file for feeds")
		return
	}

	if _, err = tmpfile.Write(raw); err != nil {
		c.ctx.WithError(err).Error("Error writing config file")
		return
	}
	if err = tmpfile.Close(); err != nil {
		c.ctx.WithError(err).Error("Error writing config file")
		return
	}

	if err = os.Rename(tmpfile.Name(), c.FeedFile); err != nil {
		c.ctx.WithError(err).Error("Error writing config file")
		return
	}

	c.ctx.Info("Saved feeds.")
}

// NewSubscription returns a new subscription for a given configuration.
func NewSubscription(config FeedConfig) Subscription {
	fp := gofeed.NewParser()
	return Subscription{fp, config, make([]gofeed.Item, 0)}
}

// getUpdates fetches feed updates for specified subscription
func (s Subscription) getUpdates(ctx *log.Entry) (updates []gofeed.Item, err error) {

	defer ctx.WithField("url", s.config.URL).Trace("Get updates").Stop(&err)

	updates = make([]gofeed.Item, 0)

	var feed *gofeed.Feed
	feed, err = s.parser.ParseURL(s.config.URL)
	if err != nil {
		return updates, err
	}

	for _, i := range feed.Items {
		updates = append(updates, *i)
	}

	return updates, nil
}
