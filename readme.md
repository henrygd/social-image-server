### WIP

Self-hosted server for generating social preview images

Inspired by https://image.social/

## Installation

### Binary

If you have Chrome installed, you can download and run the latest binary from the [releases page](https://github.com/henrygd/social-image-server/releases).

### Docker

See the example [docker-compose.yml](/docker-compose.yml). The `chromedp/headless-shell` is needed to provide a Chrome instance. Other headless Chrome images should work but seem to be much larger.

## Usage

Make request to `/get` route with URL parameter `url`.

If you need to adjust `width` or `delay` you can use the `regen` parameter, but please remove it from any shared URLs.

Add the image URL to HTML inside the `head` tag. A useful site for testing and generating HTML is [heymeta.com](https://www.heymeta.com/).

```html
<meta property="og:image" content="https://yourserver.com/get?url=example.com" />
```

## URL Parameters

| name  | default | description                                                                                                            |
| ----- | ------- | ---------------------------------------------------------------------------------------------------------------------- |
| url   | -       | URL to generate image for.                                                                                             |
| width | 1400    | Width of browser viewport in pixels (max 2500). Output image is scaled to 2000px width.                                |
| delay | 0       | Delay in milliseconds after page load before generating image.                                                         |
| regen | -       | Bypasses and clears cache for URL. Use to tweak delay / width. Must match `REGEN_KEY` value. Don't use in public URLs. |

## Environment Variables

| name            | default | description                                                                                         |
| --------------- | ------- | --------------------------------------------------------------------------------------------------- |
| ALLOWED_DOMAINS | -       | List of allowed domains. Example: "example.com,example.org"                                         |
| CACHE_TIME      | 30 days | Time to cache images on server.                                                                     |
| PORT            | 8080    | Port to listen on.                                                                                  |
| REMOTE_URL      | -       | Connect to an existing Chrome DevTools instance using a WebSocket URL. Example: ws://localhost:9222 |
| REGEN_KEY       | -       | Key used to bypass cache for specific URL. Use to tweak delay / width.                              |
| DATA_DIR        | -       | Directory to store program data (images and database). Default: `./data`.                           |

## Remote Browser Instance

Default behavior is to launch a new instance of Chrome for every screenshot.

To connect to an existing instance, use the `REMOTE_URL` environment variable.

### Examples

Using the chromedp `headless-shell` docker image (see [docker-compose.yml](/docker-compose.yml)):

```sh
docker run -d -p 127.0.0.1:9222:9222 --rm chromedp/headless-shell:latest
```

Using Chrome directly:

```sh
google-chrome-stable --remote-debugging-port=9222
```

### Troubleshooting

When using `chromedp/headless-shell` on a site that doesn't provide fonts, sans-serif will fall back to DejaVu Sans. This can make the text look different than it does in your browser.

DejaVu Sans is the only font in the image, so if that's an issue, try using a different headless chrome image, or install chrome on your local machine and use the binary.
