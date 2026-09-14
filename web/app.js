import { APIError, createClient } from "./api.js";

const $ = (id) => document.getElementById(id);
$("api-url").textContent = `${location.origin}/products`;
const form = $("product-form");
const history = [];
let selectedRequest;
let editingID = null;
let busy = false;
let loading = false;
let controller;
let page = 1;
let query = "";
let sort = "id";
let order = "asc";
let meta = { total: 0, total_pages: 0 };

// Render server-provided text with textContent, never innerHTML.
function element(tag, text, className) {
  const node = document.createElement(tag);
  node.textContent = text;
  if (className) node.className = className;
  return node;
}

function inspect(entry) {
  selectedRequest = entry;
  $("response-status").textContent = `${entry.status} · ${entry.method} ${entry.path} · request ${entry.requestId}`;
  $("response-json").textContent = entry.body === null
    ? "204 No Content — success, with no response body."
    : JSON.stringify(entry.body, null, 2);
  const options = { method: entry.method };
  if (entry.requestBody !== undefined) options.headers = { "Content-Type": "application/json" };
  let optionsCode = JSON.stringify(options, null, 2);
  if (entry.requestBody !== undefined) {
    optionsCode = optionsCode.slice(0, -2) + `,\n  body: JSON.stringify(${JSON.stringify(entry.requestBody, null, 2)})\n}`;
  }
  $("fetch-example").textContent = `const response = await fetch(${JSON.stringify(location.origin + entry.path)}, ${optionsCode});\nconst payload = response.status === 204 ? null : await response.json();\nif (!response.ok) throw new Error(payload.error.message);\nconsole.log(payload);`;
  renderHistory();
}

function renderHistory() {
  $("history").replaceChildren(...history.map((entry) => {
    const button = element("button", `${entry.status}  ${entry.method} ${entry.path}`);
    button.type = "button";
    button.setAttribute("aria-pressed", String(entry === selectedRequest));
    button.addEventListener("click", () => inspect(entry));
    return button;
  }));
}

const api = createClient("", (entry) => {
  history.unshift(entry);
  history.splice(8);
  inspect(entry);
});

function notice(message, isError = false) {
  $("notice").textContent = message;
  $("notice").dataset.error = String(isError);
}

function clearErrors() {
  for (const field of ["name", "price", "description"]) {
    $(`${field}-error`).textContent = "";
    $(field).removeAttribute("aria-invalid");
  }
}

function showError(error) {
  if (error.name === "AbortError") return;
  if (error instanceof APIError) {
    notice(`${error.message} (${error.status} · ${error.code})`, true);
    for (const field of ["name", "price", "description"]) {
      if (error.details[field]) {
        $(`${field}-error`).textContent = error.details[field];
        $(field).setAttribute("aria-invalid", "true");
      }
    }
  } else {
    notice("Could not reach the API. Check that the server is running, then try Refresh. If a save was interrupted, check the list before submitting again.", true);
  }
}

function updateControls() {
  $("product-fields").disabled = busy;
  for (const id of ["refresh", "try-error", "search", "sort"]) $(id).disabled = busy;
  $("search-form").querySelector("button").disabled = busy;
  for (const button of $("products").querySelectorAll("button")) button.disabled = busy || loading;
  $("previous").disabled = busy || loading || page <= 1;
  $("next").disabled = busy || loading || page >= meta.total_pages;
  $("products").setAttribute("aria-busy", String(loading));
}

// Disable mutations while one is in flight, and always restore controls in finally.
async function perform(action) {
  if (busy) return;
  busy = true;
  clearErrors();
  updateControls();
  try {
    await action();
  } catch (error) {
    showError(error);
  } finally {
    busy = false;
    updateControls();
  }
}

function resetForm() {
  editingID = null;
  form.reset();
  clearErrors();
  $("form-title").textContent = "Create a product";
  $("form-method").textContent = "POST /products";
  $("save").textContent = "Create product";
  $("cancel").hidden = true;
}

function productButton(label, action, danger = false) {
  const button = element("button", label, danger ? "secondary danger" : "secondary");
  button.type = "button";
  button.addEventListener("click", () => perform(action));
  return button;
}

function renderProducts(products) {
  if (!products.length) {
    $("products").replaceChildren(element("p", query
      ? "No matches. Try another search or clear the search box."
      : "Your collection starts here. Create your first product using the form.", "empty"));
    return;
  }
  $("products").replaceChildren(...products.map((product) => {
    const row = element("article", "", "product");
    const copy = element("div", "", "product-copy");
    const info = element("div", "", "product-meta");
    info.append(element("span", `#${product.id}`), element("span", product.price.toFixed(2), "product-price"));
    copy.append(info, element("h3", product.name), element("p", product.description || "No description"));
    const actions = element("div", "", "product-actions");
    actions.append(
      productButton("View JSON", async () => {
        await api.get(product.id);
        notice(`Loaded product #${product.id}. See the response below.`);
      }),
      productButton("Edit", async () => {
        const { data } = await api.get(product.id);
        editingID = data.id;
        for (const field of ["name", "price", "description"]) $(field).value = data[field];
        $("form-title").textContent = `Edit product #${data.id}`;
        $("form-method").textContent = `PUT /products/${data.id}`;
        $("save").textContent = "Save product";
        $("cancel").hidden = false;
        notice("Edit the fields, then save to replace the product.");
        // Focus after perform() re-enables the fieldset.
        requestAnimationFrame(() => $("name").focus());
      }),
      productButton("Make free", async () => {
        await api.patch(product.id, { price: 0 });
        notice(`PATCH changed only the price of product #${product.id}.`);
        if (editingID === product.id) resetForm();
        await loadProducts();
      }),
      productButton("Delete", async () => {
        if (!window.confirm(`Delete “${product.name}”? This cannot be undone.`)) return;
        await api.remove(product.id);
        notice(`Product #${product.id} deleted. The API returned 204 with no body.`);
        if (editingID === product.id) resetForm();
        await loadProducts();
      }, true),
    );
    row.append(copy, actions);
    return row;
  }));
}

async function loadProducts() {
  controller?.abort();
  const current = new AbortController();
  controller = current;
  loading = true;
  updateControls();
  try {
    const result = await api.list({ page, limit: 5, q: query, sort, order }, current.signal);
    if (current !== controller) return;
    meta = result.meta;
    // Deleting the last row of the final page should show the previous page.
    if (page > 1 && page > Math.max(1, meta.total_pages)) {
      page = Math.max(1, meta.total_pages);
      return await loadProducts();
    }
    renderProducts(result.data);
    $("total").textContent = meta.total;
    $("page-label").textContent = meta.total === 0 ? "0 products" : `Page ${page} of ${meta.total_pages}`;
    if ($("notice").textContent === "Refreshing products…") notice("Products refreshed.");
  } catch (error) {
    if (current !== controller) return;
    showError(error);
    if (!$("products").querySelector("article")) {
      $("products").replaceChildren(element("p", "Products could not be loaded. Try Refresh.", "empty"));
    }
  } finally {
    if (current === controller) {
      loading = false;
      updateControls();
    }
  }
}

form.addEventListener("submit", (event) => {
  event.preventDefault();
  perform(async () => {
    const product = { name: $("name").value, price: Number($("price").value), description: $("description").value };
    const { data } = editingID === null ? await api.create(product) : await api.replace(editingID, product);
    notice(`Saved product #${data.id}. Select its request below to inspect the response.`);
    resetForm();
    // Show the saved item even if the previous list was filtered or on another page.
    query = "";
    sort = "id";
    order = "desc";
    page = 1;
    $("search").value = "";
    $("sort").value = "id:desc";
    await loadProducts();
  });
});
$("cancel").addEventListener("click", resetForm);
$("refresh").addEventListener("click", () => { notice("Refreshing products…"); loadProducts(); });
$("search-form").addEventListener("submit", (event) => {
  event.preventDefault();
  query = $("search").value.trim();
  [sort, order] = $("sort").value.split(":");
  page = 1;
  notice("");
  loadProducts();
});
$("previous").addEventListener("click", () => { page--; loadProducts(); });
$("next").addEventListener("click", () => { page++; loadProducts(); });
$("try-error").addEventListener("click", () => perform(async () => {
  await api.create({ name: "", price: -1 });
}));
loadProducts();
