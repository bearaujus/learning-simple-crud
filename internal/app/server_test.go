package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func testApp(t *testing.T) (*sql.DB, http.Handler) {
	t.Helper()
	db, err := OpenDatabase(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, NewHandler(db, []string{"*"})
}

func request(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func expectStatus(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status = %d, want %d: %s", w.Code, status, w.Body.String())
	}
	if w.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing request ID")
	}
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
		t.Fatalf("invalid JSON response: %v: %s", err, w.Body.String())
	}
	return value
}

func responseProduct(t *testing.T, w *httptest.ResponseRecorder) Product {
	t.Helper()
	return decode[struct{ Data Product }](t, w).Data
}

func expectError(t *testing.T, w *httptest.ResponseRecorder, status int, code string) apiError {
	t.Helper()
	expectStatus(t, w, status)
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatal("error should be JSON")
	}
	value := decode[struct{ Error apiError }](t, w).Error
	if value.Code != code || value.Message == "" || value.RequestID != w.Header().Get("X-Request-ID") {
		t.Fatalf("unexpected error envelope: %+v", value)
	}
	return value
}

func TestProductLifecycle(t *testing.T) {
	_, handler := testApp(t)
	created := request(handler, "POST", "/products", `{"name":"  Notebook  ","price":12.50,"description":" Dotted "}`)
	expectStatus(t, created, 201)
	p := responseProduct(t, created)
	if p.ID <= 0 || p.Name != "Notebook" || p.Price != 12.5 || p.Description != "Dotted" {
		t.Fatalf("unexpected created product: %+v", p)
	}
	path := fmt.Sprintf("/products/%d", p.ID)
	if created.Header().Get("Location") != path {
		t.Fatal("Location should identify the created product")
	}
	got := request(handler, "GET", path, "")
	expectStatus(t, got, 200)
	if responseProduct(t, got) != p {
		t.Fatal("created product was not persisted")
	}

	patched := request(handler, "PATCH", path, `{"price":0,"description":""}`)
	expectStatus(t, patched, 200)
	p = responseProduct(t, patched)
	if p.Name != "Notebook" || p.Price != 0 || p.Description != "" {
		t.Fatalf("PATCH lost omitted fields or ignored zero values: %+v", p)
	}
	invalid := request(handler, "PUT", path, `{"price":8.99}`)
	expectError(t, invalid, 422, "validation_error")
	if responseProduct(t, request(handler, "GET", path, "")) != p {
		t.Fatal("failed validation changed the stored product")
	}
	replaced := request(handler, "PUT", path, `{"name":"Pen","price":2.99,"description":"Blue"}`)
	expectStatus(t, replaced, 200)
	if got := responseProduct(t, replaced); got.Name != "Pen" || got.Price != 2.99 || got.Description != "Blue" || got.ID != p.ID {
		t.Fatalf("unexpected PUT result: %+v", got)
	}
	replaced = request(handler, "PUT", path, `{"name":"Pen","price":2.99}`)
	expectStatus(t, replaced, 200)
	if responseProduct(t, replaced).Description != "" {
		t.Fatal("PUT should reset an omitted optional description")
	}
	deleted := request(handler, "DELETE", path, "")
	expectStatus(t, deleted, 204)
	if deleted.Body.Len() != 0 || deleted.Header().Get("Content-Type") != "" {
		t.Fatal("204 must have no JSON body or content type")
	}
	for _, method := range []string{"GET", "DELETE", "PUT", "PATCH"} {
		expectError(t, request(handler, method, path, `{"name":"Missing","price":0}`), 404, "product_not_found")
	}
}

func TestValidation(t *testing.T) {
	_, handler := testApp(t)
	cases := []struct {
		name, body, field string
	}{
		{"missing name", `{"price":0}`, "name"},
		{"missing price", `{"name":"Test"}`, "price"},
		{"blank name", `{"name":"  ","price":0}`, "name"},
		{"negative price", `{"name":"Test","price":-1}`, "price"},
		{"price too large", `{"name":"Test","price":1000000.01}`, "price"},
		{"precision", `{"name":"Test","price":1.001}`, "price"},
		{"small excess precision", `{"name":"Test","price":1.000000001}`, "price"},
		{"string price", `{"name":"Test","price":"1"}`, "price"},
		{"null price", `{"name":"Test","price":null}`, "price"},
		{"null name", `{"name":null,"price":1}`, "name"},
		{"null description", `{"name":"Test","price":1,"description":null}`, "description"},
		{"wrong name type", `{"name":123,"price":1}`, "name"},
		{"wrong description type", `{"name":"Test","price":1,"description":[]}`, "description"},
		{"unknown field", `{"name":"Test","price":1,"prize":2}`, "prize"},
		{"read-only ID", `{"name":"Test","price":1,"id":5}`, "id"},
		{"case sensitive fields", `{"Name":"Test","price":1}`, "Name"},
		{"long name", `{"name":"` + strings.Repeat("a", 101) + `","price":1}`, "name"},
		{"long description", `{"name":"Test","price":1,"description":"` + strings.Repeat("a", 1001) + `"}`, "description"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value := expectError(t, request(handler, "POST", "/products", tc.body), 422, "validation_error")
			if value.Details[tc.field] == "" {
				t.Fatalf("missing field error for %s: %+v", tc.field, value)
			}
		})
	}
	list := decode[struct{ Data []Product }](t, request(handler, "GET", "/products", ""))
	if len(list.Data) != 0 {
		t.Fatal("invalid requests wrote products")
	}
	for _, body := range []string{`null`, `[]`, `"text"`, `{"name":`, `{} {}`, `{"name":"Test",}`} {
		expectError(t, request(handler, "POST", "/products", body), 400, "invalid_json")
	}
	expectError(t, request(handler, "POST", "/products", strings.Repeat(" ", maxBodyBytes+1)), 413, "payload_too_large")
	expectError(t, request(handler, "POST", "/products", ""), 415, "unsupported_media_type")
	for _, mediaType := range []string{"application/json; charset=utf-8", "text/plain", "application/json; broken"} {
		r := httptest.NewRequest("POST", "/products", strings.NewReader(`{"name":"Free","price":0}`))
		r.Header.Set("Content-Type", mediaType)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if mediaType == "application/json; charset=utf-8" {
			expectStatus(t, w, 201)
		} else {
			expectError(t, w, 415, "unsupported_media_type")
		}
	}
	for _, body := range []string{`{}`, `{"price":null}`, `{"description":null}`} {
		expectError(t, request(handler, "PATCH", "/products/1", body), 422, "validation_error")
	}
	unicode := request(handler, "POST", "/products", `{"name":"`+strings.Repeat("猫", 100)+`","price":1000000}`)
	expectStatus(t, unicode, 201)
}

func TestListProducts(t *testing.T) {
	db, handler := testApp(t)
	empty := request(handler, "GET", "/products", "")
	expectStatus(t, empty, 200)
	if !strings.Contains(empty.Body.String(), `"data":[]`) {
		t.Fatal("empty lists must be arrays")
	}
	if _, err := SeedProducts(db); err != nil {
		t.Fatal(err)
	}
	type pageResponse struct {
		Data []Product
		Meta pagination
	}
	w := request(handler, "GET", "/products?page=2&limit=2&sort=price&order=asc", "")
	expectStatus(t, w, 200)
	list := decode[pageResponse](t, w)
	if list.Meta != (pagination{Page: 2, Limit: 2, Total: 6, TotalPages: 3}) || len(list.Data) != 2 || list.Data[0].Name != "Canvas tote" || list.Data[1].Name != "Water bottle" {
		t.Fatalf("unexpected page: %+v", list)
	}
	for _, query := range []string{"NOTEBOOK", "dotted"} {
		list = decode[pageResponse](t, request(handler, "GET", "/products?q="+query, ""))
		if list.Meta.Total != 1 || len(list.Data) != 1 || list.Data[0].Name != "Notebook" {
			t.Fatalf("search should find name and description: %+v", list)
		}
	}
	for _, query := range []string{"q=missing", "q=%25", "q=%27%20OR%201%3D1--", "page=1000000"} {
		w := request(handler, "GET", "/products?"+query, "")
		expectStatus(t, w, 200)
		if len(decode[pageResponse](t, w).Data) != 0 {
			t.Fatalf("expected empty list for %s", query)
		}
	}
	for _, query := range []string{"page=0", "page=-1", "page=1.5", "page=1000001", "limit=101", "limit=0", "limit=", "sort=price;DROP", "sort=", "order=DESC", "page=1&page=2", "foo=bar", "q=" + strings.Repeat("a", 101), "q=%ZZ", "q=x;y"} {
		expectError(t, request(handler, "GET", "/products?"+query, ""), 400, "invalid_query")
	}
	// Stable tie ordering avoids duplicates or skipped items between pages.
	for _, name := range []string{"A", "B", "C"} {
		expectStatus(t, request(handler, "POST", "/products", fmt.Sprintf(`{"name":%q,"price":1}`, name)), 201)
	}
	first := decode[pageResponse](t, request(handler, "GET", "/products?sort=price&limit=2&page=1", ""))
	second := decode[pageResponse](t, request(handler, "GET", "/products?sort=price&limit=2&page=2", ""))
	if first.Data[1].Name != "A" || second.Data[0].Name != "B" || second.Data[1].Name != "C" {
		t.Fatal("equal prices should be ordered by ascending ID")
	}
}

func TestRoutingAndCORS(t *testing.T) {
	db, handler := testApp(t)
	for _, method := range []string{"GET", "PUT", "PATCH", "DELETE"} {
		for _, id := range []string{"0", "-1", "abc", "9223372036854775808"} {
			expectError(t, request(handler, method, "/products/"+id, `{"name":"Test","price":1}`), 400, "invalid_id")
		}
	}
	for _, path := range []string{"/missing", "/products/1/extra"} {
		expectError(t, request(handler, "GET", path, ""), 404, "route_not_found")
	}
	w := request(handler, "PUT", "/products", `{}`)
	expectError(t, w, 405, "method_not_allowed")
	if w.Header().Get("Allow") != "GET, HEAD, OPTIONS, POST" {
		t.Fatalf("incorrect Allow: %s", w.Header().Get("Allow"))
	}
	for _, origins := range [][]string{{"*"}, {" http://localhost:5173 ", "http://localhost:3000"}, {"http://example.test"}} {
		handler := NewHandler(db, origins)
		r := httptest.NewRequest("OPTIONS", "/products/1", nil)
		r.Header.Set("Origin", "http://localhost:5173")
		r.Header.Set("Access-Control-Request-Method", "PATCH")
		r.Header.Set("Access-Control-Request-Headers", "content-type")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		expectStatus(t, w, 204)
		allowed := w.Header().Get("Access-Control-Allow-Origin")
		if origins[0] == "http://example.test" {
			if allowed != "" {
				t.Fatal("unexpected CORS permission")
			}
		} else if allowed == "" || !strings.Contains(w.Header().Get("Access-Control-Allow-Methods"), "PATCH") || !strings.Contains(w.Header().Get("Access-Control-Expose-Headers"), "Location") {
			t.Fatal("missing CORS headers")
		}
		if w.Body.Len() != 0 {
			t.Fatal("preflight should have no body")
		}
	}
	expectStatus(t, request(handler, "GET", "/health", ""), 200)
	for path, contentType := range map[string]string{"/": "text/html", "/docs": "text/html", "/api.js": "javascript", "/app.js": "javascript", "/styles.css": "text/css", "/openapi.json": "application/json"} {
		w := request(handler, "GET", path, "")
		expectStatus(t, w, 200)
		if !strings.Contains(w.Header().Get("Content-Type"), contentType) {
			t.Fatalf("unexpected content type for %s: %s", path, w.Header().Get("Content-Type"))
		}
	}
	// Exercise HEAD through an actual HTTP server; the transport suppresses its body.
	server := httptest.NewServer(handler)
	defer server.Close()
	response, err := http.Head(server.URL + "/products")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("HEAD should be supported")
	}
}

func TestInternalErrorsDoNotLeakDatabaseDetails(t *testing.T) {
	db, handler := testApp(t)
	db.Close()
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		path := "/products/1"
		if method == "POST" {
			path = "/products"
		}
		value := expectError(t, request(handler, method, path, `{"name":"Test","price":1}`), 500, "internal_error")
		if strings.Contains(value.Message, "sql") || strings.Contains(value.Message, "closed") || value.Details != nil {
			t.Fatalf("database error leaked: %+v", value)
		}
	}
	expectError(t, request(handler, "GET", "/products", ""), 500, "internal_error")
	expectError(t, request(handler, "GET", "/health", ""), 503, "service_unavailable")
}

func TestConcurrentPatchesKeepUnrelatedFields(t *testing.T) {
	_, handler := testApp(t)
	created := request(handler, "POST", "/products", `{"name":"Original","price":10}`)
	expectStatus(t, created, 201)
	path := created.Header().Get("Location")
	var wg sync.WaitGroup
	responses := make(chan *httptest.ResponseRecorder, 2)
	for _, body := range []string{`{"name":"Updated"}`, `{"description":"Also updated"}`} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			responses <- request(handler, "PATCH", path, body)
		}()
	}
	wg.Wait()
	close(responses)
	for response := range responses {
		expectStatus(t, response, 200)
	}
	p := responseProduct(t, request(handler, "GET", path, ""))
	if p.Name != "Updated" || p.Description != "Also updated" || p.Price != 10 {
		t.Fatalf("lost a concurrent change: %+v", p)
	}
}

func TestSeedingAndLegacyDatabase(t *testing.T) {
	db, handler := testApp(t)
	for _, want := range []int{6, 0} {
		count, err := SeedProducts(db)
		if err != nil || count != want {
			t.Fatalf("seed = %d, %v; want %d", count, err, want)
		}
	}
	expectStatus(t, request(handler, "PATCH", "/products/1", `{"name":"My edit"}`), 200)
	if _, err := SeedProducts(db); err != nil {
		t.Fatal(err)
	}
	if responseProduct(t, request(handler, "GET", "/products/1", "")).Name != "My edit" {
		t.Fatal("seeding overwrote existing data")
	}
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`CREATE TABLE products (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, price REAL NOT NULL, description TEXT);
		INSERT INTO products (name, price, description) VALUES ('Existing', 5, NULL)`)
	legacy.Close()
	if err != nil {
		t.Fatal(err)
	}
	opened, err := OpenDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	oldHandler := NewHandler(opened, []string{"*"})
	w := request(oldHandler, "GET", "/products/1", "")
	expectStatus(t, w, 200)
	if p := responseProduct(t, w); p.Name != "Existing" || p.Description != "" {
		t.Fatalf("legacy product not preserved: %+v", p)
	}
	expectStatus(t, request(oldHandler, "GET", "/products", ""), 200)
	if count, err := SeedProducts(opened); count != 0 || err != nil {
		t.Fatalf("seed should leave existing databases alone: %d, %v", count, err)
	}
}
