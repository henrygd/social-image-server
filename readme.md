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

| name  | default | description                                                                                                            |
| ----- | ------- | ---------------------------------------------------------------------------------------------------------------------- |
| url   | -       | URL to generate image for.                                                                                             |
| width | 1400    | Width of browser viewport in pixels. Output image is scaled to 2200px width.                                           |
| delay | 0       | Delay in milliseconds after page load before generating image.                                                         |
| regen | -       | Bypasses and clears cache for URL. Use to tweak delay / width. Must match `REGEN_KEY` value. Don't use in public URLs. |

## Environment Variables

| name            | default | description                                                                                         |
| --------------- | ------- | --------------------------------------------------------------------------------------------------- |
| ALLOWED_DOMAINS | -       | List of allowed domains. Example: "example.com example.org"                                         |
| CACHE_TIME      | 30 days | Time to cache images on server.                                                                     |
| PORT            | 8080    | Port to listen on.                                                                                  |
| REMOTE_URL      | -       | Connect to an existing Chrome DevTools instance using a WebSocket URL. Example: ws://localhost:9222 |
| REGEN_KEY       | -       | Key used to bypass cache for specific URL. Use to tweak delay / width.                              |

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
