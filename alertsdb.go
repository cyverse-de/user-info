package main

import (
	"context"
	"database/sql"
	"time"
)

// GlobalAlertDBRecord represents a global alert as stored in the database
type GlobalAlertDBRecord struct {
	StartDate sql.NullTime
	EndDate   sql.NullTime
	Alert     string
}

// GlobalAlertRecord represents a global alert for use in the service
type GlobalAlertRecord struct {
	StartDate *time.Time `json:"start_date"`
	EndDate   *time.Time `json:"end_date"`
	Alert     string     `json:"alert"`
}

// ToService converts a database record to a service record
func (dbr GlobalAlertDBRecord) ToService() GlobalAlertRecord {
	r := GlobalAlertRecord{
		EndDate: &dbr.EndDate.Time,
		Alert:   dbr.Alert,
	}

	if dbr.StartDate.Valid {
		r.StartDate = &dbr.StartDate.Time
	}

	return r
}

// ToDB converts a service record to a database record
func (r GlobalAlertRecord) ToDB() GlobalAlertDBRecord {
	dbr := GlobalAlertDBRecord{
		EndDate: sql.NullTime{Time: *r.EndDate, Valid: r.EndDate != nil && !r.EndDate.IsZero()},
		Alert:   r.Alert,
	}

	if r.StartDate != nil && !r.StartDate.IsZero() {
		dbr.StartDate = sql.NullTime{Time: *r.StartDate, Valid: r.StartDate != nil && !r.StartDate.IsZero()}
	}

	return dbr
}

type aDB interface {
	// aDB defines the interface for interacting with the database for alerts.
	getAllAlerts(ctx context.Context) ([]GlobalAlertRecord, error)
	getActiveAlerts(ctx context.Context) ([]GlobalAlertRecord, error)
	insertAlert(ctx context.Context, alert GlobalAlertRecord) error
	deleteAlert(ctx context.Context, endDate time.Time, alert string) error
}

// AlertsDB handles interacting with the global alerts database.
type AlertsDB struct {
	db *sql.DB
}

// NewAlertsDB returns a newly created *AlertsDB
func NewAlertsDB(db *sql.DB) *AlertsDB {
	return &AlertsDB{
		db: db,
	}
}

// getAllAlerts returns all alerts, whether active or not
func (a *AlertsDB) getAllAlerts(ctx context.Context) ([]GlobalAlertRecord, error) {
	query := `SELECT start_date AT TIME ZONE (select current_setting('TIMEZONE')),
                         end_date AT TIME ZONE (select current_setting('TIMEZONE')),
			 alert
	            FROM global_alerts
		ORDER BY end_date ASC`

	rows, err := a.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []GlobalAlertRecord
	for rows.Next() {
		var dbRecord GlobalAlertDBRecord
		if err := rows.Scan(&dbRecord.StartDate, &dbRecord.EndDate, &dbRecord.Alert); err != nil {
			return nil, err
		}
		alerts = append(alerts, dbRecord.ToService())
	}

	if err := rows.Err(); err != nil {
		return alerts, err
	}

	return alerts, nil
}

// getActiveAlerts returns all active alerts (where current time is between start_date and end_date)
func (a *AlertsDB) getActiveAlerts(ctx context.Context) ([]GlobalAlertRecord, error) {
	query := `SELECT start_date AT TIME ZONE (select current_setting('TIMEZONE')),
                         end_date AT TIME ZONE (select current_setting('TIMEZONE')),
			 alert
	            FROM global_alerts
		   WHERE (start_date IS NULL OR CURRENT_TIMESTAMP >= start_date AT TIME ZONE (SELECT current_setting('TIMEZONE')))
		     AND (CURRENT_TIMESTAMP <= end_date AT TIME ZONE (SELECT current_setting('TIMEZONE')))
		ORDER BY end_date ASC`

	rows, err := a.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []GlobalAlertRecord
	for rows.Next() {
		var dbRecord GlobalAlertDBRecord
		if err := rows.Scan(&dbRecord.StartDate, &dbRecord.EndDate, &dbRecord.Alert); err != nil {
			return nil, err
		}
		alerts = append(alerts, dbRecord.ToService())
	}

	if err := rows.Err(); err != nil {
		return alerts, err
	}

	return alerts, nil
}

// insertAlert adds a new global alert to the database
func (a *AlertsDB) insertAlert(ctx context.Context, alert GlobalAlertRecord) error {
	dbRecord := alert.ToDB()
	query := `INSERT INTO global_alerts (start_date, end_date, alert)
			 VALUES ($1 AT TIME ZONE (SELECT current_setting('TIMEZONE')),
			         $2 AT TIME ZONE (SELECT current_setting('TIMEZONE')),
				 $3)`

	_, err := a.db.ExecContext(ctx, query, dbRecord.StartDate, dbRecord.EndDate, dbRecord.Alert)
	return err
}

// deleteAlert removes a global alert from the database
func (a *AlertsDB) deleteAlert(ctx context.Context, endDate time.Time, alert string) error {
	query := `DELETE FROM ONLY global_alerts 
			 WHERE end_date = $1 AND alert = $2`

	_, err := a.db.ExecContext(ctx, query, endDate, alert)
	return err
}
