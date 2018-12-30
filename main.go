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
	"strings"
	"time"

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

	WebhookURL  string `json:"WebhookURL"`
	Token       string `json:"Token,omitempty"`
	Channel     string `json:"Channel"`
	IconURL     string `json:"IconURL,omitempty"`
	Username    string `json:"Username"`
	SkipInitial bool   `json:"SkipInitial"`
	Interval    string `json:"Interval"`

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
}

// The FeedItem hold information for a single feed update.
type FeedItem struct {
	gofeed.Item
	FeedConfig
}

// The MattermostMessage for talking to the Mattermost API.
type MattermostMessage struct {
	Channel  string `json:"channel,omitempty"`
	Username string `json:"username,omitempty"`
	Icon     string `json:"icon_url,omitempty"`
	Message  string `json:"text"`
}

// Version of this application.
var Version = "development"

func main() {
	cPath := flag.String("config", "./config.json", "Path to the config file.")
	httpBind := flag.String("bind", "127.0.0.1:9090", "HTTP Binding")
	printVersion := flag.Bool("version", false, "Show Version")

	flag.Parse()

	if *printVersion {
		fmt.Println("mattermost-rss-reader, version:", Version)
		return
	}

	cfg := LoadConfig(*cPath)

	// Set up command server
	go func() {
		http.HandleFunc("/feeds", feedCommandHandler(cfg))
		fmt.Printf("Listening for commands on http://%s/feeds\n", *httpBind)
		err := http.ListenAndServe(*httpBind, nil)
		if err != nil {
			fmt.Println("Error starting server:", err)
		}
	}()

	//get all of our feeds and process them initially
	subscriptions := make([]Subscription, 0)
	for _, feed := range cfg.Feeds {
		subscriptions = append(subscriptions, NewSubscription(feed))
	}

	feedItems := make(chan FeedItem, 200)
	updateTimer := time.Tick(cfg.interval)

	// Run once at start
	fmt.Println("Ready to fetch feeds. Interval:", cfg.interval)
	run(cfg, subscriptions, feedItems)

	for {
		select {
		case <-updateTimer:
			run(cfg, subscriptions, feedItems)
		case item := <-feedItems:
			toMattermost(cfg, feedItemToMessage(item))
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
		updates := subscription.getUpdates()
		for _, update := range updates {
			hsh := sha1.Sum(append([]byte(update.Title), []byte(subscription.config.URL)...))

			shownFeeds[hsh] = true

			if initialRun && cfg.SkipInitial {
				fmt.Println("Skipping", update.Title, ", initial run")

				continue
			} else if _, ok := cfg.shownFeeds[hsh]; ok {
				fmt.Println("Skipping", update.Title, ", already published")
				continue
			}

			ch <- NewFeedItem(subscription, update)
		}
	}

	cfg.shownFeeds = shownFeeds
}

// feedCommandHandler handles HTTP requests according to the slash command
// documentation from mattermost.
// See https://docs.mattermost.com/developer/slash-commands.html fore more
// documentation.
//
// BUG(Jo): Multiple feeds with the same name can be added.
func feedCommandHandler(cfg *Config) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		token := r.PostFormValue("token")

		if token != cfg.Token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		text := r.PostFormValue("text")
		tokens := strings.Split(text, " ")
		action := tokens[0]
		switch action {
		case "add":
			if len(tokens) < 3 {
				j, _ := json.Marshal(MattermostMessage{Message: "Usage: add <name> <url> [iconURL]"})
				w.Write(j)
				w.WriteHeader(http.StatusNotAcceptable)
				return
			}

			username := r.PostFormValue("user_name")
			channel := r.PostFormValue("channel_name")
			name := tokens[1]
			url := tokens[2]
			iconURL := ""

			if len(tokens) >= 4 {
				iconURL = tokens[3]
			}

			cfg.Feeds = append(cfg.Feeds, FeedConfig{Name: name, URL: url, IconURL: iconURL, Channel: channel})
			fmt.Println("User", username, "in channel", channel, "added feed:", name, url)
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

			j, _ := json.Marshal(MattermostMessage{Message: "Removed feed."})
			w.Write(j)
		case "list":
			str := ""
			for _, f := range cfg.Feeds {
				str += "* " + f.Name + " (" + f.URL + ")\n"
			}
			j, _ := json.Marshal(MattermostMessage{Message: str})
			w.Write(j)
			w.WriteHeader(http.StatusOK)
		default:
			j, _ := json.Marshal(MattermostMessage{Message: "Unknown command"})
			w.Write(j)
			w.WriteHeader(http.StatusNotAcceptable)
		}

	}
}

// NewFeedItem encapsulates a feed item to be published to Mattermost.
func NewFeedItem(sub Subscription, item gofeed.Item) FeedItem {
	return FeedItem{item, sub.config}
}

// feedItemToMessage formats a feed to be able to present it in Mattermost.
func feedItemToMessage(item FeedItem) MattermostMessage {
	var message string

	if item.Image != nil {
		message = fmt.Sprintf("[%s](%s)\n%s", item.Title, item.Link, item.Image.URL)
	} else {
		message = fmt.Sprintf("[%s](%s)", item.Title, item.Link)
	}

	return MattermostMessage{item.Channel, item.Username, item.IconURL, message}
}

// toMattermost sends a message to mattermost.
func toMattermost(config *Config, msg MattermostMessage) {

	if msg.Channel == "" {
		msg.Channel = config.Channel
	}
	if msg.Username == "" {
		msg.Username = config.Username
	}
	if msg.Icon == "" {
		msg.Icon = config.IconURL
	}

	fmt.Printf("To Mattermost #%s as %s: %s\n", msg.Channel, msg.Username, msg.Message)

	buff := new(bytes.Buffer)
	json.NewEncoder(buff).Encode(msg)
	response, err := http.Post(config.WebhookURL, "application/json;charset=utf-8", buff)
	if err != nil {
		fmt.Println("Error Posting message to Mattermost:", err)
		return
	}
	defer response.Body.Close()
}

// LoadConfig returns the config from json.
func LoadConfig(file string) *Config {
	raw, err := ioutil.ReadFile(file)
	if err != nil {
		fmt.Println("Error reading config file:", err)
		os.Exit(1)
	}
	var config Config
	config.file = file
	if err = json.Unmarshal(raw, &config); err != nil {
		fmt.Println("Error reading feed file:", err)
		os.Exit(1)
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

	fmt.Println("Loaded configuration.")
	return &config
}

// LoadFeeds will load feeds from a separate feed file.
func (c *Config) LoadFeeds() {
	raw, err := ioutil.ReadFile(c.FeedFile)
	if err != nil {
		fmt.Println("Error reading feed file:", err)
		return
	}

	if err = json.Unmarshal(raw, &c.Feeds); err != nil {
		fmt.Println("Error reading feed file:", err)
		os.Exit(1)
	}

	// Remove bad feeds
	newlist := make([]FeedConfig, 0, len(c.Feeds))
	for _, f := range c.Feeds {
		if f.URL != "" {
			newlist = append(newlist, f)
		}
	}
	c.Feeds = newlist
}

// SaveFeeds will save the current list of feeds.
func (c *Config) SaveFeeds() {
	if c.FeedFile == "" {
		fmt.Println("Not saving feeds, configure `FeedFile`.")
	}

	raw, err := json.MarshalIndent(c.Feeds, "", "  ")
	if err != nil {
		fmt.Println("Error serializing feeds:", err)
		return
	}

	tmpfile, err := ioutil.TempFile("", "mamo-rss-reader")
	if err != nil {
		fmt.Println("Error opening tempfile for saving feeds", err)
		return
	}

	if _, err = tmpfile.Write(raw); err != nil {
		fmt.Println("Error writing config file:", err)
		return
	}
	if err = tmpfile.Close(); err != nil {
		fmt.Println("Error writing config file:", err)
		return
	}

	if err = os.Rename(tmpfile.Name(), c.FeedFile); err != nil {
		fmt.Println("Error writing config file:", err)
		return
	}

	fmt.Println("Saved feeds.")
}

// NewSubscription returns a new subscription for a given configuration.
func NewSubscription(config FeedConfig) Subscription {
	fp := gofeed.NewParser()
	return Subscription{fp, config, make([]gofeed.Item, 0)}
}

// getUpdates fetches feed updates for specified subscription
func (s Subscription) getUpdates() []gofeed.Item {

	fmt.Println("Get updates from", s.config.URL)

	updates := make([]gofeed.Item, 0)

	feed, err := s.parser.ParseURL(s.config.URL)
	if err != nil {
		fmt.Println(err)
		return updates
	}

	for _, i := range feed.Items {
		if i.PublishedParsed != nil {
			updates = append(updates, *i)
		}
	}

	fmt.Println("Got", len(updates), "updates from", s.config.URL)

	return updates
}
