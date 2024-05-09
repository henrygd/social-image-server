# Social Image Server

Self-hosted server for generating social / open graph images.

Inspired by https://image.social/

## Installation

### Binary

You can download and run the latest binary from the [releases page](https://github.com/henrygd/social-image-server/releases). You must have Chrome or Chromium installed on your system.

### Docker

See the example [docker-compose.yml](/docker-compose.yml). The `chromedp/headless-shell` is needed to provide a browser instance. Other headless Chrome images should work but seem to be much larger.

It may be possible to use a native Chrome installation by giving the container access to your host ports and running Chrome with the `--remote-debugging-port` flag.

## Usage

The `/get` endpoint generates an image for any URL you pass in via the `url` query parameter.

Add an `og:image` meta tag into the `<head>` of your website.

```html
<meta property="og:image" content="https://yourserver.com/get?url=example.com" />
```

Ideally you want to use this in a layout template that will generate the correct URL for each page.

A useful site for previewing or generating boilerplate is [heymeta.com](https://www.heymeta.com/).

## URL Parameters

| Name        | Default | Description                                                                                                                                                                                     |
| ----------- | ------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `url`       | -       | URL to generate image for.                                                                                                                                                                      |
| `width`     | 1400    | Width of browser viewport in pixels (max 2500). Output image is scaled to 2000px width.                                                                                                         |
| `delay`     | 0       | Delay in milliseconds after page load before generating image.                                                                                                                                  |
| `dark`      | false   | Sets prefers-color-scheme to dark.                                                                                                                                                              |
| `cache_key` | -       | Regenerates image if changed. This is validated using your origin URL. If the `cache_key` doesn't match, the server will return a previously cached image (or error if no cached image exists). |
| `_regen_`   | -       | Do not use in public URLs. Forces full regeneration on every request. Use to manually purge a URL or tweak params, then remove. Must match `REGEN_KEY` value.                                   |

## Environment Variables

| Name            | Default | Description                                                                                         |
| --------------- | ------- | --------------------------------------------------------------------------------------------------- |
| ALLOWED_DOMAINS | -       | Restrict to certain domains. Example: "example.com,example.org"                                     |
| CACHE_TIME      | 30 days | Time to cache images on server.                                                                     |
| PORT            | 8080    | Port to listen on.                                                                                  |
| REMOTE_URL      | -       | Connect to an existing Chrome DevTools instance using a WebSocket URL. Example: ws://localhost:9222 |
| REGEN_KEY       | -       | Key used to bypass cache for specific URL. Use to tweak delay / width.                              |
| DATA_DIR        | ./data  | Directory to store program data (images and database).                                              |
| FONT_FAMILY     | -       | Change browser fallback font. Must be available on your system / image.                             |

## Remote Browser Instance

Default behavior is to launch a new instance of Chrome for every screenshot.

To connect to an existing instance, use the `REMOTE_URL` environment variable.

### Examples

Using the chromedp `headless-shell` docker image (see [docker-compose.yml](https://github.com/henrygd/social-image-server/blob/main/docker-compose.yml)):

```sh
docker run -d -p 127.0.0.1:9222:9222 --rm chromedp/headless-shell:latest
```

Using Chrome directly (most flags are from [chromedp's default options](https://pkg.go.dev/github.com/chromedp/chromedp@v0.9.5#pkg-variables)):

```sh
google-chrome-stable --remote-debugging-port=9222 --headless=new --hide-scrollbars --font-render-hinting=none --disable-background-networking --enable-features=NetworkService,NetworkServiceInProcess --disable-extensions --disable-breakpad --disable-backgrounding-occluded-windows --disable-default-apps --disable-background-timer-throttling --disable-features=site-per-process,Translate,BlinkGenPropertyTrees --disable-hang-monitor --disable-client-side-phishing-detection --disable-popup-blocking --disable-prompt-on-repost --disable-sync --disable-translate --metrics-recording-only --no-first-run --password-store=basic --use-mock-keychain
```

## Frequently Asked Questions

### How can I add custom styles or scripts when the screenshot is taken?

The server's outgoing request to websites always includes the URL parameter `og-image-request=true`, so check for that.

### Why does the image look different than in my browser?

Likely because the website isn't providing fonts over the network and the browser is using a different default font than your personal setup. If you're using a native browser installation, you may be able to force the font using the `FONT_FAMILY` environment variable.

When using `chromedp/headless-shell`, sans-serif will fall back to DejaVu Sans, because it's the only font in the image. If that's an issue, try a different headless chrome image, or running a native Chrome installation with the `--remote-debugging-port` option.

It may be possible to mount local fonts in the `headless-shell` container, but I haven't tested that.

## Response headers

The server includes status headers for successful image requests. These are useful for debugging.

### X-Og-Cache

| Value | Description             |
| ----- | ----------------------- |
| HIT   | Cached image was served |
| MISS  | New image was generated |

### X-Og-Code

| Value | Description                                                                          |
| ----- | ------------------------------------------------------------------------------------ |
| 0     | New image generated because it did not exist in cache                                |
| 1     | New image generated due to `_regen_` parameter                                       |
| 2     | Found matching cached image                                                          |
| 3     | `cache_key` does not match `cache_key` on origin URL. Using previously cached image. |
