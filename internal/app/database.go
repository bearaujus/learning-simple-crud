package app

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// OpenDatabase opens SQLite and initializes the products table if needed.
func OpenDatabase(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// One connection keeps this small SQLite app simple and serializes writes.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	_, err = db.Exec(`
		PRAGMA busy_timeout = 5000;
		CREATE TABLE IF NOT EXISTS products (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			price REAL NOT NULL,
			description TEXT
		);`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	return db, nil
}

// SeedProducts adds sample products only when the database is empty.
func SeedProducts(db *sql.DB) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRow("SELECT COUNT(*) FROM products").Scan(&count); err != nil {
		return 0, err
	}
	if count > 0 {
		return 0, nil
	}
	products := []Product{
		{Name: "Notebook", Price: 4.5, Description: "A dotted notebook for new ideas."},
		{Name: "Desk lamp", Price: 29.99, Description: "A little light for late-night learning."},
		{Name: "Canvas tote", Price: 12, Description: "Room for your laptop and a good book."},
		{Name: "Water bottle", Price: 18.5, Description: "Stay hydrated while you code."},
		{Name: "Keyboard", Price: 49.95, Description: "A compact keyboard for your workspace."},
		{Name: "Sticker pack", Price: 0, Description: "Free stickers. Zero is a valid price!"},
	}
	for _, p := range products {
		if _, err := tx.Exec("INSERT INTO products (name, price, description) VALUES (?, ?, ?)", p.Name, p.Price, p.Description); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(products), nil
}
