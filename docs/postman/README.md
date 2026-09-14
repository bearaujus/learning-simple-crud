# Postman examples

Import [`products.postman_collection.json`](products.postman_collection.json) into Postman. This collection belongs with the learning documentation; the API server does not load it at runtime.

1. From the repository root, start the API with `go run ./cmd/api`.
2. Import the collection using Postman's **Import** action.
3. Open the collection's variables. `base_url` defaults to `http://localhost:8080`; change it if your server uses another address. Omit the trailing slash.
4. Run the **CRUD walkthrough (run in order)** folder. The create request saves the new ID into `product_id`, which the remaining requests use. You do not need to set it manually.
5. Open **Learn error handling (expected failures)** to explore validation and other errors. These requests intentionally return 4xx statuses, and their tests expect those statuses.

The walkthrough creates and deletes its own product. Other products are preserved. If you stop the walkthrough before deletion, the created product remains in the database.

No separate environment file is required: the collection includes its variables and uses no credentials. Keep the `.postman_collection.json` suffix when exporting updates so its purpose remains clear.

For request schemas and response definitions, see [`api/openapi.json`](../../api/openapi.json). For fetch examples, see the [frontend guide](../frontend-guide.md).
