import { test } from "node:test";
import assert from "node:assert/strict";
import { APIError, createClient } from "./api.js";

test("create sends numeric zero and the JSON content type", async (t) => {
  const product = { name: "Free", price: 0, description: "" };
  const observed = [];
  t.mock.method(globalThis, "fetch", async (url, options) => {
    assert.equal(url, "http://localhost:8080/products");
    assert.equal(options.method, "POST");
    assert.equal(options.headers["Content-Type"], "application/json");
    assert.deepEqual(JSON.parse(options.body), product);
    return Response.json({ data: { id: 42, ...product } }, { status: 201, headers: { "X-Request-ID": "abc" } });
  });
  const result = await createClient("http://localhost:8080/", (value) => observed.push(value)).create(product);
  assert.equal(result.data.id, 42);
  assert.equal(observed[0].requestId, "abc");
});

test("DELETE handles a real 204 without trying to parse JSON", async (t) => {
  t.mock.method(globalThis, "fetch", async (url, options) => {
    assert.equal(url, "/products/42");
    assert.equal(options.method, "DELETE");
    assert.equal(options.body, undefined);
    assert.equal(options.headers["Content-Type"], undefined);
    return new Response(null, { status: 204 });
  });
  assert.equal(await createClient().remove(42), null);
});

test("HTTP validation errors retain field messages and request IDs", async (t) => {
  t.mock.method(globalThis, "fetch", async () => Response.json({
    error: { code: "validation_error", message: "Fix fields", details: { price: "Must be a number" }, request_id: "req-1" },
  }, { status: 422 }));
  await assert.rejects(createClient().create({ price: "1" }), (error) => {
    assert.ok(error instanceof APIError);
    assert.equal(error.status, 422);
    assert.equal(error.code, "validation_error");
    assert.equal(error.details.price, "Must be a number");
    assert.equal(error.requestId, "req-1");
    return true;
  });
});

test("list URL encodes filters and passes cancellation to fetch", async (t) => {
  const controller = new AbortController();
  t.mock.method(globalThis, "fetch", async (url, options) => {
    const parsed = new URL(url, "http://localhost");
    assert.equal(parsed.searchParams.get("q"), "pen & paper");
    assert.equal(parsed.searchParams.get("page"), "2");
    assert.equal(options.signal, controller.signal);
    return Response.json({ data: [], meta: { page: 2, total: 0 } });
  });
  const result = await createClient().list({ q: "pen & paper", page: 2 }, controller.signal);
  assert.deepEqual(result.data, []);
});

test("PATCH preserves explicit zero and empty values without inventing fields", async (t) => {
  t.mock.method(globalThis, "fetch", async (url, options) => {
    assert.equal(options.method, "PATCH");
    assert.deepEqual(JSON.parse(options.body), { price: 0, description: "" });
    return Response.json({ data: { id: 1, name: "Still here", price: 0, description: "" } });
  });
  assert.equal((await createClient().patch(1, { price: 0, description: "" })).data.name, "Still here");
});

test("network failures and cancellations propagate without retrying writes", async (t) => {
  for (const error of [new TypeError("Network failure"), new DOMException("Aborted", "AbortError")]) {
    const mock = t.mock.method(globalThis, "fetch", async () => { throw error; });
    await assert.rejects(createClient().create({ name: "Once", price: 1 }), (actual) => actual === error);
    assert.equal(mock.mock.callCount(), 1);
    mock.mock.restore();
  }
});
