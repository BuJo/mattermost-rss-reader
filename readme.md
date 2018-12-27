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
    - Copy the webhook URL generated into the "WebhookUrl" property in config.json.


## Config Requirements

Configuration is loaded from the included config.json.  Supply the following variables:

1.  WebhookUrl - url to post the messages to Mattermost
3.  IconUrl - URL to an image to use for the icon for each post.
4.  Username - Username the post will be displayed as.
4.  Feeds - Collection of RSS URLs to poll.

## Docker

Run it as a docker container!!
```bash
docker build -t "name_of_image" .
docker run "name_of_image"
```
Run it with `docker-compose`:
```bash
docker-compose up
```

## Releasing

Add a git tag
```bash
git tag v0.7
```

### Build Docker Release

```bash
docker build -t name_of_image:0.7 --build-arg release=v0.7 .
```

### Build local release

```bash
make all release=v0.7
```
