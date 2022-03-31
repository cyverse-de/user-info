package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cyverse-de/queries"
)

// BagsAPI provides an API for interacting with bags.
type BagsAPI struct {
	db *sql.DB
}

// BagRecord represents a bag as stored in the database.
type BagRecord struct {
	ID       string      `json:"id"`
	Contents BagContents `json:"contents"`
	UserID   string      `json:"user_id"`
}

// BagContents represents a bag's contents stored in the database.
type BagContents map[string]interface{}

// Value ensures that the BagContents type implements the driver.Valuer interface.
func (b BagContents) Value() (driver.Value, error) {
	return json.Marshal(b)
}

// Scan implements the sql.Scanner interface for *BagContents
func (b *BagContents) Scan(value interface{}) error {
	valueBytes, ok := value.([]byte) //make sure that value can be type asserted to a []byte.
	if !ok {
		return errors.New("failed to cast value to []byte")
	}
	return json.Unmarshal(valueBytes, &b)
}

// HasBags returns true if the user has bags and false otherwise.
func (b *BagsAPI) HasBags(ctx context.Context, username string) (bool, error) {
	query := `SELECT count(*)
				FROM bags b,
					 users u
			   WHERE b.user_id = u.id
				 AND u.username = $1`
	var count int64
	if err := b.db.QueryRowContext(ctx, query, username).Scan(&count); err != nil {
		return false, fmt.Errorf("error checking if %s has any bags: %w", username, err)
	}
	return count > 0, nil
}

// HasDefaultBag returns true if the user has a default bag.
func (b *BagsAPI) HasDefaultBag(ctx context.Context, username string) (bool, error) {
	query := `SELECT count(*)
				FROM default_bags d,
					 users u
			   WHERE d.user_id = u.id
				 AND u.username = $1`
	var count int64
	if err := b.db.QueryRowContext(ctx, query, username).Scan(&count); err != nil {
		return false, fmt.Errorf("error checking if %s has a default bag: %w", username, err)
	}
	return count > 0, nil

}

// HasBag returns true if the specified bag exists in the database.
func (b *BagsAPI) HasBag(ctx context.Context, username, bagID string) (bool, error) {
	query := `SELECT count(*)
				FROM bags b,
					 users u
			   WHERE b.user_id = u.id
				 AND u.username = $1
				 AND b.id = $2`
	var count int64
	if err := b.db.QueryRowContext(ctx, query, username, bagID).Scan(&count); err != nil {
		return false, fmt.Errorf("error checking for bag %s for %s: %w", bagID, username, err)
	}
	return count > 0, nil
}

// GetBags returns all of the bags for the provided user.
func (b *BagsAPI) GetBags(ctx context.Context, username string) ([]BagRecord, error) {
	query := `SELECT b.id,
					 b.contents,
					 b.user_id
				FROM bags b,
					 users u
			   WHERE b.user_id = u.id
				 AND u.username = $1`

	rows, err := b.db.QueryContext(ctx, query, username)
	if err != nil {
		return nil, fmt.Errorf("error getting all bags for %s: %w", username, err)
	}

	bagList := []BagRecord{}
	for rows.Next() {
		record := BagRecord{}
		err = rows.Scan(&record.ID, &record.Contents, &record.UserID)
		if err != nil {
			return nil, fmt.Errorf("error scanning record while getting bags for %s: %w", username, err)
		}

		bagList = append(bagList, record)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error from rows object while getting bags for %s: %w", username, err)
	}
	return bagList, nil
}

// GetBag returns the specified bag for the specified user according to the specified specifier for the
// bag record.
func (b *BagsAPI) GetBag(ctx context.Context, username, bagID string) (BagRecord, error) {
	query := `SELECT b.id,
					 b.contents,
					 b.user_id
				FROM bags b,
					 users u
			   WHERE b.user_id = u.id
				 AND u.username = $2
				 AND b.id = $1`
	var record BagRecord
	err := b.db.QueryRowContext(ctx, query, bagID, username).Scan(&record.ID, &record.Contents, &record.UserID)
	if err != nil {
		return record, fmt.Errorf("error getting bag id %s for %s: %w", bagID, username, err)
	}
	return record, nil

}

func (b *BagsAPI) createDefaultBag(ctx context.Context, username string) (BagRecord, error) {
	var (
		err         error
		record      BagRecord
		newBagID    string
		newContents []byte
		userID      string
	)
	defaultContents := map[string]interface{}{}
	record.Contents = defaultContents

	if newContents, err = json.Marshal(defaultContents); err != nil {
		return record, fmt.Errorf("error marshaling default bag: %w", err)
	}

	if newBagID, err = b.AddBag(ctx, username, string(newContents)); err != nil {
		return record, fmt.Errorf("error adding bag for user %s: %w", username, err)
	}

	record.ID = newBagID

	if err = b.SetDefaultBag(ctx, username, newBagID); err != nil {
		return record, fmt.Errorf("error setting the default bag for %s: %w", username, err)
	}

	if userID, err = queries.UserID(ctx, b.db, username); err != nil {
		return record, fmt.Errorf("error getting the user id for %s: %w", username, err)
	}

	record.UserID = userID

	return record, err
}

// GetDefaultBag returns the specified bag for the indicated user.
func (b *BagsAPI) GetDefaultBag(ctx context.Context, username string) (BagRecord, error) {
	var (
		err        error
		hasDefault bool
		record     BagRecord
	)

	// if the user doesn't have a default bag, add bag and set it as the default, then return it.
	if hasDefault, err = b.HasDefaultBag(ctx, username); err != nil {
		return record, fmt.Errorf("error from HasDefaultBag in GetDefaultBag for %s: %w", username, err)
	}

	if !hasDefault {
		return b.createDefaultBag(ctx, username)
	}

	query := `SELECT b.id,
					 b.contents,
					 b.user_id
				FROM bags b
				JOIN default_bags d ON b.id = d.bag_id
				JOIN users u ON d.user_id = u.id
			   WHERE u.username = $1`

	if err = b.db.QueryRowContext(ctx, query, username).Scan(&record.ID, &record.Contents, &record.UserID); err != nil {
		return record, fmt.Errorf("error getting default bag for %s from the database: %w", username, err)
	}

	return record, nil
}

// SetDefaultBag allows the user to update their default bag.
func (b *BagsAPI) SetDefaultBag(ctx context.Context, username, bagID string) error {
	var (
		err    error
		userID string
	)

	if userID, err = queries.UserID(ctx, b.db, username); err != nil {
		return fmt.Errorf("error getting user ID for %s while setting default bag: %w", username, err)
	}

	query := `INSERT INTO default_bags VALUES ( $1, $2 ) ON CONFLICT (user_id) DO UPDATE SET bag_id = $2`
	if _, err = b.db.ExecContext(ctx, query, userID, bagID); err != nil {
		return fmt.Errorf("error setting the default bag for %s: %w", username, err)
	}
	return nil

}

// AddBag adds (not updates) a new bag for the user. Returns the ID of the new bag record in the database.
func (b *BagsAPI) AddBag(ctx context.Context, username, contents string) (string, error) {
	query := `INSERT INTO bags (contents, user_id) VALUES ($1, $2) RETURNING id`

	userID, err := queries.UserID(ctx, b.db, username)
	if err != nil {
		return "", fmt.Errorf("error from queries.UserID in AddBag for %s: %w", username, err)
	}

	var bagID string
	if err = b.db.QueryRowContext(ctx, query, contents, userID).Scan(&bagID); err != nil {
		return "", fmt.Errorf("error adding bag for %s: %w", username, err)
	}

	return bagID, nil
}

// UpdateBag updates a specific bag with new contents.
func (b *BagsAPI) UpdateBag(ctx context.Context, username, bagID, contents string) error {
	query := `UPDATE ONLY bags SET contents = $1 WHERE id = $2 and user_id = $3`

	userID, err := queries.UserID(ctx, b.db, username)
	if err != nil {
		return fmt.Errorf("error from queries.UserID in UpdateBag for %s: %w", username, err)
	}

	if _, err = b.db.ExecContext(ctx, query, contents, bagID, userID); err != nil {
		return fmt.Errorf("error updating bag %s for %s: %w", bagID, username, err)
	}

	return nil
}

// UpdateDefaultBag updates the default bag with new content.
func (b *BagsAPI) UpdateDefaultBag(ctx context.Context, username, contents string) error {
	var (
		err        error
		defaultBag BagRecord
	)

	if defaultBag, err = b.GetDefaultBag(ctx, username); err != nil {
		return fmt.Errorf("error updating default bag for %s: %w", username, err)
	}

	return b.UpdateBag(ctx, username, defaultBag.ID, contents)
}

// DeleteBag deletes the specified bag for the user.
func (b *BagsAPI) DeleteBag(ctx context.Context, username, bagID string) error {
	query := `DELETE FROM ONLY bags WHERE id = $1 and user_id = $2`

	userID, err := queries.UserID(ctx, b.db, username)
	if err != nil {
		return fmt.Errorf("error from queries.UserID in DeleteBag for %s: %w", username, err)
	}

	if _, err = b.db.ExecContext(ctx, query, bagID, userID); err != nil {
		return fmt.Errorf("error deleting bag %s for %s: %w", bagID, username, err)
	}

	return nil
}

// DeleteDefaultBag deletes the default bag for the user. It will get
// recreated with nothing in it the next time it is retrieved through
// GetDefaultBag.
func (b *BagsAPI) DeleteDefaultBag(ctx context.Context, username string) error {
	var (
		err        error
		defaultBag BagRecord
	)

	if defaultBag, err = b.GetDefaultBag(ctx, username); err != nil {
		return fmt.Errorf("error deleting default bag for %s: %w", username, err)
	}

	return b.DeleteBag(ctx, username, defaultBag.ID)
}

// DeleteAllBags deletes all of the bags for the specified user.
func (b *BagsAPI) DeleteAllBags(ctx context.Context, username string) error {
	query := `DELETE FROM ONLY bags WHERE user_id = $1`

	userID, err := queries.UserID(ctx, b.db, username)
	if err != nil {
		return fmt.Errorf("error from queries.UserID for %s: %w", username, err)
	}

	if _, err = b.db.ExecContext(ctx, query, userID); err != nil {
		return fmt.Errorf("error deleting all bags for %s: %w", username, err)
	}

	return nil
}
