package main

import (
	"bytes"
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

type Subscription struct {
	fp      *gofeed.Parser
	config  FeedConfig
	Updates []gofeed.Item
	LastRun int64
}

type Config struct {
	file string

	WebhookUrl string `json:"WebhookUrl"`
	Token      string `json:"Token,omitempty"`
	Channel    string `json:"Channel"`
	IconURL    string `json:"IconURL,omitempty"`
	Username   string `json:"Username"`

	// Application-Updated Configuration
	LastRun int64        `json:"LastTime"`
	Feeds   []FeedConfig `json:"Feeds"`
}

type FeedConfig struct {
	Name     string `json:"Name,omitempty"`
	Url      string `json:"Url"`
	IconUrl  string `json:"IconUrl,omitempty"`
	Username string `json:"Username,omitempty"`
	Channel  string `json:"Channel,omitempty"`
}

type FeedItem struct {
	gofeed.Item
	FeedConfig
}

type MattermostMessage struct {
	Channel  string `json:"channel"`
	Username string `json:"username"`
	Icon     string `json:"icon_url"`
	Message  string `json:"text"`
}

func main() {
	cPath := flag.String("config", "./config.json", "Path to the config file.")
	httpBind := flag.String("bind", "127.0.0.1:9090", "HTTP Binding")

	flag.Parse()

	cfg := LoadConfig(*cPath)

	// Set up command server
	http.HandleFunc("/feeds", feedCommandHandler(cfg))
	go http.ListenAndServe(*httpBind, nil)
	fmt.Printf("Listening for commands on http://%s/feeds\n", *httpBind)

	//get all of our feeds and process them initially
	subscriptions := make([]Subscription, 0)
	for _, feed := range cfg.Feeds {
		subscriptions = append(subscriptions, NewSubscription(feed, cfg.LastRun))
	}

	feedItems := make(chan FeedItem, 200)
	updateTimer := time.Tick(5 * time.Minute)

	// Run once at start
	run(subscriptions, feedItems)
	cfg.LastRun = time.Now().Unix()
	cfg.Save()

	for {
		select {
		case t := <-updateTimer:
			run(subscriptions, feedItems)

			cfg.LastRun = t.Unix()
			cfg.Save()
		case item := <-feedItems:
			toMattermost(cfg, feedItemToMessage(item))
		}
	}
}

func run(subscriptions []Subscription, ch chan<- FeedItem) {

	for _, subscription := range subscriptions {
		updates := subscription.getUpdates()
		for _, update := range updates {
			ch <- NewFeedItem(subscription, update)
		}
	}
}

func feedCommandHandler(cfg *Config) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		token := r.PostFormValue("token")

		if token != cfg.Token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		text := r.PostFormValue("text")
		tokens := strings.Split(text, " ")
		action := tokens[0]
		switch action {
		case "add":
			if len(tokens) < 3 {
				toMattermost(cfg, MattermostMessage{Message: "Usage: add <name> <url> [...]"})
				w.WriteHeader(http.StatusNotAcceptable)
				return
			}

			username := r.PostFormValue("user_name")
			channel := r.PostFormValue("channel_name")
			name := tokens[1]
			url := tokens[2]
			iconUrl := ""

			if len(tokens) >= 4 {
				iconUrl = tokens[3]
			}

			cfg.Feeds = append(cfg.Feeds, FeedConfig{Name: name, Url: url, IconUrl: iconUrl, Channel: channel})
			fmt.Println("User", username, "in channel", channel, "added feed:", name, url)
			cfg.Save()
		case "remove":
			newlist := make([]FeedConfig, len(cfg.Feeds)-1)
			for _, f := range cfg.Feeds {
				if f.Name != tokens[1] {
					newlist = append(newlist, f)
				}
			}
			cfg.Save()
		case "list":
			feeds := make([]string, len(cfg.Feeds))
			for _, f := range cfg.Feeds {
				feeds = append(feeds, f.Url)
			}
			toMattermost(cfg, MattermostMessage{Message: strings.Join(feeds, ",")})
		default:
			toMattermost(cfg, MattermostMessage{Message: "Unknown command"})
		}

	}
}

func NewFeedItem(sub Subscription, item gofeed.Item) FeedItem {
	return FeedItem{item, sub.config}
}

func feedItemToMessage(item FeedItem) MattermostMessage {
	var message string

	if item.Image != nil {
		message = fmt.Sprintf("[%s](%s)\n%s", item.Title, item.Link, item.Image.URL)
	} else {
		message = fmt.Sprintf("[%s](%s)", item.Title, item.Link)
	}

	return MattermostMessage{item.Channel, item.Username, item.IconUrl, message}
}

//send a message to mattermost
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
	response, err := http.Post(config.WebhookUrl, "application/json;charset=utf-8", buff)
	if err != nil {
		fmt.Println("Error Posting message to Mattermost: ", err)
		return
	}
	defer response.Body.Close()
}

//Returns the config from json
func LoadConfig(file string) *Config {
	raw, err := ioutil.ReadFile(file)
	if err != nil {
		fmt.Println("Error reading config file: ", err)
		os.Exit(1)
	}
	var config Config
	config.file = file
	json.Unmarshal(raw, &config)

	if config.LastRun == 0 {
		config.LastRun = time.Now().Unix() - 300*60*1000
	}

	fmt.Println("Loaded configuration.")
	return &config
}

func (c *Config) Save() {
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		fmt.Println("Error serializing configuration", err)
		return
	}

	// XXX: Fail, atomic move
	err = ioutil.WriteFile(c.file, raw, 0640)
	if err != nil {
		fmt.Println("Error writing config file: ", err)
	}

	fmt.Println("Saved configuration.")
}

func NewSubscription(config FeedConfig, LastRun int64) Subscription {
	fp := gofeed.NewParser()
	return Subscription{fp, config, make([]gofeed.Item, 0), LastRun}
}

//fetch feed updates for specified subscription
func (s Subscription) getUpdates() []gofeed.Item {

	fmt.Println("Get updates from ", s.config.Url)

	updates := make([]gofeed.Item, 0)

	feed, err := s.fp.ParseURL(s.config.Url)
	if err != nil {
		fmt.Println(err)
		return updates
	}

	for _, i := range feed.Items {
		if i.PublishedParsed != nil && i.PublishedParsed.Unix() > s.LastRun {
			updates = append(updates, *i)
		}
	}
	s.LastRun = time.Now().Unix()

	fmt.Println("Got ", len(updates), " updates from ", s.config.Url)

	return updates
}
