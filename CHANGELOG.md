# mattermost-rss-reader Changelog

## Release v1.4 - 2020-10-06

* Show channel name in list command

## Release v1.3 - 2020-05-20

* Bugfixes for RSS deduplication
  Handling akin to [RSS Duplicate Detection](http://www.xn--8ws00zhy3a.com/blog/2006/08/rss-dup-detection)

## Release v1.2 - 2020-04-29

* Add structured logging
* Add logging to graylog
* Add health and metrics endpoints

## Release v1.1 - 2020-04-29

* New config option `ShowInitial`, for showing only a number of initial messages
  if `SkipInitial` is set to `false`.
* Allow feeds without publish dates
* On detailed messages, use content if the item description is empty.

## Release v1.0 - 2020-04-27

* Add richtext messages
* Make feeds more configurable
* Various bugfixes and cleanups

## Release v0.9 - 2018-12-30

* Use separate configurable `FeedFile` for saving new feeds added via Slash
  Commands.
  The configuration file is no longer edited by the application.

## Release v0.8 - 2018-12-30

* Normalize capitalization of URL.
  Note! This is a backwards incompatible change, as the format of the
  configuration changed.

  Use `URL` everywhere instead of `Url` and sometimes `URL`.

## Release v0.7 - 2018-12-28

* Software has been released under MIT license.
* Add `-version` flag to software.
* Nicer responses to Slash Commands.

## Release v0.6 - 2018-12-23

* Make fetching interval configurable.
  See README for documentation.

## Release v0.4 - 2018-12-18

* Add Slash Command handler to be able to configure the feeds via Mattermost.
  See README for documentation.

## Release v0.2 - 2018-12-18

* Enable overriding defaults with per-feed configuration.
  Note! This is a backwards incompatible change, as the format of the
  configuration changed.

  Use `"Feeds": [{"Url":"https://..."}]` instead of `"Feeds": ["https://..."]`

## Release v0.1 - 2017-10-25

* Initial Release
