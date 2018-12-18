package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
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
	Token      string `json:"Token"`
	Channel    string `json:"Channel"`
	IconURL    string `json:"IconURL"`
	Username   string `json:"Username"`

	Feeds []FeedConfig `json:"Feeds"`
}
type FeedConfig struct {
	Name     string
	Url      string
	IconUrl  string
	Username string
	Channel  string
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
	LastRun := time.Now().Unix() - 300*60*1000
	cPath := flag.String("config", "./config.json", "Path to the config file.")
	flag.Parse()

	cfg := LoadConfig(*cPath)

	//get all of our feeds and process them initially
	subscriptions := make([]Subscription, 0)
	for _, feed := range cfg.Feeds {
		subscriptions = append(subscriptions, NewSubscription(feed, LastRun))
	}

	feedItems := make(chan FeedItem, 200)
	updateTimer := time.Tick(5 * time.Minute)

	// Run once at start
	run(subscriptions, feedItems)

	for {
		select {
		case <-updateTimer:
			run(subscriptions, feedItems)
		case item := <-feedItems:
			toMattermost(cfg, item)
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

func NewFeedItem(sub Subscription, item gofeed.Item) FeedItem {
	return FeedItem{item, sub.config}
}

//send a message to mattermost
func toMattermost(config *Config, item FeedItem) {
	var message string

	if item.Image != nil {
		message = fmt.Sprintf("[%s](%s)\n%s", item.Title, item.Link, item.Image.URL)
	} else {
		message = fmt.Sprintf("[%s](%s)", item.Title, item.Link)
	}

	msg := MattermostMessage{item.Channel, item.Username, item.IconUrl, message}

	if msg.Channel == "" {
		msg.Channel = config.Channel
	}
	if msg.Username == "" {
		msg.Username = config.Username
	}
	if msg.Icon == "" {
		msg.Icon = config.IconURL
	}

	fmt.Printf("To Mattermost #%s as %s: %s\n", msg.Channel, msg.Username, message)

	buff := new(bytes.Buffer)
	json.NewEncoder(buff).Encode(msg)
	response, err := http.Post(config.WebhookUrl, "application/json;charset=utf-8", buff)
	if err != nil {
		fmt.Println("Error Posting message to Mattermost: ", err.Error())
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
	fmt.Println("Loaded configuration.")
	return &config
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
