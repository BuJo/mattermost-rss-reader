package main

import "testing"

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
