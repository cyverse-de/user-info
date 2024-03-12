package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

// UserPreferencesRecord represents a user's preferences stored in the database
type UserPreferencesRecord struct {
	ID          string
	Preferences string
	UserID      string
}

// convert makes sure that the JSON has the correct format. "wrap" tells convert
// whether to wrap the object in a map with "preferences" as the key.
func convertPrefs(record *UserPreferencesRecord, wrap bool) (map[string]interface{}, error) {
	var values map[string]interface{}

	if record.Preferences != "" {
		if err := json.Unmarshal([]byte(record.Preferences), &values); err != nil {
			return nil, err
		}
	}

	// We don't want the return value wrapped in a preferences object, so unwrap it
	// if it is wrapped.
	if !wrap {
		if _, ok := values["preferences"]; ok {
			return values["preferences"].(map[string]interface{}), nil
		}
		return values, nil
	}

	// We do want the return value wrapped in a preferences object, so wrap it if it
	// isn't already.
	if _, ok := values["preferences"]; !ok {
		newmap := make(map[string]interface{})
		newmap["preferences"] = values
		return newmap, nil
	}

	return values, nil
}

// UserPreferencesApp is an implementation of the App interface created to manage
// user preferences.
type UserPreferencesApp struct {
	prefs  pDB
	router *mux.Router
}

// NewPrefsApp returns a new *UserPreferencesApp
func NewPrefsApp(db pDB, router *mux.Router) *UserPreferencesApp {
	prefsApp := &UserPreferencesApp{
		prefs:  db,
		router: router,
	}
	prefsApp.router.HandleFunc("/preferences/", prefsApp.Greeting).Methods("GET")
	prefsApp.router.HandleFunc("/preferences/{username}", prefsApp.GetRequest).Methods("GET")
	prefsApp.router.HandleFunc("/preferences/{username}", prefsApp.PutRequest).Methods("PUT")
	prefsApp.router.HandleFunc("/preferences/{username}", prefsApp.PostRequest).Methods("POST")
	prefsApp.router.HandleFunc("/preferences/{username}", prefsApp.DeleteRequest).Methods("DELETE")
	return prefsApp
}

// Greeting prints out a greeting to the writer from user-prefs.
func (u *UserPreferencesApp) Greeting(writer http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(writer, "Hello from user-preferences.\n")
}

func (u *UserPreferencesApp) getUserPreferencesForRequest(ctx context.Context, username string, wrap bool) ([]byte, error) {
	var retval UserPreferencesRecord

	prefs, err := u.prefs.getPreferences(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("error getting preferences for username %s: %s", username, err)
	}

	if len(prefs) >= 1 {
		retval = prefs[0]
	}

	response, err := convertPrefs(&retval, wrap)
	if err != nil {
		return nil, fmt.Errorf("error generating response for username %s: %s", username, err)
	}

	var jsoned []byte
	if len(response) > 0 {
		jsoned, err = json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("error generating preferences JSON for user %s: %s", username, err)
		}
	} else {
		jsoned = []byte("{}")
	}

	return jsoned, nil
}

// GetRequest handles writing out a user's preferences as a response.
func (u *UserPreferencesApp) GetRequest(writer http.ResponseWriter, r *http.Request) {
	var (
		username   string
		userExists bool
		err        error
		ok         bool
		v          = mux.Vars(r)
		ctx        = r.Context()
	)

	if username, ok = v["username"]; !ok {
		badRequest(writer, "Missing username in URL")
		return
	}

	log.WithFields(log.Fields{
		"service": "preferences",
	}).Info("Getting user preferences for ", username)
	if userExists, err = u.prefs.isUser(ctx, username); err != nil {
		badRequest(writer, fmt.Sprintf("Error checking for username %s: %s", username, err))
		return
	}

	if !userExists {
		handleNonUser(writer, username)
		return
	}

	jsoned, err := u.getUserPreferencesForRequest(ctx, username, false)
	if err != nil {
		errored(writer, err.Error())
	}

	writer.Write(jsoned) // nolint:errcheck
}

// PutRequest handles creating new user preferences.
func (u *UserPreferencesApp) PutRequest(writer http.ResponseWriter, r *http.Request) {
	u.PostRequest(writer, r)
}

// PostRequest handles modifying an existing user's preferences.
func (u *UserPreferencesApp) PostRequest(writer http.ResponseWriter, r *http.Request) {
	var (
		username   string
		userExists bool
		hasPrefs   bool
		err        error
		ok         bool
		v          = mux.Vars(r)
		ctx        = r.Context()
	)

	if username, ok = v["username"]; !ok {
		badRequest(writer, "Missing username in URL")
		return
	}

	if userExists, err = u.prefs.isUser(ctx, username); err != nil {
		badRequest(writer, fmt.Sprintf("Error checking for username %s: %s", username, err))
		return
	}

	if !userExists {
		handleNonUser(writer, username)
		return
	}

	if hasPrefs, err = u.prefs.hasPreferences(ctx, username); err != nil {
		errored(writer, fmt.Sprintf("Error checking preferences for user %s: %s", username, err))
		return
	}

	var checked map[string]interface{}
	bodyBuffer, err := io.ReadAll(r.Body)
	if err != nil {
		errored(writer, fmt.Sprintf("Error reading body: %s", err))
		return
	}

	if err = json.Unmarshal(bodyBuffer, &checked); err != nil {
		errored(writer, fmt.Sprintf("Error parsing request body: %s", err))
		return
	}

	bodyString := string(bodyBuffer)
	if !hasPrefs {
		if err = u.prefs.insertPreferences(ctx, username, bodyString); err != nil {
			errored(writer, fmt.Sprintf("Error inserting preferences for user %s: %s", username, err))
			return
		}
	} else {
		if err = u.prefs.updatePreferences(ctx, username, bodyString); err != nil {
			errored(writer, fmt.Sprintf("Error updating preferences for user %s: %s", username, err))
			return
		}
	}

	jsoned, err := u.getUserPreferencesForRequest(ctx, username, true)
	if err != nil {
		errored(writer, err.Error())
		return
	}

	writer.Write(jsoned) // nolint:errcheck
}

// DeleteRequest handles deleting a user's preferences.
func (u *UserPreferencesApp) DeleteRequest(writer http.ResponseWriter, r *http.Request) {
	var (
		username   string
		userExists bool
		hasPrefs   bool
		err        error
		ok         bool
		v          = mux.Vars(r)
		ctx        = r.Context()
	)

	if username, ok = v["username"]; !ok {
		badRequest(writer, "Missing username in URL")
		return
	}

	if userExists, err = u.prefs.isUser(ctx, username); err != nil {
		badRequest(writer, fmt.Sprintf("Error checking for username %s: %s", username, err))
		return
	}

	if !userExists {
		handleNonUser(writer, username)
		return
	}

	if hasPrefs, err = u.prefs.hasPreferences(ctx, username); err != nil {
		errored(writer, fmt.Sprintf("Error checking preferences for user %s: %s", username, err))
		return
	}

	if !hasPrefs {
		return
	}

	if err = u.prefs.deletePreferences(ctx, username); err != nil {
		errored(writer, fmt.Sprintf("Error deleting preferences for user %s: %s", username, err))
	}
}
