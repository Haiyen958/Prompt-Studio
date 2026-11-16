package storage

// Collections (the system "Chưa phân loại" collection is protected).

import (
	"database/sql"
	"errors"
	"strings"
)

func (r *Repo) ListCollections() ([]Collection, int, error) {
	rows, err := r.db.Query(`
		SELECT c.id, c.name, c.is_system, c.sort_order,
		       COUNT(s.id) AS cnt
		FROM collections c
		LEFT JOIN sessions s ON s.collection_id = c.id AND s.saved_at IS NOT NULL
		GROUP BY c.id
		ORDER BY c.is_system ASC, c.sort_order ASC, c.name COLLATE NOCASE ASC
	`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var cols []Collection
	totalSaved := 0
	for rows.Next() {
		var c Collection
		var sysInt int
		if err := rows.Scan(&c.ID, &c.Name, &sysInt, &c.SortOrder, &c.Count); err != nil {
			return nil, 0, err
		}
		c.IsSystem = sysInt != 0
		totalSaved += c.Count
		cols = append(cols, c)
	}
	return cols, totalSaved, rows.Err()
}

func (r *Repo) CreateCollection(name string) (*Collection, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, errors.New("tên collection không được để trống")
	}
	id := NewID()
	var maxOrder int
	_ = r.db.QueryRow(`SELECT COALESCE(MAX(sort_order), 0) + 1 FROM collections WHERE is_system = 0`).Scan(&maxOrder)

	_, err := r.db.Exec(
		`INSERT INTO collections (id, name, is_system, sort_order) VALUES (?, ?, 0, ?)`,
		id, trimmed, maxOrder,
	)
	if err != nil {
		return nil, err
	}
	return &Collection{
		ID:        id,
		Name:      trimmed,
		IsSystem:  false,
		SortOrder: maxOrder,
		Count:     0,
	}, nil
}

func (r *Repo) RenameCollection(id, name string) (*Collection, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, errors.New("tên collection không được để trống")
	}
	var sysInt int
	if err := r.db.QueryRow(`SELECT is_system FROM collections WHERE id = ?`, id).Scan(&sysInt); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	if sysInt != 0 || id == SystemCollectionID {
		return nil, ErrProtectedResource
	}

	if _, err := r.db.Exec(`UPDATE collections SET name = ? WHERE id = ?`, trimmed, id); err != nil {
		return nil, err
	}
	cols, _, err := r.ListCollections()
	if err != nil {
		return nil, err
	}
	for _, c := range cols {
		if c.ID == id {
			return &c, nil
		}
	}
	return nil, ErrNotFound
}

func (r *Repo) DeleteCollection(id string) error {
	var sysInt int
	if err := r.db.QueryRow(`SELECT is_system FROM collections WHERE id = ?`, id).Scan(&sysInt); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if sysInt != 0 || id == SystemCollectionID {
		return ErrProtectedResource
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`UPDATE sessions SET collection_id = ? WHERE collection_id = ?`, SystemCollectionID, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE settings SET value = ? WHERE key = 'last_collection_id' AND value = ?`, SystemCollectionID, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM collections WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}
