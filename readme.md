### WIP

Self-hosted server for generating social preview images

Inspired by https://image.social/

## Installation

### Docker

todo

### Binary

todo - must have libvips and browser available

## Usage

Make request to `/get` route with URL parameter `url`.

## URL Parameters

| name  | default | description                                                        |
| ----- | ------- | ------------------------------------------------------------------ |
| url   | -       | URL to generate image for.                                         |
| width | 1400    | Width of browser viewport in pixels. Does not affect output width. |
| delay | 0       | Delay in milliseconds before generating image.                     |

## Environment Variables

| name            | default | description                                                                                             |
| --------------- | ------- | ------------------------------------------------------------------------------------------------------- |
| ALLOWED_DOMAINS | -       | List of allowed domains.                                                                                |
| CACHE_TIME      | 30 days | Time to cache images on server.                                                                         |
| PORT            | 8080    | Port to listen on.                                                                                      |
| REMOTE_URL      | -       | Connect to an existing Chrome DevTools instance using a WebSocket URL. For example: ws://localhost:9222 |
| KEY             | -       | Key used to bust cache for specific URL, if you need to change delay or width.                          |

## Remote Browser Instance

Default behavior is to launch a new instance of Chrome for every screenshot.

To connect to an existing instance, use the `REMOTE_URL` environment variable.

### Examples

Using the chromedp `headless-shell` docker image:

```sh
docker run -d -p 127.0.0.1:9222:9222 --rm chromedp/headless-shell:latest
```

Using Google Chrome:

```sh
google-chrome-stable --remote-debugging-protocol=9222
```
