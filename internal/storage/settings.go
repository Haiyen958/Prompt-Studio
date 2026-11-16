package storage

// Settings table and the API key (stored in the secrets file, never in SQLite).

func (r *Repo) GetSettings() (map[string]string, error) {
	rows, err := r.db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (r *Repo) SaveSettings(kv map[string]string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for k, v := range kv {
		if _, err := tx.Exec(
			`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			k, v,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repo) GetAPIKey() (string, string, bool, error) {
	if r.secrets == nil {
		return "", "", false, nil
	}
	return r.secrets.Get()
}

func (r *Repo) SetAPIKey(apiKey string) error {
	if r.secrets == nil {
		return nil
	}
	return r.secrets.Set(apiKey)
}

func (r *Repo) DeleteAPIKey() error {
	if r.secrets == nil {
		return nil
	}
	return r.secrets.Delete()
}
