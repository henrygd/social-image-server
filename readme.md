### WIP

self-hosted server for generating social preview images

inspired by https://image.social/

## URL Parameters

| name  | default | description                                                        |
| ----- | ------- | ------------------------------------------------------------------ |
| url   | -       | URL to generate image for.                                         |
| width | 1400    | Width of browser viewport in pixels. Does not affect output width. |
| delay | 0       | Delay in milliseconds before generating image.                     |

## Environment Variables

| name            | default | description              |
| --------------- | ------- | ------------------------ |
| ALLOWED_DOMAINS | -       | List of allowed domains. |
| CACHE_TIME      | 30 days | Time to cache images.    |
| PORT            | 8080    | Port to listen on.       |
