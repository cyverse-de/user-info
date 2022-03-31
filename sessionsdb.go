package main

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/cyverse-de/queries"
)

// UserSessionRecord represents a user session stored in the database
type UserSessionRecord struct {
	ID      string
	Session string
	UserID  string
}

// convert makes sure that the JSON has the correct format. "wrap" tells convert
// whether to wrap the object in a map with "session" as the key.
func convertSessions(record *UserSessionRecord, wrap bool) (map[string]interface{}, error) {
	var values map[string]interface{}

	if record.Session != "" {
		if err := json.Unmarshal([]byte(record.Session), &values); err != nil {
			return nil, err
		}
	}

	// We don't want the return value wrapped in a session object, so unwrap it
	// if it is wrapped.
	if !wrap {
		if _, ok := values["session"]; ok {
			return values["session"].(map[string]interface{}), nil
		}
		return values, nil
	}

	// We do want the return value wrapped in a session object, so wrap it if it
	// isn't already.
	if _, ok := values["session"]; !ok {
		newmap := make(map[string]interface{})
		newmap["session"] = values
		return newmap, nil
	}

	return values, nil
}

type sDB interface {
	isUser(ctx context.Context, username string) (bool, error)

	// DB defines the interface for interacting with the user-sessions database.
	hasSessions(ctx context.Context, username string) (bool, error)
	getSessions(ctx context.Context, username string) ([]UserSessionRecord, error)
	insertSession(ctx context.Context, username, session string) error
	updateSession(ctx context.Context, username, session string) error
	deleteSession(ctx context.Context, username string) error
}

// SessionsDB handles interacting with the sessions database.
type SessionsDB struct {
	db *sql.DB
}

// NewSessionsDB returns a newly created *SessionsDB
func NewSessionsDB(db *sql.DB) *SessionsDB {
	return &SessionsDB{
		db: db,
	}
}

// isUser returnes whether or not the user is present in the sessions database.
func (s *SessionsDB) isUser(ctx context.Context, username string) (bool, error) {
	return queries.IsUser(ctx, s.db, username)
}

// hasSessions returns whether or not the given user has a session already.
func (s *SessionsDB) hasSessions(ctx context.Context, username string) (bool, error) {
	query := `SELECT COUNT(s.*)
              FROM user_sessions s,
                   users u
             WHERE s.user_id = u.id
               AND u.username = $1`
	var count int64
	if err := s.db.QueryRowContext(ctx, query, username).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// getSessions returns a []UserSessionRecord of all of the sessions associated
// with the provided username.
func (s *SessionsDB) getSessions(ctx context.Context, username string) ([]UserSessionRecord, error) {
	query := `SELECT s.id AS id,
                   s.user_id AS user_id,
                   s.session AS session
              FROM user_sessions s,
                   users u
             WHERE s.user_id = u.id
               AND u.username = $1`

	rows, err := s.db.QueryContext(ctx, query, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []UserSessionRecord
	for rows.Next() {
		var session UserSessionRecord
		if err := rows.Scan(&session.ID, &session.UserID, &session.Session); err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}

	if err := rows.Err(); err != nil {
		return sessions, err
	}

	return sessions, nil
}

// insertSession adds a new session to the database for the user.
func (s *SessionsDB) insertSession(ctx context.Context, username, session string) error {
	query := `INSERT INTO user_sessions (user_id, session)
                 VALUES ($1, $2)`
	userID, err := queries.UserID(ctx, s.db, username)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, query, userID, session)
	return err
}

// updateSession updates the session in the database for the user.
func (s *SessionsDB) updateSession(ctx context.Context, username, session string) error {
	query := `UPDATE ONLY user_sessions
                    SET session = $2
                  WHERE user_id = $1`
	userID, err := queries.UserID(ctx, s.db, username)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, query, userID, session)
	return err
}

// deleteSession deletes the user's session from the database.
func (s *SessionsDB) deleteSession(ctx context.Context, username string) error {
	query := `DELETE FROM ONLY user_sessions WHERE user_id = $1`
	userID, err := queries.UserID(ctx, s.db, username)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, query, userID)
	return err
}
