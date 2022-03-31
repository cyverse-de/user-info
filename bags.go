package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/cyverse-de/queries"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

// BagsApp contains the routing and request handling code for bags.
type BagsApp struct {
	api        *BagsAPI
	router     *mux.Router
	userDomain string
}

// NewBagsApp creates a new BagsApp instance.
func NewBagsApp(db *sql.DB, router *mux.Router, userDomain string) *BagsApp {
	bagsApp := &BagsApp{
		api: &BagsAPI{
			db: db,
		},
		router:     router,
		userDomain: userDomain,
	}
	bagsApp.router.HandleFunc("/bags/", bagsApp.Greeting).Methods(http.MethodGet)
	bagsApp.router.HandleFunc("/bags/{username}", bagsApp.HasBags).Methods(http.MethodHead)
	bagsApp.router.HandleFunc("/bags/{username}/default", bagsApp.GetDefaultBag).Methods(http.MethodGet)
	bagsApp.router.HandleFunc("/bags/{username}/default", bagsApp.UpdateDefaultBag).Methods(http.MethodPost)
	bagsApp.router.HandleFunc("/bags/{username}/default", bagsApp.DeleteDefaultBag).Methods(http.MethodDelete)
	bagsApp.router.HandleFunc("/bags/{username}", bagsApp.GetBags).Methods(http.MethodGet)
	bagsApp.router.HandleFunc("/bags/{username}/{bagID}", bagsApp.GetBag).Methods(http.MethodGet)
	bagsApp.router.HandleFunc("/bags/{username}", bagsApp.AddBag).Methods(http.MethodPut)
	bagsApp.router.HandleFunc("/bags/{username}/{bagID}", bagsApp.UpdateBag).Methods(http.MethodPost)
	bagsApp.router.HandleFunc("/bags/{username}/{bagID}", bagsApp.DeleteBag).Methods(http.MethodDelete)
	bagsApp.router.HandleFunc("/bags/{username}", bagsApp.DeleteAllBags).Methods(http.MethodDelete)
	return bagsApp
}

// AddUsernameSuffix appends the @iplantcollaborative.org string to the
// username if it's not already there.
func (b *BagsApp) AddUsernameSuffix(username string) string {
	var retval string
	if !strings.Contains(username, "@") {
		retval = fmt.Sprintf("%s%s", username, IplantSuffix)
	} else {
		retval = username
	}
	return retval
}

// Greeting prints out a greeting for the bags endpoints.
func (b *BagsApp) Greeting(writer http.ResponseWriter, request *http.Request) {
	fmt.Fprintf(writer, "Hello from the bags handler")
}

func (b *BagsApp) getUser(ctx context.Context, vars map[string]string) (string, int, error) {
	var (
		username       string
		err            error
		ok, userExists bool
	)
	if username, ok = vars["username"]; !ok {
		return "", http.StatusBadRequest, errors.New("missing username in the URL")
	}

	username = b.AddUsernameSuffix(username)

	if userExists, err = queries.IsUser(ctx, b.api.db, username); err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("error checking for bags %s: %s", username, err)
	}

	if !userExists {
		return "", http.StatusNotFound, fmt.Errorf("user %s does not exist", username)
	}

	return username, http.StatusOK, nil
}

// GetBags returns a listing of the bags for the user.
func (b *BagsApp) GetBags(writer http.ResponseWriter, request *http.Request) {
	var (
		username string
		bags     []BagRecord
		err      error
		status   int
		vars     = mux.Vars(request)
		ctx      = request.Context()
	)

	if username, status, err = b.getUser(ctx, vars); err != nil {
		http.Error(writer, err.Error(), status)
		return
	}

	if bags, err = b.api.GetBags(ctx, username); err != nil {
		http.Error(writer, fmt.Sprintf("error getting bags for %s: %s", username, err), http.StatusInternalServerError)
		return
	}

	jsonBytes, err := json.Marshal(map[string][]BagRecord{"bags": bags})
	if err != nil {
		http.Error(writer, fmt.Sprintf("error JSON encoding result for %s: %s", username, err), http.StatusInternalServerError)
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	if _, err = writer.Write(jsonBytes); err != nil {
		log.Error(err)
	}
}

// GetBag returns a single bag.
func (b *BagsApp) GetBag(writer http.ResponseWriter, request *http.Request) {
	var (
		username, bagID string
		bag             BagRecord
		err             error
		ok              bool
		status          int
		vars            = mux.Vars(request)
		jsonBytes       []byte
		ctx             = request.Context()
	)

	if username, status, err = b.getUser(ctx, vars); err != nil {
		http.Error(writer, err.Error(), status)
	}

	if bagID, ok = vars["bagID"]; !ok {
		badRequest(writer, "missing bagID in the URL")
		return
	}

	if ok, err = b.api.HasBag(ctx, username, bagID); err != nil {
		badRequest(writer, fmt.Sprintf("error checking database for bag %s for %s: %s", bagID, username, err))
		return
	}

	if !ok {
		http.Error(writer, fmt.Sprintf("bag %s not found for user %s", bagID, username), http.StatusNotFound)
		return
	}

	if bag, err = b.api.GetBag(ctx, username, bagID); err != nil {
		http.Error(writer, fmt.Sprintf("error getting bags for %s: %s", username, err), http.StatusInternalServerError)
		return
	}

	if jsonBytes, err = json.Marshal(bag); err != nil {
		http.Error(writer, fmt.Sprintf("error JSON encoding result for %s: %s", username, err), http.StatusInternalServerError)
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	if _, err = writer.Write(jsonBytes); err != nil {
		log.Error(err)
	}
}

// GetDefaultBag will return the default bag for the user, creating a new one and setting it as the default if no default is
// already set.
func (b *BagsApp) GetDefaultBag(writer http.ResponseWriter, request *http.Request) {
	var (
		username  string
		bag       BagRecord
		err       error
		status    int
		jsonBytes []byte
		vars      = mux.Vars(request)
		ctx       = request.Context()
	)

	if username, status, err = b.getUser(ctx, vars); err != nil {
		http.Error(writer, err.Error(), status)
	}

	if bag, err = b.api.GetDefaultBag(ctx, username); err != nil {
		http.Error(writer, fmt.Sprintf("error getting default bag for %s: %s", username, err), http.StatusInternalServerError)
		return
	}

	if jsonBytes, err = json.Marshal(bag); err != nil {
		http.Error(writer, fmt.Sprintf("error JSON encoding result for %s: %s", username, err), http.StatusInternalServerError)
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	if _, err = writer.Write(jsonBytes); err != nil {
		log.Error(err)
	}
}

// AddBag adds an additional bag to the list for the user.
func (b *BagsApp) AddBag(writer http.ResponseWriter, request *http.Request) {
	var (
		username, bagID string
		bag             BagRecord
		err             error
		body            []byte
		retval          []byte
		status          int
		vars            = mux.Vars(request)
		ctx             = request.Context()
	)

	if username, status, err = b.getUser(ctx, vars); err != nil {
		http.Error(writer, err.Error(), status)
	}

	if body, err = ioutil.ReadAll(request.Body); err != nil {
		errored(writer, fmt.Sprintf("error reading body: %s", err))
		return
	}

	if err = json.Unmarshal(body, &bag); err != nil {
		badRequest(writer, fmt.Sprintf("failed to JSON decode body: %s", err))
		return
	}

	if bagID, err = b.api.AddBag(ctx, username, string(body)); err != nil {
		errored(writer, fmt.Sprintf("failed to add bag for %s: %s", username, err))
		return
	}

	if retval, err = json.Marshal(map[string]string{"id": bagID}); err != nil {
		errored(writer, fmt.Sprintf("failed to JSON encode response body: %s", err))
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	if _, err = writer.Write(retval); err != nil {
		log.Error(err)
	}
}

// UpdateBag updates the indicated bag.
func (b *BagsApp) UpdateBag(writer http.ResponseWriter, request *http.Request) {
	var (
		username, bagID string
		bag             BagRecord
		err             error
		ok              bool
		body            []byte
		status          int
		vars            = mux.Vars(request)
		ctx             = request.Context()
	)

	if username, status, err = b.getUser(ctx, vars); err != nil {
		http.Error(writer, err.Error(), status)
	}

	if bagID, ok = vars["bagID"]; !ok {
		badRequest(writer, "missing bagID in the URL")
		return
	}

	if ok, err = b.api.HasBag(ctx, username, bagID); err != nil {
		badRequest(writer, fmt.Sprintf("error checking database for bag %s for %s: %s", bagID, username, err))
		return
	}

	if !ok {
		http.Error(writer, fmt.Sprintf("bag %s not found for user %s", bagID, username), http.StatusNotFound)
		return
	}

	if body, err = ioutil.ReadAll(request.Body); err != nil {
		errored(writer, fmt.Sprintf("error reading body: %s", err))
		return
	}

	if err = json.Unmarshal(body, &bag); err != nil {
		errored(writer, fmt.Sprintf("failed to JSON decode body: %s", err))
		return
	}

	if err = b.api.UpdateBag(ctx, username, bagID, string(body)); err != nil {
		errored(writer, fmt.Sprintf("error updating bag for user %s: %s", username, err))
		return
	}
}

// UpdateDefaultBag sets new contents for the user's default bag.
func (b *BagsApp) UpdateDefaultBag(writer http.ResponseWriter, request *http.Request) {
	var (
		username    string
		bag, newBag BagRecord
		err         error
		body        []byte
		status      int
		vars        = mux.Vars(request)
		retval      []byte
		ctx         = request.Context()
	)

	if username, status, err = b.getUser(ctx, vars); err != nil {
		http.Error(writer, err.Error(), status)
	}

	if body, err = ioutil.ReadAll(request.Body); err != nil {
		errored(writer, fmt.Sprintf("error reading body: %s", err))
		return
	}

	if err = json.Unmarshal(body, &bag); err != nil {
		errored(writer, fmt.Sprintf("failed to JSON decode body: %s", err))
		return
	}

	if err = b.api.UpdateDefaultBag(ctx, username, string(body)); err != nil {
		errored(writer, fmt.Sprintf("error updating default bag for user %s: %s", username, err))
		return
	}

	if newBag, err = b.api.GetDefaultBag(ctx, username); err != nil {
		errored(writer, fmt.Sprintf("error getting new bag value for user %s: %s", username, err))
		return
	}

	if retval, err = json.Marshal(newBag); err != nil {
		errored(writer, fmt.Sprintf("error serializing new bag value for user %s: %s", username, err))
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	if _, err = writer.Write(retval); err != nil {
		log.Error(err)
	}
}

// DeleteBag deletes a single bag for a user.
func (b *BagsApp) DeleteBag(writer http.ResponseWriter, request *http.Request) {
	var (
		username, bagID string
		err             error
		ok              bool
		status          int
		vars            = mux.Vars(request)
		ctx             = request.Context()
	)

	if username, status, err = b.getUser(ctx, vars); err != nil {
		http.Error(writer, err.Error(), status)
	}

	if bagID, ok = vars["bagID"]; !ok {
		badRequest(writer, "missing bagID in the URL")
		return
	}

	if err = b.api.DeleteBag(ctx, username, bagID); err != nil {
		errored(writer, fmt.Sprintf("error deleting bag for user %s: %s", username, err))
		return
	}
}

// DeleteDefaultBag deletes the default bag for the user from the database.
func (b *BagsApp) DeleteDefaultBag(writer http.ResponseWriter, request *http.Request) {
	var (
		username string
		err      error
		status   int
		vars     = mux.Vars(request)
		newBag   BagRecord
		retval   []byte
		ctx      = request.Context()
	)

	if username, status, err = b.getUser(ctx, vars); err != nil {
		http.Error(writer, err.Error(), status)
	}

	if err = b.api.DeleteDefaultBag(ctx, username); err != nil {
		errored(writer, fmt.Sprintf("error deleting default bag for user %s: %s", username, err))
		return
	}

	if newBag, err = b.api.GetDefaultBag(ctx, username); err != nil {
		errored(writer, fmt.Sprintf("error getting new bag value for user %s: %s", username, err))
		return
	}

	if retval, err = json.Marshal(newBag); err != nil {
		errored(writer, fmt.Sprintf("error serializing new bag value for user %s: %s", username, err))
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	if _, err = writer.Write(retval); err != nil {
		log.Error(err)
	}

}

// DeleteAllBags deletes all bags for a user
func (b *BagsApp) DeleteAllBags(writer http.ResponseWriter, request *http.Request) {
	var (
		username string
		err      error
		status   int
		vars     = mux.Vars(request)
		ctx      = request.Context()
	)

	if username, status, err = b.getUser(ctx, vars); err != nil {
		http.Error(writer, err.Error(), status)
	}

	if err = b.api.DeleteAllBags(ctx, username); err != nil {
		errored(writer, fmt.Sprintf("error deleting bag for user %s: %s", username, err))
		return
	}
}

// HasBags returns true if the user has at least a single bag in the database.
func (b *BagsApp) HasBags(writer http.ResponseWriter, request *http.Request) {
	var (
		username string
		err      error
		hasBags  bool
		status   int
		vars     = mux.Vars(request)
		ctx      = request.Context()
	)

	if username, status, err = b.getUser(ctx, vars); err != nil {
		http.Error(writer, err.Error(), status)
	}

	if hasBags, err = b.api.HasBags(ctx, username); err != nil {
		errored(writer, fmt.Sprintf("error looking for bags for %s: %s", username, err))
		return
	}

	if !hasBags {
		writer.WriteHeader(http.StatusNotFound)
	} else {
		writer.WriteHeader(http.StatusOK)
	}
}
