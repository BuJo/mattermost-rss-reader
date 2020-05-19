package main

import (
	"github.com/apex/log/handlers/memory"
	"os"
	"testing"

	"github.com/apex/log"
	"github.com/microcosm-cc/bluemonday"
)

func XTestFefe(t *testing.T) {

	sub := NewSubscription(FeedConfig{
		Name: "Fefe",
		URL:  "https://blog.fefe.de/rss.xml?html",
	})
	updates, err := sub.getUpdates(log.WithField("feed", sub.config.Name))
	if err != nil {
		t.Error(err)
	}
	if len(updates) == 0 {
		t.Fail()
	}
}

func XTestGoogleAlert(t *testing.T) {
	config := &Config{
		sanitizer: bluemonday.StrictPolicy(),
	}

	sub := NewSubscription(FeedConfig{
		Name: "GoogleAlert",
		URL:  "https://www.google.de/alerts/feeds/06708116347342762808/6740125697618148595",
	})
	updates, err := sub.getUpdates(log.WithField("feed", sub.config.Name))
	if err != nil {
		t.Error(err)
	}
	if len(updates) == 0 {
		t.Fail()
		return
	}
	item := NewFeedItem(sub, updates[0])
	msg := itemToDetailedMessage(config, item)
	if msg.Attachments[0].Text == "" {
		t.Error("Message should not be empty")
	}
}

func TestContargoHomepage(t *testing.T) {
	sub := NewSubscription(FeedConfig{
		Name: "ContargoHomepage",
		URL:  "https://www.contargo.net/de/feed.xml?format=feed&type=rss",
	})
	updates, err := sub.getUpdates(log.WithField("feed", sub.config.Name))
	if err != nil {
		t.Error(err)
	}
	if len(updates) == 0 {
		t.Error("No updates")
		return
	}

	for i1, u1 := range updates {
		for i2, u2 := range updates {
			if i1 != i2 {
				if sub.Equal(u1, u2) {
					t.Error("Should not equal", u1.GUID, u2.GUID, u1.Link, u2.Link)
				}
			}
		}
	}
}

func TestMain(m *testing.M) {
	log.SetHandler(memory.New())
	os.Exit(m.Run())
}
