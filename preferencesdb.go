package main

import (
	"context"
	"database/sql"

	"github.com/cyverse-de/queries"
)

type pDB interface {
	isUser(ctx context.Context, username string) (bool, error)

	// DB defines the interface for interacting with the user-prefs database.
	hasPreferences(ctx context.Context, username string) (bool, error)
	getPreferences(ctx context.Context, username string) ([]UserPreferencesRecord, error)
	insertPreferences(ctx context.Context, username, prefs string) error
	updatePreferences(ctx context.Context, username, prefs string) error
	deletePreferences(ctx context.Context, username string) error
}

// PrefsDB implements the DB interface for interacting with the user-preferences
// database.
type PrefsDB struct {
	db *sql.DB
}

// NewPrefsDB returns a newly created *PrefsDB.
func NewPrefsDB(db *sql.DB) *PrefsDB {
	return &PrefsDB{
		db: db,
	}
}

// isUser returns whether or not the user exists in the database preferences.
func (p *PrefsDB) isUser(ctx context.Context, username string) (bool, error) {
	return queries.IsUser(ctx, p.db, username)
}

// hasPreferences returns whether or not the given user has preferences already.
func (p *PrefsDB) hasPreferences(ctx context.Context, username string) (bool, error) {
	query := `SELECT COUNT(p.*)
              FROM user_preferences p,
                   users u
             WHERE p.user_id = u.id
               AND u.username = $1`
	var count int64
	if err := p.db.QueryRowContext(ctx, query, username).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// getPreferences returns a []UserPreferencesRecord of all of the preferences associated
// with the provided username.
func (p *PrefsDB) getPreferences(ctx context.Context, username string) ([]UserPreferencesRecord, error) {
	query := `SELECT p.id AS id,
                   p.user_id AS user_id,
                   p.preferences AS preferences
              FROM user_preferences p,
                   users u
             WHERE p.user_id = u.id
               AND u.username = $1`

	rows, err := p.db.QueryContext(ctx, query, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prefs []UserPreferencesRecord
	for rows.Next() {
		var pref UserPreferencesRecord
		if err := rows.Scan(&pref.ID, &pref.UserID, &pref.Preferences); err != nil {
			return nil, err
		}
		prefs = append(prefs, pref)
	}

	if err := rows.Err(); err != nil {
		return prefs, err
	}

	return prefs, nil
}

func (p *PrefsDB) mutation(ctx context.Context, query, username string, args ...interface{}) error {
	userID, err := queries.UserID(ctx, p.db, username)
	if err != nil {
		return err
	}
	allargs := append([]interface{}{userID}, args...)
	_, err = p.db.ExecContext(ctx, query, allargs...)
	return err
}

// insertPreferences adds new preferences to the database for the user.
func (p *PrefsDB) insertPreferences(ctx context.Context, username, prefs string) error {
	query := `INSERT INTO user_preferences (user_id, preferences)
                 VALUES ($1, $2)`
	return p.mutation(ctx, query, username, prefs)
}

// updatePreferences updates the preferences in the database for the user.
func (p *PrefsDB) updatePreferences(ctx context.Context, username, prefs string) error {
	query := `UPDATE ONLY user_preferences
                    SET preferences = $2
                  WHERE user_id = $1`
	return p.mutation(ctx, query, username, prefs)
}

// deletePreferences deletes the user's preferences from the database.
func (p *PrefsDB) deletePreferences(ctx context.Context, username string) error {
	query := `DELETE FROM ONLY user_preferences WHERE user_id = $1`
	return p.mutation(ctx, query, username)
}
