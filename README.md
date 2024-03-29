[![Build Status](https://travis-ci.org/mjhuber/mattermost-rss-reader.svg?branch=master)](https://travis-ci.org/mjhuber/mattermost-rss-reader)
# Mattermost RSS Feed Streamer

This utility will parse Atom/RSS feeds and post updates to a Mattermost channel.

## Setup

1.  Configure Mattermost
    - Go to the System Console.  Under Integrations=>Custom Integrations, set the following:
      - Enable Incoming Webhooks: True
      - Enable Integrations to override usernames: True
      - Enable Integrations to override profile picture icons: True
2.  Add incoming webhook in Mattermost
    - Go to your team's Integrations page and click "Incoming Webhooks".
    - Add an incoming Webhook:
      - Display Name: Xkcd
      - Description: Xkcd
      - Channel: Name of the channel you want to post into
    - Copy the webhook URL generated into the `WebhookURL` property in `config.json`.

If you want to allow Mattermost users to configure the feeds, also configure a Slash command:

1.  Deploy the mattermost-rss-reader (preferably behind a tls proxy)
2.  Add Slash Command in Mattermost
    - Go to your team's Integrations page and click "Slash Commands".
    - Add Slash Command
      - Title: Xkcd
      - Command Trigger Word: feed
      - Request URL: https:// URL to mattermost-rss-reader
      - Request Method: POST
      - Autocomplete: true
      - Autocomplete Hint: `list | add <Feed Name> <Feed URL> [<Image URL>] | remove <Feed name>`
    - Copy the Token from the resulting Slash Command into the `Token` property in `config.json`

Using e.g. `/feed add Xkcd https://xkcd.com/rss.xml` in a suitable channel will then post Xkcd
updates to this channel.

## Config Requirements

Configuration is loaded from the included config.json.  Supply the following variables:

1.  `WebhookURL`: URL to post the messages to Mattermost
3.  `IconURL`: URL to an image to use for the icon for each post (optional, can be overridden in feed).
4.  `Username`: Username the post will be displayed as (optional, can be overridden in feed).
5.  `Token`: Token for allowing slash commands to affect the configured feeds from Mattermost (optional).
6.  `SkipInitial`: Allows the first articles to be discarded on application start (optional, `false` by default)
7.  `Interval`: At which interval the feeds are polled (optional, `5m` by default).
8.  `FeedFile`: Configure a separate file for feeds.  Configure this when using Slash Commands to edit the feed list.
9.  `Feeds`: Collection of RSS URLs to poll.
    - `Name`: Used for displaying and identifying the feed
    - `URL`: which URL to pool.
    - `IconURL`: optional icon URL
    - `Username`: optional username
    - `Channel`: optional channel name
    - `Detailed`: optional rich text output

## Development

### Releasing

* Add [CHANGELOG](./CHANGELOG.md) entry
* Add a git tag
