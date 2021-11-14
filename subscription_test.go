package main

import (
	"github.com/apex/log/handlers/memory"
	"os"
	"testing"

	"github.com/apex/log"
	"github.com/microcosm-cc/bluemonday"
)

func TestFefe(t *testing.T) {

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

func TestGoogleAlert(t *testing.T) {
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
		// Too bad, but sometimes google alerts are indeed empty
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
func TestNTV(t *testing.T) {
	sub := NewSubscription(FeedConfig{
		Name: "NTV",
		URL:  "https://www.n-tv.de/rss",
	})
	updates, err := sub.getUpdates(log.WithField("feed", sub.config.Name))
	if err != nil {
		t.Error(err)
	}
	if len(updates) == 0 {
		t.Error("No updates")
		return
	}

	for _, u := range updates {
		sub.SetShown(u)
	}
	for _, u := range updates {
		if !sub.Shown(u) {
			t.Error("Should count as already shown")
		}
	}
}

func TestMain(m *testing.M) {
	log.SetHandler(memory.New())
	os.Exit(m.Run())
}
