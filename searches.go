package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gorilla/mux"
)

// SavedSearchesApp is an implementation of the App interface created to manage
// saved-searches
type SavedSearchesApp struct {
	searches seDB
	router   *mux.Router
}

// NewSearchesApp returns a new *SavedSearchesApp
func NewSearchesApp(db seDB, router *mux.Router) *SavedSearchesApp {
	searchesApp := &SavedSearchesApp{
		searches: db,
		router:   router,
	}
	router.HandleFunc("/searches/", searchesApp.Greeting).Methods("GET")
	router.HandleFunc("/searches/{username}", searchesApp.GetRequest).Methods("GET")
	router.HandleFunc("/searches/{username}", searchesApp.PutRequest).Methods("PUT")
	router.HandleFunc("/searches/{username}", searchesApp.PostRequest).Methods("POST")
	router.HandleFunc("/searches/{username}", searchesApp.DeleteRequest).Methods("DELETE")
	router.Handle("/debug/vars", http.DefaultServeMux)
	return searchesApp
}

// Greeting prints out a greeting to the writer from saved-searches.
func (s *SavedSearchesApp) Greeting(writer http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(writer, "Hello from saved-searches.\n")
}

// GetRequest handles writing out a user's saved searches as a response.
func (s *SavedSearchesApp) GetRequest(writer http.ResponseWriter, r *http.Request) {
	var (
		username   string
		userExists bool
		err        error
		ok         bool
		searches   []string
		v          = mux.Vars(r)
		ctx        = r.Context()
	)

	if username, ok = v["username"]; !ok {
		badRequest(writer, "Missing username in URL")
		return
	}

	if userExists, err = s.searches.isUser(ctx, username); err != nil {
		badRequest(writer, fmt.Sprintf("Error checking for username %s: %s", username, err))
		return
	}

	if !userExists {
		handleNonUser(writer, username)
		return
	}

	if searches, err = s.searches.getSavedSearches(ctx, username); err != nil {
		errored(writer, err.Error())
		return
	}

	if len(searches) < 1 {
		fmt.Fprintf(writer, "{}")
		return
	}

	fmt.Fprintf(writer, searches[0])
}

// PutRequest handles creating new user saved searches.
func (s *SavedSearchesApp) PutRequest(writer http.ResponseWriter, r *http.Request) {
	s.PostRequest(writer, r)
}

// PostRequest handles modifying an existing user's saved searches.
func (s *SavedSearchesApp) PostRequest(writer http.ResponseWriter, r *http.Request) {
	var (
		username    string
		userExists  bool
		hasSearches bool
		err         error
		ok          bool
		v           = mux.Vars(r)
		ctx         = r.Context()
	)

	if username, ok = v["username"]; !ok {
		badRequest(writer, "Missing username in URL")
		return
	}

	bodyBuffer, err := io.ReadAll(r.Body)
	if err != nil {
		errored(writer, fmt.Sprintf("Error reading body: %s", err))
		return
	}

	// Make sure valid JSON was uploaded in the body.
	var parsedBody interface{}
	if err = json.Unmarshal(bodyBuffer, &parsedBody); err != nil {
		badRequest(writer, fmt.Sprintf("Error parsing body: %s", err.Error()))
		return
	}

	bodyString := string(bodyBuffer)

	if userExists, err = s.searches.isUser(ctx, username); err != nil {
		badRequest(writer, fmt.Sprintf("Error checking for username %s: %s", username, err))
		return
	}

	if !userExists {
		handleNonUser(writer, username)
		return
	}

	if hasSearches, err = s.searches.hasSavedSearches(ctx, username); err != nil {
		errored(writer, err.Error())
		return
	}

	var upsert func(context.Context, string, string) error
	if hasSearches {
		upsert = s.searches.updateSavedSearches
	} else {
		upsert = s.searches.insertSavedSearches
	}
	if err = upsert(ctx, username, bodyString); err != nil {
		errored(writer, err.Error())
		return
	}

	retval := map[string]interface{}{
		"saved_searches": parsedBody,
	}
	jsoned, err := json.Marshal(retval)
	if err != nil {
		errored(writer, err.Error())
		return
	}

	writer.Write(jsoned) // nolint:errcheck
}

// DeleteRequest handles deleting a user's saved searches.
func (s *SavedSearchesApp) DeleteRequest(writer http.ResponseWriter, r *http.Request) {
	var (
		err        error
		ok         bool
		userExists bool
		username   string
		v          = mux.Vars(r)
		ctx        = r.Context()
	)

	if username, ok = v["username"]; !ok {
		badRequest(writer, "Missing username in URL")
		return
	}

	if userExists, err = s.searches.isUser(ctx, username); err != nil {
		badRequest(writer, fmt.Sprintf("Error checking for username %s: %s", username, err))
		return
	}

	if !userExists {
		return
	}

	if err = s.searches.deleteSavedSearches(ctx, username); err != nil {
		errored(writer, err.Error())
	}
}
