package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

// AlertsApp is an implementation of the App interface created to manage
// global alerts.
type AlertsApp struct {
	alerts aDB
	router *mux.Router
}

// NewAlertsApp returns a new *AlertsApp
func NewAlertsApp(db aDB, router *mux.Router) *AlertsApp {
	alertsApp := &AlertsApp{
		alerts: db,
		router: router,
	}
	alertsApp.router.HandleFunc("/alerts/", alertsApp.Greeting).Methods("GET")
	alertsApp.router.HandleFunc("/alerts/active", alertsApp.GetActiveAlerts).Methods("GET")
	alertsApp.router.HandleFunc("/alerts/all", alertsApp.GetAllAlerts).Methods("GET")
	alertsApp.router.HandleFunc("/alerts/", alertsApp.CreateAlert).Methods("POST")
	alertsApp.router.HandleFunc("/alerts/", alertsApp.DeleteAlert).Methods("DELETE")

	// also without slash
	alertsApp.router.HandleFunc("/alerts", alertsApp.Greeting).Methods("GET")
	alertsApp.router.HandleFunc("/alerts", alertsApp.CreateAlert).Methods("POST")
	alertsApp.router.HandleFunc("/alerts", alertsApp.DeleteAlert).Methods("DELETE")
	return alertsApp
}

type Alerts struct {
	Alerts []GlobalAlertRecord `json:"alerts"`
}

// Greeting prints out a greeting to the writer from alerts.
func (a *AlertsApp) Greeting(writer http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(writer, "Hello from alerts.\n")
}

// GetAllAlerts handles returning all alerts
func (a *AlertsApp) GetAllAlerts(writer http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	alerts, err := a.alerts.getAllAlerts(ctx)
	if err != nil {
		errored(writer, fmt.Sprintf("error getting all alerts: %s", err))
		return
	}

	if alerts == nil {
		alerts = []GlobalAlertRecord{}
	}

	response, err := json.Marshal(Alerts{alerts})
	if err != nil {
		errored(writer, fmt.Sprintf("error marshaling response: %s", err))
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.Write(response) // nolint:errcheck
}

// GetActiveAlerts handles returning all active alerts
func (a *AlertsApp) GetActiveAlerts(writer http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	alerts, err := a.alerts.getActiveAlerts(ctx)
	if err != nil {
		errored(writer, fmt.Sprintf("error getting active alerts: %s", err))
		return
	}

	if alerts == nil {
		alerts = []GlobalAlertRecord{}
	}

	response, err := json.Marshal(Alerts{alerts})
	if err != nil {
		errored(writer, fmt.Sprintf("error marshaling response: %s", err))
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.Write(response) // nolint:errcheck
}

// CreateAlert handles creating a new global alert
func (a *AlertsApp) CreateAlert(writer http.ResponseWriter, r *http.Request) {
	var alert GlobalAlertRecord
	ctx := r.Context()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		errored(writer, fmt.Sprintf("error reading body: %s", err))
		return
	}

	if err = json.Unmarshal(body, &alert); err != nil {
		errored(writer, fmt.Sprintf("error parsing request body: %s", err))
		return
	}

	if err = a.alerts.insertAlert(ctx, alert); err != nil {
		errored(writer, fmt.Sprintf("error creating alert: %s", err))
		return
	}

	writer.WriteHeader(http.StatusCreated)
}

// DeleteAlert handles deleting a global alert
func (a *AlertsApp) DeleteAlert(writer http.ResponseWriter, r *http.Request) {
	var params struct {
		EndDate time.Time `json:"end_date"`
		Alert   string    `json:"alert"`
	}
	ctx := r.Context()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		errored(writer, fmt.Sprintf("error reading body: %s", err))
		return
	}

	if err = json.Unmarshal(body, &params); err != nil {
		errored(writer, fmt.Sprintf("error parsing request body: %s", err))
		return
	}

	if err = a.alerts.deleteAlert(ctx, params.EndDate, params.Alert); err != nil {
		errored(writer, fmt.Sprintf("error deleting alert: %s", err))
		return
	}

	writer.WriteHeader(http.StatusOK)
}
