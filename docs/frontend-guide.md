# Build your first product frontend

Start the API with `go run ./cmd/api` in one terminal. Open its playground at `http://localhost:8080` to check it is running. Your own frontend can run on a different port; the API enables CORS for local practice.

## A working first page

Create a separate frontend folder. Copy this repository's `web/api.js` into that folder, then create `index.html` with this content:

```html
<!doctype html>
<html lang="en">
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>My first product list</title>
  <h1>Products</h1>
  <button id="refresh">Refresh</button>
  <p id="status" role="status"></p>
  <ul id="products"></ul>
  <script type="module">
    import { createClient, APIError } from "./api.js";
    const api = createClient("http://localhost:8080");
    const list = document.querySelector("#products");
    const status = document.querySelector("#status");
    const refresh = document.querySelector("#refresh");
    let controller;

    async function loadProducts() {
      controller?.abort();
      const current = new AbortController();
      controller = current;
      refresh.disabled = true;
      status.textContent = "Loading…";
      try {
        const { data, meta } = await api.list({ page: 1, limit: 10 }, current.signal);
        if (current !== controller) return;
        list.replaceChildren(...data.map((product) => {
          const item = document.createElement("li");
          item.textContent = `${product.name}: ${product.price.toFixed(2)}`;
          return item;
        }));
        status.textContent = data.length
          ? `Showing ${data.length} of ${meta.total} products.`
          : "No products yet. Create one in the playground.";
      } catch (error) {
        if (current !== controller || error.name === "AbortError") return;
        status.textContent = error instanceof APIError
          ? error.message
          : "Cannot reach the API. Check the server and try again.";
      } finally {
        if (current === controller) refresh.disabled = false;
      }
    }

    refresh.addEventListener("click", loadProducts);
    loadProducts();
  </script>
</html>
```

Serve this folder over HTTP using your editor's local server or your normal frontend tooling. If you have Python installed, `python -m http.server 5173` works too; visit `http://localhost:5173`. Avoid opening the HTML with `file://`, because browser module loading requires a suitable origin.

The page reads real data. Create or edit a product in the playground, then refresh your own page to see it.

## Add one feature at a time

1. **Create:** add labeled name, price, and description inputs. On submit, prevent the default page navigation and call `api.create({ name, price: Number(priceInput.value), description })`. Use `required` on price so an empty string does not silently become zero. Refresh the list after success.
2. **Validate:** try an empty name, a negative price, and a string price. Catch `APIError`; put `error.details.name` and `error.details.price` beside their inputs. Keep the entered values when saving fails. Browser validation improves feedback, but the server remains authoritative.
3. **Edit:** use `api.get(id)` to load current values. Save the complete form with `api.replace(id, { name, price, description })`. The server may return `404` if another client deleted the product.
4. **Patch:** add a button calling `api.patch(id, { price: 0 })`. Confirm the name and description survive. Use an empty string to clear a description; omitting a field preserves it.
5. **Delete:** confirm the action, await `api.remove(id)`, then update your list. A successful delete has no JSON body. A second deletion returns `404` because the item is already gone.
6. **Search:** call `api.list({ q: searchInput.value, page: 1 })`. Use `URLSearchParams` rather than concatenating raw user input. Reset to page 1 when filters change.
7. **Paginate:** keep `page` and `limit` in state. Disable Previous on page 1; disable Next when `page >= meta.total_pages`. After deletion, move back if the last page is empty.
8. **Handle slow requests:** use your browser's Network throttling, show loading feedback, and disable Save while it is pending. Restore controls in `finally`. Cancel outdated reads or ignore their results so old responses cannot replace newer search results.
9. **Handle failures:** stop the API, try Refresh, and show a retry action. Restart it and recover without reloading the page. Cancelled reads should not display an error to the user.

## Practices to carry into other projects

- Keep HTTP calls in one module. UI components should not each reinvent headers, parsing, and error handling.
- Check `response.ok`. `fetch()` rejects for network failures and cancellation, but resolves for HTTP `404`, `422`, and `500` responses. [MDN Fetch guide](https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API/Using_Fetch).
- Match UI states to reality: loading, success, empty results, validation errors, and connection errors need different feedback.
- Use the representation returned by a successful write; the server assigns the ID and normalizes values.
- Disable duplicate submissions. Do not automatically retry a POST after a network failure: the server may already have committed it. Refresh and inspect the list before trying again. The API does not implement idempotency keys.
- Render names, descriptions, and error messages with `textContent` or your framework's normal escaped text binding. User-provided HTML is data, not markup to execute.
- Do not treat an allowed CORS origin as a login. This API deliberately has no authentication; real apps need separate authorization checks.
- Keep API URLs in one configuration point. Never embed server secrets in frontend code.
- Use a product's server ID as its list key; array indexes change when the user sorts, filters, or deletes.
- Keep HTTP status and stable `error.code` for decisions; treat human-readable messages as text that may change.

See [the complete playground](../web/app.js) for these patterns working together. It keeps writes explicit and refreshes after a confirmed response. Simultaneous edits to the same field use last-write-wins; conflict detection with versions or ETags is a useful later exercise.

## Common problems

| Symptom | Check |
| --- | --- |
| `Failed to fetch` | Is the Go server running? Is the API URL correct? Check DevTools Network and any configured CORS allowlist. |
| `415` | JSON writes need `Content-Type: application/json`. Use the client helper or set it yourself. |
| `422` for a price | Send a number, not the string returned by an input. Read `error.details.price`. |
| `Unexpected end of JSON input` after deletion | Do not parse the empty `204` response. |
| `.map is not a function` | Map `payload.data`; the payload also contains pagination metadata. |
| A partial PUT fails | Include name and price, or use PATCH for the changed fields. |
| A created product seems missing | Clear search filters, inspect sorting, and refresh the first page. Check which database the server uses. |
| Only 10 products appear | Request subsequent pages or a larger `limit` (maximum 100). |
| Browser says CORS error | Match the frontend's exact origin in `CORS_ORIGINS`, including its port. `localhost` differs from `127.0.0.1`. Do not use `mode: "no-cors"`; that hides the response. |
