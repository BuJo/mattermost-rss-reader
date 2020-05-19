package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/BuJo/mattermost-rss-reader/journal"
	"github.com/apex/log"
	"github.com/apex/log/handlers/graylog"
	"github.com/apex/log/handlers/multi"
	"github.com/apex/log/handlers/text"
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

var cPath = flag.String("config", "./config.json", "Path to the config file.")
var httpBind = flag.String("bind", "127.0.0.1:9090", "HTTP Binding")
var environment = flag.String("environment", "dev", "Runtime environment")
var printVersion = flag.Bool("version", false, "Show Version")
var logLevel = flag.String("loglevel", "info", "Log level (debug, _info_, warn, error, fatal)")
var logGraylog = flag.String("graylog", "", "Optional Graylog host for logging")
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
	go func(ctx *log.Entry) {
		http.HandleFunc("/feeds", feedCommandHandler(cfg))
		http.Handle("/actuator/metrics", promhttp.Handler())
		http.HandleFunc("/actuator/health", healthHandler(cfg))

		ctx.Infof("Listening for commands on http://%s/feeds\n", *httpBind)

		l, err := net.Listen("tcp", *httpBind)
		if err != nil {
			ctx.WithError(err).Error("Error starting server")
		}
		if *systemd {
			daemon.SdNotify(false, "READY=1")
		}
		http.Serve(l, nil)
	}(cfg.ctx)

	//get all of our feeds and process them initially
	subscriptions := make([]*Subscription, 0)
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
func run(cfg *Config, subscriptions []*Subscription, ch chan<- FeedItem) {

	initialRun := cfg.initialRun
	cfg.initialRun = false

	for _, subscription := range subscriptions {
		ctx := cfg.ctx.WithField("feed", subscription.config.Name)

		updates, _ := subscription.getUpdates(ctx)
		nr := 1

		ctx = ctx.WithField("count", len(updates))

		for _, update := range updates {
			ctx = ctx.WithField("title", update.Title).WithField("nr", nr)

			if initialRun && cfg.SkipInitial {
				ctx.Debug("Skipping initial run")

				continue
			} else if initialRun && nr > cfg.ShowInitial {
				ctx.Debug("Skipping initial run")

				continue
			} else if subscription.Shown(update) {
				ctx.Debug("Skipping already published")
				continue
			}

			nr++
			subscription.SetShown(update)
			ch <- NewFeedItem(subscription, update)
		}
	}

}

// LoadConfig returns the config from json.
func LoadConfig() *Config {
	var config Config

	log.SetLevelFromString(*logLevel)

	loghandlers := make([]log.Handler, 1)

	if *logGraylog != "" {
		g, err := graylog.New(*logGraylog)
		if err != nil {
			log.WithError(err).Error("Failed to initialize Graylog logger")
		} else {
			loghandlers = append(loghandlers, g)
		}
	}

	if *systemd {
		loghandlers = append(loghandlers, journal.New())
	} else {
		loghandlers = append(loghandlers, text.New(os.Stderr))
	}
	log.SetHandler(multi.New(loghandlers...))

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

	config.initialRun = true

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
