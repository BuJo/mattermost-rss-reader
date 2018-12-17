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
	Name    string
	Link    string
	Url     string
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
		d := getUpdates(LastRun, feed.Url)
		subscriptions = append(subscriptions, *d)
		NewFeedItems(cfg, d.Updates)
	}
	run(subscriptions, cfg)
}

func run(subscriptions []Subscription, config *Config) {
	for {
		for _, subscription := range subscriptions {
			fmt.Println("Get updates for ", subscription.Name)
			subscription := getUpdates(subscription.LastRun, subscription.Url)
			for _, update := range subscription.Updates {
				fmt.Println("Processing feed update.")
				toMattermost(config, fmt.Sprintf("[%s](%s)", update.Title, update.Link))
			}
		}

		//sleep 5 minutes
		time.Sleep(60 * time.Second)
	}
}

func NewFeedItems(config *Config, items []gofeed.Item) {
	for _, item := range items {
		//toMattermost(config, fmt.Sprintf("[%s](%s)", item.Title, item.Link))
		if item.Image != nil {
			toMattermost(config, fmt.Sprintf("[%s](%s)\n%s", item.Title, item.Link, item.Image.URL))
		} else {
			toMattermost(config, fmt.Sprintf("[%s](%s)", item.Title, item.Link))
		}
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
func getUpdates(LastRun int64, url string) *Subscription {
	fp := gofeed.NewParser()
	feed, _ := fp.ParseURL(url)
	data := Subscription{feed.Title, feed.Link, url, make([]gofeed.Item, 0), time.Now().Unix()}

	for i := 0; i < len(feed.Items); i++ {
		if feed.Items[i].PublishedParsed != nil && feed.Items[i].PublishedParsed.Unix() > LastRun {
			data.Updates = append(data.Updates, *feed.Items[i])
		}
	}
	return &data
}
