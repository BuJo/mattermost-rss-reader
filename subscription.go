package main

import (
	"github.com/apex/log"
	"github.com/mmcdole/gofeed"
)

// A Subscription holds the configuration to fetch updates from a single URL.
type Subscription struct {
	parser  *gofeed.Parser
	config  FeedConfig
	updates []gofeed.Item
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
