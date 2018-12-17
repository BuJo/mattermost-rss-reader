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
	config  FeedConfig
	Updates []gofeed.Item
	LastRun int64
}

type Config struct {
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

type MattermostMessage struct {
	Channel  string `json:"channel"`
	Username string `json:"username"`
	Icon     string `json:"icon_url"`
	Message  string `json:"text"`
}

func main() {
	LastRun := time.Now().Unix() - 300*60*1000
	cfg := LoadConfig()

	//get all of our feeds and process them initially
	subscriptions := make([]Subscription, 0)
	for _, feed := range cfg.Feeds {
		s := Subscription{feed, make([]gofeed.Item, 0), LastRun}
		subscriptions = append(subscriptions, s)
	}

	ch := make(chan gofeed.Item)

	go run(subscriptions, ch)

	for item := range ch {
		NewFeedItem(cfg, item)
	}
}

func run(subscriptions []Subscription, ch chan<- gofeed.Item) {
	for {
		for _, subscription := range subscriptions {
			fmt.Println("Get updates for ", subscription.config.Name)
			updates := subscription.getUpdates()
			for _, update := range updates {
				ch <- update
			}
		}

		//sleep 5 minutes
		time.Sleep(60 * time.Second)
	}
}

func NewFeedItem(config *Config, item gofeed.Item) {
	if item.Image != nil {
		toMattermost(config, fmt.Sprintf("[%s](%s)\n%s", item.Title, item.Link, item.Image.URL))
	} else {
		toMattermost(config, fmt.Sprintf("[%s](%s)", item.Title, item.Link))
	}
}

//send a message to mattermost
func toMattermost(config *Config, message string) bool {
	fmt.Println("To Mattermost: ", message)
	msg := MattermostMessage{config.Channel, config.Username, config.IconURL, message}
	buff := new(bytes.Buffer)
	json.NewEncoder(buff).Encode(msg)
	response, err := http.Post(config.WebhookUrl, "application/json;charset=utf-8", buff)
	if err != nil {
		fmt.Println("Error Posting message to Mattermost: ", err.Error())
		return false
	}
	defer response.Body.Close()
	return true
}

//Returns the config from json
func LoadConfig() *Config {
	cPath := flag.String("config", "./config.json", "Path to the config file.")
	flag.Parse()

	raw, err := ioutil.ReadFile(*cPath)
	if err != nil {
		fmt.Println("Error reading config file: ", err.Error())
		os.Exit(1)
	}
	var config Config
	json.Unmarshal(raw, &config)
	return &config
}

//fetch feed updates for specified subscription
func (s *Subscription) getUpdates() []gofeed.Item {
	fp := gofeed.NewParser()
	feed, _ := fp.ParseURL(s.config.Url)
	updates := make([]gofeed.Item, 0)

	for i := 0; i < len(feed.Items); i++ {
		if feed.Items[i].PublishedParsed != nil && feed.Items[i].PublishedParsed.Unix() > s.LastRun {
			updates = append(updates, *feed.Items[i])
		}
	}
	s.LastRun = time.Now().Unix()

	return updates
}
