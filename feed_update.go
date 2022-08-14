package main

import "github.com/mmcdole/gofeed"

type feedUpdate struct {
	gofeed.Item
}

// Equal compares two feed items
func (u1 feedUpdate) Equal(u2 feedUpdate) bool {
	if u1.GUID != "" && u2.GUID != "" {
		// If GUIDs are available
		if u1.GUID == u2.GUID {
			// GUID same but link different is suspicious
			return u1.Link == u2.Link
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
