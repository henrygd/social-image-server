# Social Image Server

Self-hosted server to automatically generate unique OG images for every page of your website.

## Endpoints

### Capture

The `/capture` endpoint generates a screen capture for any URL you pass in via the `url` query parameter.

```html
<meta property="og:image" content="https://your-server/capture?url=turso.tech/pricing" />
```

<table width="100%">
  <thead>
    <tr>
      <th width="50%">Before</th>
      <th width="50%">After</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td width="50%"><img src="https://henrygd-assets.b-cdn.net/social-image-server/before-capture.png" alt="example of turso.tech/pricing link which is missing an og:image as of may 11 2024"/></td>
      <td width="50%"><img src="https://henrygd-assets.b-cdn.net/social-image-server/after-capture.webp" alt="example of turso.tech/pricing link using an image generated by the server as it's og:image"/></td>
    </tr>
  </tbody>
</table>

### Template

The `/template` endpoint renders any static web build using variables passed in via query parameters.

<!-- prettier-ignore -->
```html
<meta property="og:image" content="https://your-server/template/example?title=Tire+Dust+Makes+Up+The+Majority+of+Ocean+Microplastics&subhead=Researchers+say+tire+emissions+pose+a+threat+to+global+health%2C+and+EVs+could+make+the+problem+worse.&author=Lewin+Day&date=September+28%2C+2023&img=https%3A%2F%2Fthedrive.com%2Fuploads%2F2023%2F09%2F28%2FGettyImages-1428297317.jpg" />
```

<table width="100%">
  <thead>
    <tr>
      <th width="50%">Before</th>
      <th width="50%">After</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td width="50%"><img src="https://henrygd-assets.b-cdn.net/social-image-server/before-template.webp?" alt="example of link for article Tire Dust Makes Up The Majority of Ocean Microplastics which displays stock photo of tire"/></td>
      <td width="50%"><img src="https://henrygd-assets.b-cdn.net/social-image-server/after-template.png?" alt="example of turso.tech/pricing link using an image generated by the server as it's og:image"/></td>
    </tr>
  </tbody>
</table>

## Installation

### Binary

Download and run the latest binary from the [releases page](https://github.com/henrygd/social-image-server/releases) or use the command below. You must have Chrome or Chromium installed on your system.

Use the `--update` flag to update to the latest version.

```bash
curl -sL "https://github.com/henrygd/social-image-server/releases/latest/download/social-image-server_$(uname -s)_$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/').tar.gz" | tar -xz -O social-image-server | tee ./social-image-server >/dev/null && chmod +x social-image-server && ls social-image-server
```

### Docker

See the example [docker-compose](https://github.com/henrygd/social-image-server/blob/main/docker-compose.yml) or run the command below to try it out.

```bash
docker run --rm --init -p 8080:8080 henrygd/social-image-server
```

The image is bundled with a [slimmed down Chrome](https://github.com/chromedp/docker-headless-shell), but remains relatively lightweight -- about 10% the size of `browserless/chrome` -- and doesn't require a ton of resources.

> Note: The only font included in the image is DejaVu. If you want to use a different fallback font for websites that don't provide them, you need to mount your fonts in the container. See the [docker-compose](https://github.com/henrygd/social-image-server/blob/main/docker-compose.yml) for an example.

## Usage

Add an `og:image` meta tag in the `<head>` of your website that points to your server.

```html
<meta property="og:image" content="https://yourserver.com/capture?url=example.com/current-page" />
```

It's best to add this once in a layout template, rather than doing it for every page. See [Framework Examples](#framework-examples).

A useful site for previewing or generating boilerplate HTML is [metatags.io](https://metatags.io/).

### Templates

Templates are just static webpages -- like the output of `vite build` -- that render URL query parameters in the content.

You can use any web framework to create templates. I made the example above using Vite, Svelte, and Tailwind ([view relevant code](https://github.com/henrygd/social-image-server-template/blob/main/src/App.svelte)). The [build command](https://vitejs.dev/guide/build) generates the static files.

To add a template, create a folder containing your files in the `data/templates` directory. If your folder is called `my-template`, it would then be available at `/template/my-template`.

A `url` query parameter is still required for templates. It's used to prevent abuse by verifying that the requested image matches the image used on the origin URL. If you're just testing, use `_regen_` to skip verification and a dummy string like "test" as the url.

Please ensure that your query parameters are encoded in your request. You can use `encodeURIComponent` in JavaScript, `url.QueryEscape` in Go, `urlencode` in PHP, `urllib.parse.quote` in Python, `URLEncoder.encode` in Java, etc.

### Cache

You can refresh the cache for an image by changing any query parameter (or template name if applicable) in the origin HTML. If you're just testing, use the `_regen_` parameter.

If incoming request parameters don't match the cache, the server will verify that params on the origin URL have changed and generate a new image if so.

See [Framework Examples](#framework-examples) for examples of a version parameter that automatically refreshes the cache on new site builds.

## URL Parameters

| Name      | Default | Description                                                                                                                                     |
| --------- | ------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| `url`     | -       | URL to generate image for and verify against.                                                                                                   |
| `width`   | 1400    | Width of browser viewport in pixels (max 2500). Output image is scaled to `IMG_WIDTH` width.                                                    |
| `delay`   | 0       | Delay in milliseconds after page load before generating image.                                                                                  |
| `dark`    | false   | Sets prefers-color-scheme to dark.                                                                                                              |
| `format`  | -       | Image format. Defaults to `IMG_FORMAT` value if not specified.                                                                                  |
| `_regen_` | -       | Do not use in public URLs. Testing only. Skips origin verification and forces full regeneration on every request. Must match `REGEN_KEY` value. |

## Environment Variables

| Name              | Default | Description                                                                                                                        |
| ----------------- | ------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| `ALLOWED_DOMAINS` | -       | Restrict to certain domains. Example: "example.com,example.org"                                                                    |
| `CACHE_TIME`      | 30 days | Time to cache images on server. Minimum 1 hour.                                                                                    |
| `DATA_DIR`        | ./data  | Directory to store program data (images and database).                                                                             |
| `FONT_FAMILY`     | -       | Change browser fallback font. Must be available on your system / image.                                                            |
| `IMG_FORMAT`      | jpeg    | Default format if not specified in request. Valid values: "jpeg", "png".                                                           |
| `IMG_QUALITY`     | 92      | Compression quality (jpeg only).                                                                                                   |
| `IMG_WIDTH`       | 2000    | Width of output image in pixels.                                                                                                   |
| `LOG_LEVEL`       | info    | Logging level. Valid values: "debug", "info", "warn", "error".                                                                     |
| `MAX_TABS`        | 5       | Maximum number of active browser tabs. 2 or 3 is fine in most cases.                                                               |
| `PERSIST_BROWSER` | 5m      | Time to keep the browser process running after the last image generation. Valid units: "ms", "s", "m", "h". See FAQ for more info. |
| `PORT`            | 8080    | Port to listen on.                                                                                                                 |
| `REGEN_KEY`       | -       | Key used to force bypass cache.                                                                                                    |
| `REMOTE_URL`      | -       | Connect to an existing Chrome or Chromium instance using WebSocket. Example: wss://localhost:9222                                  |

## Frequently Asked Questions

### Does this require Chrome / Chromium running in the background indefinitely?

Only if you use `REMOTE_URL` to connect with a remote browser process. Otherwise the server will manage headless browser processes as necessary.

If it needs to generate an image and there is no process already running, it will start one. After the image is generated, a timer is started. If another image generation occurs within the timer duration, the browser process is reused, and the timer is reset. If the timer reaches zero, the browser process is terminated.

This way active servers get the performance benefits of reusing the browser process, while less active servers will not need to keep Chrome running in the background forever.

The timer duration can be configured with the 10`PERSIST_BROWSER` environment variable.

### How can I add custom styles or scripts when the screenshot is taken?

The server's outgoing request to websites always includes the URL parameter `og-image-request=true`, so check for that. Add a short delay if you're doing the check on the front end.

### Why does the image look different than in my browser?

Probably because the website isn't providing fonts over the network, and the browser has a different default font than your personal setup.

You should be able to change the font using the `FONT_FAMILY` environment variable. If using docker, mount your fonts in the container. Check the [docker-compose](https://github.com/henrygd/social-image-server/blob/main/docker-compose.yml) for a commented-out example.

If you're using a remote browser (not recommended), try setting the `--system-font-family` flag on the process.

### Why is webp not an image format option?

From what I can tell, Facebook and LinkedIn (and likely others) don't support webp open graph images. If I'm wrong, let me know and I'll add it.

### I changed my image but Twitter is still showing the old one

Twitter seems to cache images for a long time. Try adding or removing a trailing slash, query parameter, or fragment to the URL in your tweet.

## Security recommendations

- **Do not run a public server without setting `ALLOWED_DOMAINS`**. Without restrictions, an attacker can use your browser to visit a malicious URL.
- **Do not leak your regen key in your HTML**. The regen key force bypasses the cache and URL status verification, so an attacker can attempt to DoS the server by sending thousands of requests to different URL paths. If you think you may have leaked it, change the `REGEN_KEY` environment variable or remove it entirely.
- **Keep `MAX_TABS` to a reasonable value**. Your OG images are cached both on the server and usually by the service you're sharing to, so it's unlikely that you'll be handling lots of simultaneous image generations. Most servers will be fine with 2 or 3 max tabs. If all tabs are in use, new requests are just queued until one of the tabs is free.

## Remote Browser

Use the `REMOTE_URL` environment variable to connect to a remote instance of Chrome or Chromium over WebSocket.

> [!IMPORTANT]
> This approach is only recommended if you already have an existing browser process running full time. The server cannot stop / start the process, so it will need to run independently for the lifetime of the server.
>
> I'd also recommend using the binary, since it doesn't come bundled with its own browser, though I can make a tiny docker image with only the server if there's any demand for it.

If you're using a container image for Chrome, check the documentation for a port (usually 9222) or address to connect to. If you're using the server binary, expose the Chrome container's port to the host: `127.0.0.1:9222:9222`. If using the docker version, put the container in the same network as Chrome and connect using its container name: `wss://chrome-container:9222`.

If using Chrome directly, set the `--remote-debugging-port` flag. Note that if you're running the server as a container you will need to give it access to your host ports.

```sh
google-chrome-stable --remote-debugging-port=9222 --headless=new --hide-scrollbars --font-render-hinting=none --disable-background-networking --enable-features=NetworkService,NetworkServiceInProcess --disable-extensions --disable-breakpad --disable-backgrounding-occluded-windows --disable-default-apps --disable-background-timer-throttling --disable-features=site-per-process,Translate,BlinkGenPropertyTrees --disable-hang-monitor --disable-client-side-phishing-detection --disable-popup-blocking --disable-prompt-on-repost --disable-sync --disable-translate --metrics-recording-only --no-first-run --password-store=basic --use-mock-keychain
```

## Response headers

The server includes status headers for successful image requests.

### X-Og-Cache

| Value | Description             |
| ----- | ----------------------- |
| HIT   | Cached image was served |
| MISS  | New image was generated |

### X-Og-Code

| Value | Description                                                                   |
| ----- | ----------------------------------------------------------------------------- |
| 0     | New image generated because it did not exist in cache                         |
| 1     | New image generated due to `_regen_` parameter                                |
| 2     | Found matching cached image                                                   |
| 3     | Request does not match og:image on origin URL. Using previously cached image. |

## Framework examples

These examples use a query parameter `v` to bypass cache on new builds, but you can remove it if you don't need that functionality. Feel free to improve these or contribute others.

### SvelteKit

Make sure you set your website's origin in [prerender settings](https://kit.svelte.dev/docs/configuration#prerender).

```svelte
<!-- +layout.svelte -->
<script>
  import { version } from '$app/environment'
  import { page } from '$app/stores'
</script>

<sveltekit:head>
  <meta
    property="og:image"
    content="https://your-server/capture?url={$page.url.toString()}&v={version}"
  />
</sveltekit:head>
```

### Astro

```astro
<!-- Layout.astro -->
---
import {version} from '../../package.json';
---

<head>
  <meta
    property="og:image"
    content={`https://your-server/capture?url=${Astro.url}&v=${version}`}
  />
</head>
```

### WordPress

```php
// header.php
<?php
  $page_url = home_url($_SERVER['REQUEST_URI']);
  $version = wp_get_theme()->get('Version');
  $og_image_url = "https://your-server/capture?url=$page_url&v=$version";
?>

<head>
  <meta property="og:image" content="<?php echo $og_image_url ?>"/>
</head>
```
