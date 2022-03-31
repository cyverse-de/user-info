package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

// UserSessionsApp is an implementation of the App interface created to manage
// user sessions.
type UserSessionsApp struct {
	sessions sDB
	router   *mux.Router
}

// NewSessionsApp returns a new *UserSessionsApp
func NewSessionsApp(db sDB, router *mux.Router) *UserSessionsApp {
	sessionsApp := &UserSessionsApp{
		sessions: db,
		router:   router,
	}
	sessionsApp.router.HandleFunc("/sessions/", sessionsApp.Greeting).Methods("GET")
	sessionsApp.router.HandleFunc("/sessions/{username}", sessionsApp.GetRequest).Methods("GET")
	sessionsApp.router.HandleFunc("/sessions/{username}", sessionsApp.PutRequest).Methods("PUT")
	sessionsApp.router.HandleFunc("/sessions/{username}", sessionsApp.PostRequest).Methods("POST")
	sessionsApp.router.HandleFunc("/sessions/{username}", sessionsApp.DeleteRequest).Methods("DELETE")
	return sessionsApp
}

// Greeting prints out a greeting to the writer from user-sessions.
func (u *UserSessionsApp) Greeting(writer http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(writer, "Hello from user-sessions.\n")
}

func (u *UserSessionsApp) getUserSessionForRequest(ctx context.Context, username string, wrap bool) ([]byte, error) {
	sessions, err := u.sessions.getSessions(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("Error getting sessions for username %s: %s", username, err)
	}

	var retval UserSessionRecord
	if len(sessions) >= 1 {
		retval = sessions[0]
	}

	response, err := convertSessions(&retval, wrap)
	if err != nil {
		return nil, fmt.Errorf("Error generating response for username %s: %s", username, err)
	}

	var jsoned []byte
	if len(response) > 0 {
		jsoned, err = json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("Error generating session JSON for user %s: %s", username, err)
		}
	} else {
		jsoned = []byte("{}")
	}

	return jsoned, nil
}

// GetRequest handles writing out a user's session as a response.
func (u *UserSessionsApp) GetRequest(writer http.ResponseWriter, r *http.Request) {
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
		"service": "sessions",
	}).Info("Getting user session for ", username)
	if userExists, err = u.sessions.isUser(ctx, username); err != nil {
		badRequest(writer, fmt.Sprintf("Error checking for username %s: %s", username, err))
		return
	}

	if !userExists {
		badRequest(writer, fmt.Sprintf("User %s does not exist", username))
		return
	}

	jsoned, err := u.getUserSessionForRequest(ctx, username, false)
	if err != nil {
		errored(writer, err.Error())
	}

	writer.Write(jsoned) // nolint:errcheck
}

// PutRequest handles creating a new user session.
func (u *UserSessionsApp) PutRequest(writer http.ResponseWriter, r *http.Request) {
	u.PostRequest(writer, r)
}

// PostRequest handles modifying an existing user session.
func (u *UserSessionsApp) PostRequest(writer http.ResponseWriter, r *http.Request) {
	var (
		username   string
		userExists bool
		hasSession bool
		err        error
		ok         bool
		v          = mux.Vars(r)
		ctx        = r.Context()
	)

	if username, ok = v["username"]; !ok {
		badRequest(writer, "Missing username in URL")
		return
	}

	if userExists, err = u.sessions.isUser(ctx, username); err != nil {
		badRequest(writer, fmt.Sprintf("Error checking for username %s: %s", username, err))
		return
	}

	if !userExists {
		badRequest(writer, fmt.Sprintf("User %s does not exist", username))
		return
	}

	if hasSession, err = u.sessions.hasSessions(ctx, username); err != nil {
		errored(writer, fmt.Sprintf("Error checking session for user %s: %s", username, err))
		return
	}

	var checked map[string]interface{}
	bodyBuffer, err := ioutil.ReadAll(r.Body)
	if err != nil {
		errored(writer, fmt.Sprintf("Error reading body: %s", err))
		return
	}

	if err = json.Unmarshal(bodyBuffer, &checked); err != nil {
		errored(writer, fmt.Sprintf("Error parsing request body: %s", err))
		return
	}

	bodyString := string(bodyBuffer)
	if !hasSession {
		if err = u.sessions.insertSession(ctx, username, bodyString); err != nil {
			errored(writer, fmt.Sprintf("Error inserting session for user %s: %s", username, err))
			return
		}
	} else {
		if err = u.sessions.updateSession(ctx, username, bodyString); err != nil {
			errored(writer, fmt.Sprintf("Error updating session for user %s: %s", username, err))
			return
		}
	}

	jsoned, err := u.getUserSessionForRequest(ctx, username, true)
	if err != nil {
		errored(writer, err.Error())
		return
	}

	writer.Write(jsoned) // nolint:errcheck
}

// DeleteRequest handles deleting a user session.
func (u *UserSessionsApp) DeleteRequest(writer http.ResponseWriter, r *http.Request) {
	var (
		username   string
		userExists bool
		hasSession bool
		err        error
		ok         bool
		v          = mux.Vars(r)
		ctx        = r.Context()
	)

	if username, ok = v["username"]; !ok {
		badRequest(writer, "Missing username in URL")
		return
	}

	if userExists, err = u.sessions.isUser(ctx, username); err != nil {
		badRequest(writer, fmt.Sprintf("Error checking for username %s: %s", username, err))
		return
	}

	if !userExists {
		badRequest(writer, fmt.Sprintf("User %s does not exist", username))
		return
	}

	if hasSession, err = u.sessions.hasSessions(ctx, username); err != nil {
		errored(writer, fmt.Sprintf("Error checking session for user %s: %s", username, err))
		return
	}

	if !hasSession {
		return
	}

	if err = u.sessions.deleteSession(ctx, username); err != nil {
		errored(writer, fmt.Sprintf("Error deleting session for user %s: %s", username, err))
	}
}
