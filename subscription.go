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

	shown []feedID
}

type feedID struct {
	GUID  string
	Title string
	Link  string
}

// NewSubscription returns a new subscription for a given configuration.
func NewSubscription(config FeedConfig) *Subscription {
	fp := gofeed.NewParser()
	return &Subscription{
		parser:  fp,
		config:  config,
		updates: make([]gofeed.Item, 0),
		shown:   make([]feedID, 0),
	}
}

// getUpdates fetches feed updates for specified subscription
func (s *Subscription) getUpdates(ctx *log.Entry) (updates []gofeed.Item, err error) {

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

// SetShown sets the given feed item to be already shown
func (s *Subscription) SetShown(item gofeed.Item) {
	s.shown = append([]feedID{{
		GUID:  item.GUID,
		Title: item.Title,
		Link:  item.Link,
	}}, s.shown...)
	if len(s.shown) > 200 {
		s.shown = s.shown[:190]
	}
}

// Shown returns true if the given feed item has already been shown
func (s *Subscription) Shown(item gofeed.Item) bool {
	for _, i := range s.shown {
		i := gofeed.Item{
			Title: i.Title,
			Link:  i.Link,
			GUID:  i.GUID,
		}
		if s.Equal(i, item) {
			return true
		}
	}
	return false
}

// Equal compares two feed items
func (s *Subscription) Equal(u1 gofeed.Item, u2 gofeed.Item) bool {
	if u1.GUID != "" && u2.GUID != "" {
		// If GUIDs are available
		if u1.GUID == u2.GUID {
			// Handle RSS Feeds
			if u1.Link != u2.Link {
				// Suspicious
				return false
			}

			return true
		}
		// Handle RSS Feeds regenerating GUIDs each call
		if u1.Link == u2.Link && u1.Title == u2.Title {
			// Suspicious, believe they are indeed the same
			return true
		}

		return false
	} else if u1.Link != "" && u2.Link != "" {
		// If Links are available
		if u1.Link == u2.Link && u1.Title == u2.Title {
			// Assume the Link+Title to be authorative
			return true
		}

		return false
	} else {
		return u1.Title == u2.Title
	}
}
