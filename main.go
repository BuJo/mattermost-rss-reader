package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/microcosm-cc/bluemonday"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// The Config holds the configuration and state for this application.
type Config struct {
	file       string
	initialRun bool
	interval   time.Duration
	sanitizer  *bluemonday.Policy
	log        *slog.Logger

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

var cPath = flag.String("config", "./config.json", "Path to the config file.")
var httpBind = flag.String("bind", "127.0.0.1:9090", "HTTP Binding")
var environment = flag.String("environment", "dev", "Runtime environment")
var printVersion = flag.Bool("version", false, "Show Version")
var systemd = flag.Bool("systemd", false, "systemd integration")

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
	if *httpBind != "" {
		go func() {
			http.HandleFunc("/feeds", feedCommandHandler(cfg))
			http.Handle("/actuator/metrics", promhttp.Handler())
			http.HandleFunc("/actuator/health", healthHandler(cfg))

			cfg.log.Info("Listening for commands\n", "url", "http://"+*httpBind+"/feeds")

			l, err := net.Listen("tcp", *httpBind)
			if err != nil {
				cfg.log.Error("Error starting server", "err", err)
			}
			if *systemd {
				daemon.SdNotify(false, daemon.SdNotifyReady)
			}
			http.Serve(l, nil)
		}()
	}

	//get all of our feeds and process them initially
	subscriptions := make([]*Subscription, 0)
	for _, feed := range cfg.Feeds {
		subscriptions = append(subscriptions, NewSubscription(feed))
	}

	feedItems := make(chan FeedItem, 200)
	updateTimer := time.Tick(cfg.interval)

	// Run once at start
	cfg.log.Info("Ready to fetch feeds", slog.String("interval", cfg.interval.String()))
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
func run(cfg *Config, subscriptions []*Subscription, ch chan<- FeedItem) {

	initialRun := cfg.initialRun
	cfg.initialRun = false

	for _, subscription := range subscriptions {
		log := cfg.log.With(slog.String("feed", subscription.config.Name))

		updates, _ := subscription.getUpdates(log)
		nr := 0

		log = log.With("count", len(updates))

		for _, update := range updates {
			nr++
			shown := subscription.Shown(update)
			subscription.SetShown(update)

			log = log.With("title", update.Title).With("nr", nr)

			if initialRun && cfg.SkipInitial {
				log.Debug("Skipping initial run")

				continue
			} else if initialRun && nr > cfg.ShowInitial {
				log.Debug("Skipping initial run")

				continue
			} else if shown {
				log.Debug("Skipping already published")
				continue
			}

			ch <- NewFeedItem(subscription, update)
		}
	}
}

// LoadConfig returns the config from json.
func LoadConfig() *Config {
	var config Config

	config.log = slog.With(
		slog.String("application", path.Base(os.Args[0])),
		slog.String("environment", *environment),
		slog.String("version", Version),
	)

	raw, err := os.ReadFile(*cPath)
	if err != nil {
		config.log.With("err", err).Error("Error reading config file")
		os.Exit(1)
	}

	config.file = *cPath
	if err = json.Unmarshal(raw, &config); err != nil {
		config.log.Error("Error reading config file", "err", err)
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

	config.log.Debug("Configuration loaded")

	config.initialRun = true

	return &config
}

// LoadFeeds will load feeds from a separate feed file.
func (c *Config) LoadFeeds() error {
	raw, err := os.ReadFile(c.FeedFile)
	if err != nil {
		c.log.Error("Error reading feed file", "err", err)
		return err
	}

	if err = json.Unmarshal(raw, &c.Feeds); err != nil {
		c.log.Error("Error reading feed file", "err", err)
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

	return nil
}

// SaveFeeds will save the current list of feeds.
func (c *Config) SaveFeeds() {
	if c.FeedFile == "" {
		c.log.Warn("Not saving feeds, configure `FeedFile`.")
		return
	}

	raw, err := json.MarshalIndent(c.Feeds, "", "  ")
	if err != nil {
		c.log.Error("Error serializing feeds", "err", err)
		return
	}

	tmpfile, err := os.CreateTemp("", "mamo-rss-reader")
	if err != nil {
		c.log.Error("Error opening temporary file for feeds", "err", err)
		return
	}

	if _, err = tmpfile.Write(raw); err != nil {
		c.log.Error("Error writing config file", "err", err)
		return
	}
	if err = tmpfile.Close(); err != nil {
		c.log.Error("Error writing config file", "err", err)
		return
	}

	if err = os.Rename(tmpfile.Name(), c.FeedFile); err != nil {
		c.log.Error("Error writing config file", "err", err)
		return
	}

	c.log.Info("Saved feeds.")
}
