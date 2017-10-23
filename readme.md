# Mattermost RSS Feed Streamer
This utility will parse RSS feeds and post updates to a Mattermost channel.

## Config Requirements
Configuration is loaded from the included config.json.  Supply the following variables:

1.  WebhookUrl - url to post the messages to Mattermost
2.  Channel - Name of the channel to post to.
3.  IconUrl - URL to an image to use for the icon for each post.
4.  Feeds - Collection of RSS URLs to poll.

## Docker
Run it as a docker container!!
```
docker build -t "name_of_image" .
docker run "name_of_image"
```
