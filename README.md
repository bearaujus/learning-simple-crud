# CRUD lab

A small Go + SQLite API for frontend developers learning **create, read, update, and delete**. Start one server, open the browser playground, and connect your own frontend. No API key, database installation, Node.js, or frontend build step is required to run it.

The API demonstrates predictable JSON, useful field errors, HTTP status codes, bounded pagination, server-side validation, and safe partial updates. The playground is plain HTML/CSS/JavaScript so you can read and reuse it.

## Start here

Install **Go 1.25.7 or newer**, then run these commands from this repository:

```sh
go mod download
go run ./cmd/api -seed
go run ./cmd/api
```

1. Open **[http://localhost:8080](http://localhost:8080)** for the playground.
2. Create a product, edit it, use **Make free** to send a `PATCH`, and delete it.
3. Inspect the HTTP method, status, JSON, and generated `fetch` example below the list. Select an earlier request to inspect a write after the list refreshes.
4. Open **[the API guide](http://localhost:8080/docs)** and build your own frontend against `http://localhost:8080`.

`-seed` is optional. It inserts six examples **only if the database is empty**, then exits. Existing products and edits are preserved. An unseeded database starts with an empty list. Ctrl+C stops the server; products persist in `products.db` across restarts.

Run commands from the repository root; `cmd/api` is the executable package. SQLite paths remain relative to your working directory.

> **Changes from the original API:** successful product responses now use `{ "data": ... }`; lists use `{ "data": [...], "meta": ... }` and default to 10 items. Errors now contain an object under `error`. `DELETE` returns `204` with no body. `PUT` requires `name` and `price`; use `PATCH` for partial updates. Existing database files remain compatible. Update existing frontend consumers to match this contract.

## Endpoints

| Method | Path | Behavior | Success |
| --- | --- | --- | --- |
| GET | `/health` | Check database connectivity | 200 |
| GET | `/products` | List, search, sort, and paginate | 200 |
| GET | `/products/{id}` | Read one product | 200 |
| POST | `/products` | Create a product | 201 + `Location` header |
| PUT | `/products/{id}` | Replace a product's writable fields | 200 |
| PATCH | `/products/{id}` | Change only supplied fields | 200 |
| DELETE | `/products/{id}` | Delete a product | 204, no body |

`GET` routes also accept `HEAD`. Known routes support `OPTIONS` for browser CORS preflights. Unknown endpoints return JSON `404`; unsupported methods return JSON `405` with an `Allow` header. Use the paths as shown, without a trailing slash.

### Product fields

```json
{
  "id": 1,
  "name": "Notebook",
  "price": 4.5,
  "description": "A dotted notebook."
}
```

| Field | Rules |
| --- | --- |
| `id` | Positive integer assigned by the server; do not send it in write bodies. |
| `name` | Required for POST/PUT. A string of 1–100 Unicode characters after trimming whitespace. |
| `price` | Required for POST/PUT. A JSON number from 0 to 1,000,000 with at most two decimal places. Zero is valid. |
| `description` | A string of at most 1,000 Unicode characters after trimming. Optional for POST/PUT; defaults to `""`. |

Send writes with `Content-Type: application/json`. Unknown fields, wrong types, and explicit `null` values are rejected. JSON bodies are limited to 64 KiB. Field names are case sensitive. HTML input values are strings: convert a nonempty price with `Number(value)` before sending it.

**PUT versus PATCH:** `PUT` must include `name` and `price`; omitting `description` clears it. `PATCH` must include at least one writable field and preserves omitted fields. Send `{ "description": "" }` to clear the description or `{ "price": 0 }` to make the product free. This API accepts a partial JSON object for PATCH, not JSON Patch or JSON Merge Patch. Both methods return `404` for a missing product.

### Successful responses

POST, GET-one, PUT, and PATCH return the current product under `data`:

```json
{
  "data": {
    "id": 1,
    "name": "Notebook",
    "price": 4.5,
    "description": "A dotted notebook."
  }
}
```

Lists add pagination metadata:

```json
{
  "data": [],
  "meta": { "page": 1, "limit": 10, "total": 0, "total_pages": 0 }
}
```

`data` is always an array for lists, including empty or out-of-range pages. `total` is the number of matching products before pagination. `total_pages` is zero when there are no matches. After deleting the final item on a page, your frontend can move back to the last available page.

### Query parameters

```text
GET /products?page=1&limit=5&q=notebook&sort=price&order=asc
```

| Parameter | Default | Accepted values |
| --- | --- | --- |
| `page` | `1` | Integer from 1 to 1,000,000 |
| `limit` | `10` | Integer from 1 to 100 |
| `q` | Empty | Literal substring in name or description; up to 100 trimmed characters |
| `sort` | `id` | `id`, `name`, `price` |
| `order` | `asc` | `asc`, `desc` |

Use `URLSearchParams` to encode filters. Search and name sorting ignore ASCII letter case; SQLite's default case folding does not cover every language. Equal names/prices are ordered by ID ascending for stable pagination. Unknown, repeated, or invalid parameters return `400` with parameter messages under `error.details`.

### Error responses

An invalid product returns `422`:

```json
{
  "error": {
    "code": "validation_error",
    "message": "Please fix the highlighted fields.",
    "details": {
      "name": "This field is required.",
      "price": "Must be between 0 and 1000000."
    },
    "request_id": "example-request-id"
  }
}
```

Use `error.code` for program logic, `error.message` for general feedback, and `error.details[field]` beside form inputs. Details are optional. Each response includes `X-Request-ID`; errors repeat it as `error.request_id` so a failure can be correlated with server logs. Internal database details stay in server logs.

| Status | Error code | What your frontend should do |
| --- | --- | --- |
| 400 | `invalid_json`, `invalid_id`, `invalid_query` | Fix the request format, ID, or filters. |
| 404 | `product_not_found`, `route_not_found` | Show a missing-item state or correct the endpoint. |
| 405 | `method_not_allowed` | Check the HTTP method and `Allow` header. |
| 413 | `payload_too_large` | Reduce the request body. |
| 415 | `unsupported_media_type` | Set `Content-Type: application/json`. |
| 422 | `validation_error` | Display the field errors and keep the user's inputs. |
| 500 | `internal_error` | Show a retry action and retain the request ID. |
| 503 | `service_unavailable` | The health check cannot reach the database. |

## Connect your frontend

Copy [`web/api.js`](web/api.js) into your frontend. It checks `response.ok`, preserves field errors, supports cancellation for reads, and handles `204` without parsing an empty body.

```js
import { createClient, APIError } from "./api.js";

const api = createClient("http://localhost:8080");

async function practice() {
  try {
    const { data: created } = await api.create({
      name: "Notebook", price: 4.5, description: "A dotted notebook."
    });
    const { data: product } = await api.get(created.id);
    await api.patch(product.id, { price: 0 });
    await api.replace(product.id, {
      name: "Sketchbook", price: 9.99, description: "Blank pages."
    });
    const { data: products, meta } = await api.list({ page: 1, limit: 5 });
    console.log(products, meta);
    await api.remove(product.id); // Resolves to null; response was 204.
  } catch (error) {
    if (error instanceof APIError) {
      console.error(error.status, error.message, error.details);
    } else {
      console.error("Could not reach the API", error);
    }
  }
}

practice();
```

For a complete standalone frontend and exercises, see the [frontend learning guide](docs/frontend-guide.md). The working [playground source](web/app.js) also demonstrates loading and empty states, field errors, cancellation of stale list requests, delete confirmation, and disabling repeated submissions.

## Postman and OpenAPI

Import [`products.postman_collection.json`](docs/postman/products.postman_collection.json). Its `base_url` defaults to `http://localhost:8080`. Run the **CRUD walkthrough** folder in order: creation saves the returned ID to `product_id`, so subsequent requests work without assuming ID 1. The folder deletes only the product it created. A separate folder demonstrates expected errors.

Collections live in `docs/postman/` alongside their [import instructions](docs/postman/README.md). They are learning examples, not files needed by the running server. The lowercase `products.postman_collection.json` name identifies the resource and preserves Postman's recognizable file suffix.

The [OpenAPI 3.0 specification](api/openapi.json) is also served at `/openapi.json` and can be imported into API tools. It describes request fields, query parameters, responses, and validation errors. `/docs` is a local API guide with no CDN dependency.

## Configuration

| Environment variable | Default | Purpose |
| --- | --- | --- |
| `ADDR` | `127.0.0.1:8080` | Server bind address |
| `DATABASE_PATH` | `products.db` | SQLite file path; parent directory must exist |
| `CORS_ORIGINS` | `*` | Comma-separated frontend origins, or `*` for local practice |

PowerShell:

```powershell
$env:ADDR = "127.0.0.1:8081"
$env:DATABASE_PATH = "practice.db"
$env:CORS_ORIGINS = "http://localhost:5173,http://localhost:3000"
go run ./cmd/api
```

Bash:

```sh
ADDR=127.0.0.1:8081 DATABASE_PATH=practice.db CORS_ORIGINS=http://localhost:5173 go run ./cmd/api
```

Environment variables are read from the process; `.env` files are not loaded automatically. Set `DATABASE_PATH` to a new filename for a fresh exercise without deleting your existing data. Setting it to `:memory:` creates an empty, temporary database for that server process.

The default CORS setting lets local frontends use any port. An allowlist matches the exact scheme, host, and port: `localhost` and `127.0.0.1` are different origins. Credentials and authentication are not used. CORS controls browser access to responses; it does not authenticate requests. This is a shared local learning database, so any connected client can modify its products. Before public hosting, add authentication/authorization, per-user data isolation, rate limits, and deployment-specific controls.

Prices retain the original SQLite `REAL` storage for compatibility. Two-decimal validation is useful for the exercise; production money calculations should use integer minor units or an exact decimal representation with an explicit currency.

## Check and build

```sh
go test ./...
go vet ./...
go build -o bin/ ./cmd/api
```

The build produces `bin/api` on Linux/macOS or `bin/api.exe` on Windows. The playground and OpenAPI contract are embedded in the binary, so serving them does not depend on the source tree being present.

With Node.js 22 or newer, also check the fetch client:

```sh
node --test web/api.test.mjs
```

API tests use isolated databases and cover CRUD, validation, filtering, pagination, CORS, errors, concurrent PATCH updates, seeding, and compatibility with the old schema. They never use your `products.db`. CI runs these checks and Go's race detector. Local `go test -race ./...` requires a supported C toolchain even though the normal server does not.

Optional Make targets: `run`, `seed`, `test`, `check`, `build`, and `clean`. Cleaning does not delete your database. Generated binaries and database files are ignored by Git.

## Project map

```text
learning-simple-crud/
├── cmd/api/
│   └── main.go                         # Configuration, startup, shutdown
├── internal/app/
│   ├── database.go                     # SQLite setup and sample products
│   ├── products.go                     # Validation, queries, CRUD handlers
│   ├── server.go                       # Routes, middleware, response helpers
│   └── server_test.go                  # API and persistence tests
├── api/
│   ├── openapi.json                    # Machine-readable API contract
│   └── spec.go                         # Embed the contract in the binary
├── web/
│   ├── assets.go                       # Embed the browser assets
│   ├── index.html                      # CRUD playground
│   ├── docs.html                       # Browser API guide
│   ├── styles.css
│   ├── app.js                          # Playground interactions
│   ├── api.js                          # Reusable frontend fetch client
│   └── api.test.mjs                     # Fetch client tests
├── docs/
│   ├── frontend-guide.md
│   └── postman/
│       ├── README.md                   # Import and walkthrough instructions
│       └── products.postman_collection.json
├── .github/workflows/check.yml
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

This follows [Go's server-project layout guidance](https://go.dev/doc/modules/layout#server-project): executable entry points live in `cmd/`, and application implementation lives in `internal/`. Keep tests next to the code they exercise. The small backend stays in one application package, with related responsibilities organized by file.

`api/` contains the API contract, `web/` contains the frontend learners can inspect and reuse, and `docs/` contains learning material and tool-specific examples. The two small embedding files keep assets beside their source files and include them in the executable at build time.

Generated binaries go in the ignored `bin/` directory. The ignored local `products.db` remains in the repository root by default so existing data continues to work; use `DATABASE_PATH` to choose another location.

HTTP behavior follows [HTTP Semantics (RFC 9110)](https://www.rfc-editor.org/rfc/rfc9110.html); the frontend examples follow [MDN's Fetch guide](https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API/Using_Fetch). The response envelope is this project's convention, not a requirement of HTTP.
