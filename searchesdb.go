package main

import (
	"context"
	"database/sql"

	"github.com/cyverse-de/queries"
)

// seDB defines the interface for interacting with storage. Mostly included
// to make unit tests easier to write.
type seDB interface {
	isUser(context.Context, string) (bool, error)
	hasSavedSearches(context.Context, string) (bool, error)
	getSavedSearches(context.Context, string) ([]string, error)
	insertSavedSearches(context.Context, string, string) error
	updateSavedSearches(context.Context, string, string) error
	deleteSavedSearches(context.Context, string) error
}

// SearchesDB implements the DB interface for interacting with the saved-searches
// database.
type SearchesDB struct {
	db *sql.DB
}

// NewSearchesDB returns a new *SearchesDB.
func NewSearchesDB(db *sql.DB) *SearchesDB {
	return &SearchesDB{
		db: db,
	}
}

// isUser returns whether or not the user exists in the saved searches database.
func (se *SearchesDB) isUser(ctx context.Context, username string) (bool, error) {
	return queries.IsUser(ctx, se.db, username)
}

// hasSavedSearches returns whether or not the given user has saved searches already.
func (se *SearchesDB) hasSavedSearches(ctx context.Context, username string) (bool, error) {
	var (
		err    error
		exists bool
	)

	query := `SELECT EXISTS(
              SELECT 1
                FROM user_saved_searches s,
                     users u
               WHERE s.user_id = u.id
                 AND u.username = $1) AS exists`

	if err = se.db.QueryRowContext(ctx, query, username).Scan(&exists); err != nil {
		return false, err
	}

	return exists, nil
}

// getSavedSearches returns all of the saved searches associated with the
// provided username.
func (se *SearchesDB) getSavedSearches(ctx context.Context, username string) ([]string, error) {
	var (
		err    error
		retval []string
		rows   *sql.Rows
	)

	query := `SELECT s.saved_searches saved_searches
              FROM user_saved_searches s,
                   users u
             WHERE s.user_id = u.id
               AND u.username = $1`

	if rows, err = se.db.QueryContext(ctx, query, username); err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var search string
		if err = rows.Scan(&search); err != nil {
			return nil, err
		}
		retval = append(retval, search)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return retval, nil
}

// insertSavedSearches adds new saved searches to the database for the user.
func (se *SearchesDB) insertSavedSearches(ctx context.Context, username, searches string) error {
	var (
		err    error
		userID string
	)

	query := `INSERT INTO user_saved_searches (user_id, saved_searches) VALUES ($1, $2)`

	if userID, err = queries.UserID(ctx, se.db, username); err != nil {
		return err
	}

	_, err = se.db.ExecContext(ctx, query, userID, searches)
	return err
}

// updateSavedSearches updates the saved searches in the database for the user.
func (se *SearchesDB) updateSavedSearches(ctx context.Context, username, searches string) error {
	var (
		err    error
		userID string
	)

	query := `UPDATE ONLY user_saved_searches SET saved_searches = $2 WHERE user_id = $1`

	if userID, err = queries.UserID(ctx, se.db, username); err != nil {
		return err
	}

	_, err = se.db.ExecContext(ctx, query, userID, searches)
	return err
}

// deleteSavedSearches removes the user's saved sessions from the database.
func (se *SearchesDB) deleteSavedSearches(ctx context.Context, username string) error {
	var (
		err    error
		userID string
	)

	query := `DELETE FROM ONLY user_saved_searches WHERE user_id = $1`

	if userID, err = queries.UserID(ctx, se.db, username); err != nil {
		return nil
	}

	_, err = se.db.ExecContext(ctx, query, userID)
	return err
}
