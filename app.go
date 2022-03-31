package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux"
)

func badRequest(writer http.ResponseWriter, msg string) {
	http.Error(writer, msg, http.StatusBadRequest)
	log.Error(msg)
}

func errored(writer http.ResponseWriter, msg string) {
	http.Error(writer, msg, http.StatusInternalServerError)
	log.Error(msg)
}

func notFound(writer http.ResponseWriter, msg string) {
	http.Error(writer, msg, http.StatusNotFound)
	log.Error(msg)
}

func handleNonUser(writer http.ResponseWriter, username string) {
	var (
		retval []byte
		err    error
	)

	retval, err = json.Marshal(map[string]string{
		"user": username,
	})
	if err != nil {
		errored(writer, fmt.Sprintf("Error generating json for non-user %s", err))
		return
	}

	notFound(writer, string(retval))
}

func fixAddr(addr string) string {
	if !strings.HasPrefix(addr, ":") {
		return fmt.Sprintf(":%s", addr)
	}
	return addr
}

var (
	gitref  string
	appver  string
	builtby string
)

// AppVersion prints the version information to stdout
func AppVersion() {
	if appver != "" {
		fmt.Printf("App-Version: %s\n", appver)
	}
	if gitref != "" {
		fmt.Printf("Git-Ref: %s\n", gitref)
	}
	if builtby != "" {
		fmt.Printf("Built-By: %s\n", builtby)
	}
}

func makeRouter() *mux.Router {
	router := mux.NewRouter()
	router.Use(otelmux.Middleware(serviceName))
	router.Handle("/debug/vars", http.DefaultServeMux)
	router.HandleFunc("/", func(writer http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(writer, "Hello from user-info.\n")
	}).Methods("GET")

	return router
}
