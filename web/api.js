// Copy this module into your own frontend. It has no framework dependencies.
export class APIError extends Error {
  constructor(status, error = {}) {
    super(error.message || `Request failed (${status}).`);
    this.name = "APIError";
    this.status = status;
    this.code = error.code || "unknown_error";
    this.details = error.details || {};
    this.requestId = error.request_id;
  }
}

export function createClient(baseURL = "", onResponse = () => {}) {
  async function request(path, { method = "GET", body, signal } = {}) {
    const response = await fetch(`${baseURL.replace(/\/$/, "")}${path}`, {
      method,
      signal,
      headers: body === undefined ? { Accept: "application/json" } : {
        Accept: "application/json",
        "Content-Type": "application/json",
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    // DELETE returns 204 with no body. Calling response.json() would throw.
    const payload = response.status === 204 ? null : await response.json();
    onResponse({ method, path, requestBody: body, status: response.status, body: payload,
      requestId: response.headers.get("X-Request-ID") });
    // fetch resolves for HTTP errors too; always check response.ok.
    if (!response.ok) throw new APIError(response.status, payload?.error);
    return payload;
  }

  const productPath = (id) => `/products/${encodeURIComponent(id)}`;
  return {
    list: (params = {}, signal) => request(`/products?${new URLSearchParams(params)}`, { signal }),
    get: (id, signal) => request(productPath(id), { signal }),
    create: (product) => request("/products", { method: "POST", body: product }),
    replace: (id, product) => request(productPath(id), { method: "PUT", body: product }),
    patch: (id, changes) => request(productPath(id), { method: "PATCH", body: changes }),
    remove: (id) => request(productPath(id), { method: "DELETE" }),
  };
}
