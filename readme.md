### WIP

Self-hosted server for generating social preview images

inspired by https://image.social/

## URL Parameters

| name  | default | description                                                        |
| ----- | ------- | ------------------------------------------------------------------ |
| url   | -       | URL to generate image for.                                         |
| width | 1400    | Width of browser viewport in pixels. Does not affect output width. |
| delay | 0       | Delay in milliseconds before generating image.                     |

## Environment Variables

| name                               | default | description                                                           |
| ---------------------------------- | ------- | --------------------------------------------------------------------- |
| ALLOWED_DOMAINS                    | -       | List of allowed domains.                                              |
| CACHE_TIME                         | 30 days | Time to cache images.                                                 |
| PORT                               | 8080    | Port to listen on.                                                    |
| REMOTE_URL                         | -       | Connect to an existing Chrome DevTools instance using a WebSocket URL |
| . For example: ws://localhost:9222 |

## Remote Browser Instance

Default behavior is to launch a new instance of Chrome for every screenshot.

To connect to an existing instance, use the `REMOTE_URL` environment variable.

### Examples

Using the chromedp `headless-shell` docker image:

```sh
docker run -d -p 9222:9222 --rm chromedp/headless-shell:latest
```

Using Google Chrome:

```sh
google-chrome-stable --remote-debugging-protocol=9222
```
