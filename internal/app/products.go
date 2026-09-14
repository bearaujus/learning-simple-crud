package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Product struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Price       float64 `json:"price"`
	Description string  `json:"description"`
}

// Pointers distinguish an omitted PATCH field from an explicit zero or empty string.
type productInput struct {
	Name        *string
	Price       *float64
	Description *string
}

const maxBodyBytes = 64 * 1024

func readProduct(w http.ResponseWriter, r *http.Request, partial bool) (productInput, bool) {
	var input productInput
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		sendError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Send a JSON body with Content-Type: application/json.", nil)
		return input, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			sendError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "The request body must be 64 KiB or smaller.", nil)
		} else {
			sendError(w, http.StatusBadRequest, "invalid_json", "Unable to read the JSON request body.", nil)
		}
		return input, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || fields == nil {
		sendError(w, http.StatusBadRequest, "invalid_json", "The body must contain exactly one valid JSON object.", nil)
		return input, false
	}
	details := map[string]string{}
	for key, raw := range fields {
		if key != "name" && key != "price" && key != "description" {
			details[key] = "Unknown field. Allowed fields: name, price, description."
			continue
		}
		if string(raw) == "null" {
			details[key] = "Must not be null. Omit a field in PATCH to keep its current value."
			continue
		}
		switch key {
		case "name", "description":
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				details[key] = "Must be a string."
				continue
			}
			value = strings.TrimSpace(value)
			length := utf8.RuneCountInString(value)
			if key == "name" {
				input.Name = &value
				if length < 1 || length > 100 {
					details[key] = "Must contain between 1 and 100 characters after trimming spaces."
				}
			} else {
				input.Description = &value
				if length > 1000 {
					details[key] = "Must contain at most 1000 characters."
				}
			}
		case "price":
			var value float64
			if err := json.Unmarshal(raw, &value); err != nil {
				details[key] = "Must be a JSON number, not a string."
				continue
			}
			input.Price = &value
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1000000 {
				details[key] = "Must be between 0 and 1000000."
			} else if value != math.Round(value*100)/100 {
				details[key] = "Must have at most two decimal places."
			} else {
				value = math.Round(value*100) / 100
			}
		}
	}
	if !partial {
		for _, key := range []string{"name", "price"} {
			if _, ok := fields[key]; !ok {
				details[key] = "This field is required."
			}
		}
		if input.Description == nil {
			empty := ""
			input.Description = &empty
		}
	} else if len(fields) == 0 {
		details["body"] = "Include at least one of: name, price, description."
	}
	if len(details) > 0 {
		sendError(w, http.StatusUnprocessableEntity, "validation_error", "Please fix the highlighted fields.", details)
		return input, false
	}
	return input, true
}

func productID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		sendError(w, http.StatusBadRequest, "invalid_id", "Product ID must be a positive integer.", nil)
		return 0, false
	}
	return id, true
}

// COALESCE also supports nullable descriptions in databases created by the original app.
const productColumns = "id, name, price, COALESCE(description, '')"

func scanProduct(row *sql.Row) (Product, error) {
	var p Product
	err := row.Scan(&p.ID, &p.Name, &p.Price, &p.Description)
	return p, err
}

func productResult(w http.ResponseWriter, p Product, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		sendError(w, http.StatusNotFound, "product_not_found", "Product not found. It may have been deleted.", nil)
	} else if err != nil {
		internalError(w, err)
	} else {
		sendData(w, http.StatusOK, p)
	}
}

func (app *application) getProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := productID(w, r)
	if !ok {
		return
	}
	p, err := scanProduct(app.db.QueryRowContext(r.Context(), "SELECT "+productColumns+" FROM products WHERE id = ?", id))
	productResult(w, p, err)
}

func (app *application) createProduct(w http.ResponseWriter, r *http.Request) {
	input, ok := readProduct(w, r, false)
	if !ok {
		return
	}
	p, err := scanProduct(app.db.QueryRowContext(r.Context(),
		"INSERT INTO products (name, price, description) VALUES (?, ?, ?) RETURNING "+productColumns,
		input.Name, input.Price, input.Description))
	if err != nil {
		internalError(w, err)
		return
	}
	w.Header().Set("Location", "/products/"+strconv.FormatInt(p.ID, 10))
	sendData(w, http.StatusCreated, p)
}

func (app *application) replaceProduct(w http.ResponseWriter, r *http.Request) {
	app.updateProduct(w, r, false)
}

func (app *application) patchProduct(w http.ResponseWriter, r *http.Request) {
	app.updateProduct(w, r, true)
}

func (app *application) updateProduct(w http.ResponseWriter, r *http.Request, partial bool) {
	id, ok := productID(w, r)
	if !ok {
		return
	}
	input, ok := readProduct(w, r, partial)
	if !ok {
		return
	}
	// One atomic statement prevents a PATCH from overwriting unrelated concurrent edits.
	p, err := scanProduct(app.db.QueryRowContext(r.Context(),
		`UPDATE products SET name = COALESCE(?, name), price = COALESCE(?, price),
		description = COALESCE(?, description) WHERE id = ? RETURNING `+productColumns,
		input.Name, input.Price, input.Description, id))
	productResult(w, p, err)
}

func (app *application) deleteProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := productID(w, r)
	if !ok {
		return
	}
	result, err := app.db.ExecContext(r.Context(), "DELETE FROM products WHERE id = ?", id)
	if err != nil {
		internalError(w, err)
		return
	}
	count, err := result.RowsAffected()
	if err != nil {
		internalError(w, err)
		return
	}
	if count == 0 {
		sendError(w, http.StatusNotFound, "product_not_found", "Product not found. It may have been deleted.", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type pagination struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

func listOptions(values url.Values) (pagination, string, string, map[string]string) {
	meta := pagination{Page: 1, Limit: 10}
	details := map[string]string{}
	for key, entries := range values {
		if key != "page" && key != "limit" && key != "q" && key != "sort" && key != "order" {
			details[key] = "Unknown parameter. Allowed parameters: page, limit, q, sort, order."
		} else if len(entries) != 1 {
			details[key] = "Supply this parameter only once."
		}
	}
	for _, field := range []struct {
		key    string
		target *int
		max    int
	}{{"page", &meta.Page, 1000000}, {"limit", &meta.Limit, 100}} {
		if values.Has(field.key) {
			n, err := strconv.Atoi(values.Get(field.key))
			if err != nil || n < 1 || n > field.max {
				details[field.key] = "Must be an integer between 1 and " + strconv.Itoa(field.max) + "."
			} else {
				*field.target = n
			}
		}
	}
	query := strings.TrimSpace(values.Get("q"))
	if utf8.RuneCountInString(query) > 100 {
		details["q"] = "Must contain at most 100 characters."
	}
	// Only these fixed SQL fragments can enter ORDER BY; user text is never interpolated.
	sorts := map[string]string{"id": "id", "name": "name COLLATE NOCASE", "price": "price"}
	sortKey := "id"
	if values.Has("sort") {
		sortKey = values.Get("sort")
	}
	column, ok := sorts[sortKey]
	if !ok {
		details["sort"] = "Must be one of: id, name, price."
	}
	order := "asc"
	if values.Has("order") {
		order = values.Get("order")
	}
	if order != "asc" && order != "desc" {
		details["order"] = "Must be one of: asc, desc."
	}
	return meta, query, column + " " + order + ", id ASC", details
}

func (app *application) listProducts(w http.ResponseWriter, r *http.Request) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		sendError(w, http.StatusBadRequest, "invalid_query", "Query parameters must be URL encoded.", nil)
		return
	}
	meta, query, orderBy, details := listOptions(values)
	if len(details) > 0 {
		sendError(w, http.StatusBadRequest, "invalid_query", "Please fix the query parameters.", details)
		return
	}
	// Count and rows share a snapshot, so pagination metadata describes this exact list.
	tx, err := app.db.BeginTx(r.Context(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		internalError(w, err)
		return
	}
	defer tx.Rollback()
	const where = " WHERE ? = '' OR instr(lower(name || ' ' || COALESCE(description, '')), lower(?)) > 0"
	if err := tx.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM products"+where, query, query).Scan(&meta.Total); err != nil {
		internalError(w, err)
		return
	}
	rows, err := tx.QueryContext(r.Context(), "SELECT "+productColumns+" FROM products"+where+" ORDER BY "+orderBy+" LIMIT ? OFFSET ?",
		query, query, meta.Limit, (meta.Page-1)*meta.Limit)
	if err != nil {
		internalError(w, err)
		return
	}
	defer rows.Close()
	products := []Product{}
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Price, &p.Description); err != nil {
			internalError(w, err)
			return
		}
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		internalError(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		internalError(w, err)
		return
	}
	meta.TotalPages = (meta.Total + meta.Limit - 1) / meta.Limit
	sendJSON(w, http.StatusOK, struct {
		Data []Product  `json:"data"`
		Meta pagination `json:"meta"`
	}{Data: products, Meta: meta})
}
