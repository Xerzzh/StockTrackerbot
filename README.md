# StockTrackerBot

A Telegram bot written in Go that watches product pages on several Spanish /
European stores and notifies you as soon as an item becomes available
(i.e. as soon as the store shows an *add to cart* button).

It is configured entirely from Telegram: you add a product, paste its URL and
choose how often it should be checked. No config files to edit by hand.

## Supported stores

| Store | Detection strategy |
| --- | --- |
| **Amazon** (`amazon.es`, `amazon.de`, `amazon.fr`, `amazon.it`, `amazon.co.uk`, `amazon.nl`, `amazon.se`, `amazon.pl`, `amazon.com.be`) | Looks for a real *add to cart* / *buy now* button (`input`/`button`, plus Amazon's stable `*-announce` spans) in several languages. The keyboard-shortcut helper widget, which repeats the same text, is explicitly ignored. If no buy button is present the item is reported as out of stock. CAPTCHA pages are reported as *unknown*. |
| **GAME** (`game.es`) | Looks for `Añadir a la cesta`; `PRÓXIMAMENTE`, `Agotado` or `No disponible` mean out of stock. |
| **Xtralife** (`xtralife.com`) | Uses Xtralife's public JSON API (`/public-api/v1/sku`) and maps its `disponibility` field: `sell` and `reservation` are available; `out_of_stock`, `reservation_not_opened`, `restock`, `archived`, etc. are not. |
| **MediaMarkt** (`mediamarkt.es`) | Reads the product's `schema.org` JSON-LD (`InStock` / `OutOfStock` / `PreOrder`…), with a visible-text fallback. |
| **Fnac** (`fnac.es`) | Looks for the stable `data-automation-id="product-buy-btn-label"` add-to-cart button; `No disponible en Fnac.es` means out of stock. DataDome CAPTCHA pages are reported as *unknown*. |
| **Carrefour** (`carrefour.es`) | The *Añadir* button is shown even when there is no stock, so availability is read from the product's `schema.org` JSON-LD (`InStock` / `OutOfStock`). Cloudflare challenge pages are reported as *unknown*. |
| **El Corte Inglés** (`elcorteingles.es`) | Reads the main buy button (`#add_to_cart_main_button` / `data-testid="pdp-add-to-cart"`): disabled or `AGOTADO` means out of stock. The site answers `410 Gone` for sold-out products but still serves the button, so the body is inspected in that case. |

> Detection is based on visible text, structured data (JSON-LD) or public APIs,
> **not** on CSS classes or element ids, which are often auto-generated and
> change frequently. Nintendo Store is **not** supported because it sits behind
> a JavaScript/Queue-it waiting room that cannot be bypassed without a headless
> browser.

## How it works

1. Each user has a list of watched products. A product is
   `{name, sources, interval}`, where `sources` is one or more URLs from the
   supported stores. Adding several stores to the same product avoids duplicate
   notifications.
2. When the monitor is running, a goroutine per user ticks every few seconds and
   checks only the products whose own interval has elapsed. Every source of a
   product is checked independently.
3. Each check dispatches to the store-specific checker, which returns one of:
   - 🟢 **Available** — an add-to-cart / buy button was found.
   - 🔴 **Out of stock** — an explicit out-of-stock message was found (or, for
     Amazon, no buy button at all).
   - ⚪ **Unknown** — the page could not be interpreted (CAPTCHA, network error…).
4. The status is persisted and compared with the previous one. When a product
   **transitions to available** in any of its stores, a single Telegram
   notification is sent with an *Open* button per available store.
5. State survives restarts: products live in `config/products_<chatID>.json`.

## Requirements

- Go 1.25+ (only to build from source), or Docker.
- A Telegram bot token from [@BotFather](https://t.me/BotFather).

## Configuration

Copy `.env.example` to `.env` and fill it in:

```dotenv
# Required: token from @BotFather
TELEGRAM_BOT_TOKEN=123456:ABC-DEF...

# Optional: admin chat id. If set, only the admin and authorized users can use
# the bot. The admin can authorize/revoke other users from the bot menu.
# If empty, the bot is open to anyone who knows it.
ADMIN_CHAT_ID=

# Optional: HTTP timeout per request (Go duration syntax: 30s, 1m...). Min 5s.
HTTP_TIMEOUT=30s

# Optional: directory where products and authorized users are stored.
STATE_DIR=config
```

The `.env` file is loaded automatically if present, but real environment
variables always take precedence.

### Finding your chat id

Send a message to the bot and look at its logs: unauthorized attempts are
logged with the user id. Set that id as `ADMIN_CHAT_ID`.

## Running

### From source

```bash
make run          # go run .
# or
go build -o stocktrackerbot . && ./stocktrackerbot
```

### With Docker

```bash
docker build -t stocktrackerbot .
docker run -d \
  --name stocktrackerbot \
  --env-file .env \
  -v stocktracker-data:/data \
  stocktrackerbot
```

The container runs from `/data`, so `STATE_DIR=config` is stored in the volume.

## Using the bot

Open a chat with your bot and send `/menu`.

| Command | Description |
| --- | --- |
| `/menu` | Main menu with inline buttons. |
| `/add` | Add a product: **name → URLs → interval (minutes or seconds)**. Send one or several URLs (from different stores) and press **Listo**; the store is detected from each URL. |
| `/list` | Show all products with their last status and evidence. |
| `/check` | Check all products immediately. |
| `/startbot` | Start the periodic monitor. |
| `/stopbot` | Stop the periodic monitor. |
| `/edit` | Edit a product's name, URL or interval. |
| `/delete` | Delete a product. |

**Admin only** (inline buttons in the main menu when `ADMIN_CHAT_ID` matches):
*Authorize ID*, *Revoke ID* and *List IDs*.

### Example flow

```
/menu
/add
> Switch 2 Zelda
> https://www.game.es/nintendo-switch-2-edicion-zelda-40th-nintendo-switch-2-267689
> https://www.amazon.es/dp/B0F2TN43GH
> [✅ Listo]
> 30s
/startbot
```

Intervals can be written as plain minutes (`5`), minutes (`2m`, `5min`) or
seconds (`30s`, `90seg`). From then on, the bot checks all those URLs every 30
seconds and messages you once the moment the product becomes available in any
of them.

## Persistence and data files

| Path | Contents |
| --- | --- |
| `config/products_<chatID>.json` | Watched products and their last known status. |
| `config/authorized_users.txt` | Authorized chat ids (when `ADMIN_CHAT_ID` is set). |

## Development

```bash
make test   # go test ./...
make vet    # go vet ./...
make fmt    # gofmt -w *.go
make build  # static binary
```

The store detection logic is covered by unit tests using HTML/JSON fixtures, so
it can be validated without network access.

## Project structure

| File | Responsibility |
| --- | --- |
| `main.go` | Entry point: config, Telegram client, signal handling. |
| `bot.go` | Update routing, command registration, per-user config. |
| `telegram.go` | Commands, inline menus and the add/edit/delete wizard. |
| `monitor.go` | Scheduling, product checks and stock notifications. |
| `stores.go` | Store registry and per-store checkers. |
| `check.go` | Generic text/button detection, JSON-LD and Xtralife parsing. |
| `dom.go` | DOM helpers (visible text, actionable elements, attributes). |
| `scraper.go` | HTTP client with browser headers and per-domain language. |
| `storage.go` | Product persistence. |
| `auth.go` | Admin / authorized users. |
| `types.go` | Product, status and user state types. |
| `config.go` / `helpers.go` | Environment loading and formatting helpers. |

## Limitations

- Sites rendered entirely client-side (Angular/React SPAs) that do not expose an
  API or structured data cannot be scraped with plain HTTP.
- Amazon may occasionally serve a CAPTCHA; those checks are reported as
  *unknown* rather than a false result.
- The minimum check interval is 10 seconds and the maximum is 24 h.
- Please respect each store's terms of service and do not set aggressive
  intervals.
