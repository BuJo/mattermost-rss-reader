package main

import (
	"github.com/microcosm-cc/bluemonday"
	"testing"
)

func TestFefe(t *testing.T) {

	subscription := NewSubscription(FeedConfig{
		Name: "Fefe",
		URL:  "https://blog.fefe.de/rss.xml?html",
	})
	updates, err := subscription.getUpdates()
	if err != nil {
		t.Error(err)
	}
	if len(updates) == 0 {
		t.Fail()
	}
}

func TestGoogleAlert(t *testing.T) {
	config := &Config{
		sanitizer: bluemonday.StrictPolicy(),
	}

	sub := NewSubscription(FeedConfig{
		Name: "GoogleAlert",
		URL:  "https://www.google.de/alerts/feeds/06708116347342762808/6740125697618148595",
	})
	updates, err := sub.getUpdates()
	if err != nil {
		t.Error(err)
	}
	if len(updates) == 0 {
		t.Fail()
	}
	item := NewFeedItem(sub, updates[0])
	msg := itemToDetailedMessage(config, item)
	if msg.Attachments[0].Text == "" {
		t.Error("Message should not be empty")
	}
}
