// nolint
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/gorilla/mux"
)

type MockDB struct {
	storage map[string]map[string]interface{}
	users   map[string]bool
}

func NewMockDB() *MockDB {
	return &MockDB{
		storage: make(map[string]map[string]interface{}),
		users:   make(map[string]bool),
	}
}

func (m *MockDB) isUser(ctx context.Context, username string) (bool, error) {
	_, ok := m.users[username]
	return ok, nil
}

func (m *MockDB) hasPreferences(ctx context.Context, username string) (bool, error) {
	stored, ok := m.storage[username]
	if !ok {
		return false, nil
	}
	if stored == nil {
		return false, nil
	}
	prefs, ok := m.storage[username]["user-prefs"].(string)
	if !ok {
		return false, nil
	}
	if prefs == "" {
		return false, nil
	}
	return true, nil
}

func (m *MockDB) getPreferences(ctx context.Context, username string) ([]UserPreferencesRecord, error) {
	return []UserPreferencesRecord{
		{
			ID:          "id",
			Preferences: m.storage[username]["user-prefs"].(string),
			UserID:      "user-id",
		},
	}, nil
}

func (m *MockDB) insertPreferences(ctx context.Context, username, prefs string) error {
	if _, ok := m.storage[username]["user-prefs"]; !ok {
		m.storage[username] = make(map[string]interface{})
	}
	m.storage[username]["user-prefs"] = prefs
	return nil
}

func (m *MockDB) updatePreferences(ctx context.Context, username, prefs string) error {
	return m.insertPreferences(ctx, username, prefs)
}

func (m *MockDB) deletePreferences(ctx context.Context, username string) error {
	delete(m.storage, username)
	return nil
}

func TestConvertBlankPreferences(t *testing.T) {
	record := &UserPreferencesRecord{
		ID:          "test_id",
		Preferences: "",
		UserID:      "test_user_id",
	}
	actual, err := convertPrefs(record, false)
	if err != nil {
		t.Error(err)
	}
	if len(actual) > 0 {
		t.Fail()
	}
}

func TestConvertUnparseablePreferences(t *testing.T) {
	record := &UserPreferencesRecord{
		ID:          "test_id",
		Preferences: "------------",
		UserID:      "test_user_id",
	}
	actual, err := convertPrefs(record, false)
	if err == nil {
		t.Fail()
	}
	if actual != nil {
		t.Fail()
	}
}

func TestConvertEmbeddedPreferences(t *testing.T) {
	record := &UserPreferencesRecord{
		ID:          "test_id",
		Preferences: `{"preferences":{"foo":"bar"}}`,
		UserID:      "test_user_id",
	}
	actual, err := convertPrefs(record, false)
	if err != nil {
		t.Fail()
	}
	if _, ok := actual["foo"]; !ok {
		t.Fail()
	}
	if actual["foo"].(string) != "bar" {
		t.Fail()
	}
}

func TestConvertNormalPreferences(t *testing.T) {
	record := &UserPreferencesRecord{
		ID:          "test_id",
		Preferences: `{"foo":"bar"}`,
		UserID:      "test_user_id",
	}
	actual, err := convertPrefs(record, false)
	if err != nil {
		t.Fail()
	}
	if _, ok := actual["foo"]; !ok {
		t.Fail()
	}
	if actual["foo"].(string) != "bar" {
		t.Fail()
	}
}

func TestHandleNonUser(t *testing.T) {
	var (
		expectedMsg    = "{\"user\":\"test-user\"}\n"
		expectedStatus = http.StatusNotFound
	)

	recorder := httptest.NewRecorder()
	handleNonUser(recorder, "test-user")
	actualMsg := recorder.Body.String()
	actualStatus := recorder.Code

	if actualStatus != expectedStatus {
		t.Errorf("Status code was %d but should have been %d", actualStatus, expectedStatus)
	}

	if actualMsg != expectedMsg {
		t.Errorf("Message was '%s' but should have been '%s'", actualMsg, expectedMsg)
	}
}

func TestPreferencesGreeting(t *testing.T) {
	mock := NewMockDB()
	router := mux.NewRouter()
	router.Handle("/debug/vars", http.DefaultServeMux)
	n := NewPrefsApp(mock, router)

	server := httptest.NewServer(n.router)
	defer server.Close()

	url := fmt.Sprintf("%s/%s", server.URL, "preferences/")
	res, err := http.Get(url)
	if err != nil {
		t.Error(err)
	}

	expectedBody := []byte("Hello from user-preferences.\n")
	actualBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Error(err)
	}
	res.Body.Close()

	if !bytes.Equal(actualBody, expectedBody) {
		t.Errorf("Message was '%s' but should have been '%s'", actualBody, expectedBody)
	}

	expectedStatus := http.StatusOK
	actualStatus := res.StatusCode

	if actualStatus != expectedStatus {
		t.Errorf("Status code was %d but should have been %d", actualStatus, expectedStatus)
	}
}

func TestGetUserPreferencesForRequest(t *testing.T) {
	mock := NewMockDB()
	router := mux.NewRouter()
	n := NewPrefsApp(mock, router)
	ctx := context.Background()

	expected := []byte("{\"one\":\"two\"}")
	expectedWrapped := []byte("{\"preferences\":{\"one\":\"two\"}}")
	mock.users["test-user"] = true
	if err := mock.insertPreferences(ctx, "test-user", string(expected)); err != nil {
		t.Error(err)
	}

	actualWrapped, err := n.getUserPreferencesForRequest(ctx, "test-user", true)
	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(actualWrapped, expectedWrapped) {
		t.Errorf("The return value was '%s' instead of '%s'", actualWrapped, expectedWrapped)
	}

	actual, err := n.getUserPreferencesForRequest(ctx, "test-user", false)
	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(actual, expected) {
		t.Errorf("The return value was '%s' instead of '%s'", actual, expected)
	}
}

func TestPreferencesGetRequest(t *testing.T) {
	mock := NewMockDB()
	router := mux.NewRouter()
	n := NewPrefsApp(mock, router)
	ctx := context.Background()

	expected := []byte("{\"one\":\"two\"}")
	mock.users["test-user"] = true
	if err := mock.insertPreferences(ctx, "test-user", string(expected)); err != nil {
		t.Error(err)
	}

	server := httptest.NewServer(n.router)
	defer server.Close()

	url := fmt.Sprintf("%s/%s", server.URL, "preferences/test-user")
	res, err := http.Get(url)
	if err != nil {
		t.Error(err)
	}

	actualBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Error(err)
	}
	res.Body.Close()

	if !bytes.Equal(actualBody, expected) {
		t.Errorf("Message was '%s' but should have been '%s'", actualBody, expected)
	}

	expectedStatus := http.StatusOK
	actualStatus := res.StatusCode

	if actualStatus != expectedStatus {
		t.Errorf("Status code was %d but should have been %d", actualStatus, expectedStatus)
	}
}

func TestPreferencesPutRequest(t *testing.T) {
	mock := NewMockDB()
	router := mux.NewRouter()
	n := NewPrefsApp(mock, router)

	username := "test-user"
	expected := []byte(`{"one":"two"}`)

	mock.users[username] = true

	server := httptest.NewServer(n.router)
	defer server.Close()

	url := fmt.Sprintf("%s/%s", server.URL, "preferences/"+username)
	httpClient := &http.Client{}
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(expected))
	if err != nil {
		t.Error(err)
	}

	res, err := httpClient.Do(req)
	if err != nil {
		t.Error(err)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Error(err)
	}
	res.Body.Close()

	var parsed map[string]map[string]string
	if err = json.Unmarshal(body, &parsed); err != nil {
		t.Error(err)
	}

	var expectedParsed map[string]string
	if err = json.Unmarshal(expected, &expectedParsed); err != nil {
		t.Error(err)
	}

	if _, ok := parsed["preferences"]; !ok {
		t.Error("JSON did not contain a 'preferences' key")
	}

	if !reflect.DeepEqual(parsed["preferences"], expectedParsed) {
		t.Errorf("Put returned %#v instead of %#v", parsed["preferences"], expectedParsed)
	}
}

func TestPreferencesPostRequest(t *testing.T) {
	mock := NewMockDB()
	router := mux.NewRouter()
	n := NewPrefsApp(mock, router)
	ctx := context.Background()

	username := "test-user"
	expected := []byte(`{"one":"two"}`)

	mock.users[username] = true
	if err := mock.insertPreferences(ctx, username, string(expected)); err != nil {
		t.Error(err)
	}

	server := httptest.NewServer(n.router)
	defer server.Close()

	url := fmt.Sprintf("%s/%s", server.URL, "preferences/"+username)
	httpClient := &http.Client{}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(expected))
	if err != nil {
		t.Error(err)
	}

	res, err := httpClient.Do(req)
	if err != nil {
		t.Error(err)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Error(err)
	}
	res.Body.Close()

	var parsed map[string]map[string]string
	if err = json.Unmarshal(body, &parsed); err != nil {
		t.Error(err)
	}

	var expectedParsed map[string]string
	if err = json.Unmarshal(expected, &expectedParsed); err != nil {
		t.Error(err)
	}

	if _, ok := parsed["preferences"]; !ok {
		t.Error("JSON did not contain a 'preferences' key")
	}

	if !reflect.DeepEqual(parsed["preferences"], expectedParsed) {
		t.Errorf("POST requeted %#v instead of %#v", parsed["preferences"], expectedParsed)
	}
}

func TestPreferencesDelete(t *testing.T) {
	username := "test-user"
	expected := []byte(`{"one":"two"}`)

	mock := NewMockDB()
	mock.users[username] = true
	router := mux.NewRouter()
	n := NewPrefsApp(mock, router)
	ctx := context.Background()

	if err := mock.insertPreferences(ctx, username, string(expected)); err != nil {
		t.Error(err)
	}

	server := httptest.NewServer(n.router)
	defer server.Close()

	url := fmt.Sprintf("%s/%s", server.URL, "preferences/"+username)
	httpClient := &http.Client{}
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Error(err)
	}

	res, err := httpClient.Do(req)
	if err != nil {
		t.Error(err)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Error(err)
	}
	res.Body.Close()

	if len(body) > 0 {
		t.Errorf("DELETE returned a body: %s", body)
	}

	expectedStatus := http.StatusOK
	actualStatus := res.StatusCode

	if actualStatus != expectedStatus {
		t.Errorf("DELETE status code was %d instead of %d", actualStatus, expectedStatus)
	}
}

func TestNewPrefsDB(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error occurred creating the mock db: %s", err)
	}
	defer db.Close()

	prefs := NewPrefsDB(db)
	if prefs == nil {
		t.Fatal("NewPrefsDB() returned nil")
	}

	if prefs.db != db {
		t.Error("dbs did not match")
	}
}

func TestPreferencesIsUser(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("error creating the mock db: %s", err)
	}
	defer db.Close()

	p := NewPrefsDB(db)
	if p == nil {
		t.Error("NewPrefsDB returned nil")
	}

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM \\( SELECT DISTINCT id FROM users").
		WithArgs("test-user").
		WillReturnRows(sqlmock.NewRows([]string{"check_user"}).AddRow(1))

	present, err := p.isUser(context.Background(), "test-user")
	if err != nil {
		t.Errorf("error calling isUser(): %s", err)
	}

	if !present {
		t.Error("test-user was not found")
	}

	if err = mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations were not met: %s", err)
	}
}

func TestHasPreferences(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("error creating the mock db: %s", err)
	}
	defer db.Close()

	p := NewPrefsDB(db)
	if p == nil {
		t.Error("NewPrefsDB returned nil")
	}

	mock.ExpectQuery("SELECT COUNT\\(p.\\*\\) FROM user_preferences p, users u WHERE p.user_id = u.id").
		WithArgs("test-user").
		WillReturnRows(sqlmock.NewRows([]string{""}).AddRow("1"))

	hasPrefs, err := p.hasPreferences(context.Background(), "test-user")
	if err != nil {
		t.Errorf("error from hasPreferences(): %s", err)
	}

	if !hasPrefs {
		t.Error("hasPreferences() returned false")
	}

	if err = mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations were not met: %s", err)
	}
}

func TestGetPreferences(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("error creating the mock db: %s", err)
	}
	defer db.Close()

	p := NewPrefsDB(db)
	if p == nil {
		t.Error("NewPrefsDB returned nil")
	}

	mock.ExpectQuery("SELECT p.id AS id, p.user_id AS user_id, p.preferences AS preferences FROM user_preferences p, users u WHERE p.user_id = u.id AND u.username =").
		WithArgs("test-user").
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "preferences"}).AddRow("1", "2", "{}"))

	records, err := p.getPreferences(context.Background(), "test-user")
	if err != nil {
		t.Errorf("error from getPreferences(): %s", err)
	}

	if len(records) != 1 {
		t.Errorf("number of records returned was %d instead of 1", len(records))
	}

	prefs := records[0]
	if prefs.UserID != "2" {
		t.Errorf("user id was %s instead of 2", prefs.UserID)
	}

	if prefs.ID != "1" {
		t.Errorf("id was %s instead of 1", prefs.ID)
	}

	if prefs.Preferences != "{}" {
		t.Errorf("preferences was %s instead of '{}'", prefs.Preferences)
	}

	if err = mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations were not met: %s", err)
	}
}

func TestInsertPreferences(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("error creating the mock db: %s", err)
	}
	defer db.Close()

	p := NewPrefsDB(db)
	if p == nil {
		t.Error("NewPrefsDB returned nil")
	}

	mock.ExpectQuery("SELECT id FROM users WHERE username =").
		WithArgs("test-user").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("1"))

	mock.ExpectExec("INSERT INTO user_preferences \\(user_id, preferences\\) VALUES").
		WithArgs("1", "{}").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err = p.insertPreferences(context.Background(), "test-user", "{}"); err != nil {
		t.Errorf("error inserting preferences: %s", err)
	}

	if err = mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations were not met: %s", err)
	}
}

func TestUpdatePreferences(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("error creating the mock db: %s", err)
	}
	defer db.Close()

	p := NewPrefsDB(db)
	if p == nil {
		t.Error("NewPrefsDB returned nil")
	}

	mock.ExpectQuery("SELECT id FROM users WHERE username =").
		WithArgs("test-user").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("1"))

	mock.ExpectExec("UPDATE ONLY user_preferences SET preferences =").
		WithArgs("1", "{}").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err = p.updatePreferences(context.Background(), "test-user", "{}"); err != nil {
		t.Errorf("error updating preferences: %s", err)
	}

	if err = mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations were not met: %s", err)
	}
}

func TestDeletePreferences(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("error creating the mock db: %s", err)
	}
	defer db.Close()

	p := NewPrefsDB(db)
	if p == nil {
		t.Error("NewPrefsDB returned nil")
	}

	mock.ExpectQuery("SELECT id FROM users WHERE username =").
		WithArgs("test-user").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("1"))

	mock.ExpectExec("DELETE FROM ONLY user_preferences WHERE user_id =").
		WithArgs("1").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err = p.deletePreferences(context.Background(), "test-user"); err != nil {
		t.Errorf("error deleting preferences: %s", err)
	}

	if err = mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations were not met: %s", err)
	}
}

// -------- End Preferences --------

// -------- Start Sessions --------
func (m *MockDB) hasSessions(ctx context.Context, username string) (bool, error) {
	stored, ok := m.storage[username]
	if !ok {
		return false, nil
	}
	if stored == nil {
		return false, nil
	}
	prefs, ok := m.storage[username]["user-sessions"].(string)
	if !ok {
		return false, nil
	}
	if prefs == "" {
		return false, nil
	}
	return true, nil
}

func (m *MockDB) getSessions(ctx context.Context, username string) ([]UserSessionRecord, error) {
	return []UserSessionRecord{
		{
			ID:      "id",
			Session: m.storage[username]["user-sessions"].(string),
			UserID:  "user-id",
		},
	}, nil
}

func (m *MockDB) insertSession(ctx context.Context, username, session string) error {
	if _, ok := m.storage[username]["user-sessions"]; !ok {
		m.storage[username] = make(map[string]interface{})
	}
	m.storage[username]["user-sessions"] = session
	return nil
}

func (m *MockDB) updateSession(ctx context.Context, username, prefs string) error {
	return m.insertSession(ctx, username, prefs)
}

func (m *MockDB) deleteSession(ctx context.Context, username string) error {
	delete(m.storage, username)
	return nil
}

func TestConvertBlankSession(t *testing.T) {
	record := &UserSessionRecord{
		ID:      "test_id",
		Session: "",
		UserID:  "test_user_id",
	}
	actual, err := convertSessions(record, false)
	if err != nil {
		t.Error(err)
	}
	if len(actual) > 0 {
		t.Fail()
	}
}

func TestConvertUnparseableSession(t *testing.T) {
	record := &UserSessionRecord{
		ID:      "test_id",
		Session: "------------",
		UserID:  "test_user_id",
	}
	actual, err := convertSessions(record, false)
	if err == nil {
		t.Fail()
	}
	if actual != nil {
		t.Fail()
	}
}

func TestConvertEmbeddedSession(t *testing.T) {
	record := &UserSessionRecord{
		ID:      "test_id",
		Session: `{"session":{"foo":"bar"}}`,
		UserID:  "test_user_id",
	}
	actual, err := convertSessions(record, false)
	if err != nil {
		t.Fail()
	}
	if _, ok := actual["foo"]; !ok {
		t.Fail()
	}
	if actual["foo"].(string) != "bar" {
		t.Fail()
	}
}

func TestConvertNormalSession(t *testing.T) {
	record := &UserSessionRecord{
		ID:      "test_id",
		Session: `{"foo":"bar"}`,
		UserID:  "test_user_id",
	}
	actual, err := convertSessions(record, false)
	if err != nil {
		t.Fail()
	}
	if _, ok := actual["foo"]; !ok {
		t.Fail()
	}
	if actual["foo"].(string) != "bar" {
		t.Fail()
	}
}

func TestSessionsGreeting(t *testing.T) {
	mock := NewMockDB()
	router := mux.NewRouter()
	router.Handle("/debug/vars", http.DefaultServeMux)
	n := NewSessionsApp(mock, router)

	server := httptest.NewServer(n.router)
	defer server.Close()

	url := fmt.Sprintf("%s/%s", server.URL, "sessions/")
	res, err := http.Get(url)
	if err != nil {
		t.Error(err)
	}

	expectedBody := []byte("Hello from user-sessions.\n")
	actualBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Error(err)
	}
	res.Body.Close()

	if !bytes.Equal(actualBody, expectedBody) {
		t.Errorf("Message was '%s' but should have been '%s'", actualBody, expectedBody)
	}

	expectedStatus := http.StatusOK
	actualStatus := res.StatusCode

	if actualStatus != expectedStatus {
		t.Errorf("Status code was %d but should have been %d", actualStatus, expectedStatus)
	}
}

func TestGetUserSessionForRequest(t *testing.T) {
	mock := NewMockDB()
	router := mux.NewRouter()
	n := NewSessionsApp(mock, router)
	ctx := context.Background()

	expected := []byte("{\"one\":\"two\"}")
	expectedWrapped := []byte("{\"session\":{\"one\":\"two\"}}")
	mock.users["test-user"] = true
	if err := mock.insertSession(ctx, "test-user", string(expected)); err != nil {
		t.Error(err)
	}

	actualWrapped, err := n.getUserSessionForRequest(ctx, "test-user", true)
	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(actualWrapped, expectedWrapped) {
		t.Errorf("The return value was '%s' instead of '%s'", actualWrapped, expectedWrapped)
	}

	actual, err := n.getUserSessionForRequest(ctx, "test-user", false)
	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(actual, expected) {
		t.Errorf("The return value was '%s' instead of '%s'", actual, expected)
	}
}

func TestSessionsGetRequest(t *testing.T) {
	mock := NewMockDB()
	router := mux.NewRouter()
	n := NewSessionsApp(mock, router)
	ctx := context.Background()

	expected := []byte("{\"one\":\"two\"}")
	mock.users["test-user"] = true
	if err := mock.insertSession(ctx, "test-user", string(expected)); err != nil {
		t.Error(err)
	}

	server := httptest.NewServer(n.router)
	defer server.Close()

	url := fmt.Sprintf("%s/%s", server.URL, "sessions/test-user")
	res, err := http.Get(url)
	if err != nil {
		t.Error(err)
	}

	actualBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Error(err)
	}
	res.Body.Close()

	if !bytes.Equal(actualBody, expected) {
		t.Errorf("Message was '%s' but should have been '%s'", actualBody, expected)
	}

	expectedStatus := http.StatusOK
	actualStatus := res.StatusCode

	if actualStatus != expectedStatus {
		t.Errorf("Status code was %d but should have been %d", actualStatus, expectedStatus)
	}
}

func TestSessionsPutRequest(t *testing.T) {
	mock := NewMockDB()
	router := mux.NewRouter()
	n := NewSessionsApp(mock, router)

	username := "test-user"
	expected := []byte(`{"one":"two"}`)

	mock.users[username] = true

	server := httptest.NewServer(n.router)
	defer server.Close()

	url := fmt.Sprintf("%s/%s", server.URL, "sessions/"+username)
	httpClient := &http.Client{}
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(expected))
	if err != nil {
		t.Error(err)
	}

	res, err := httpClient.Do(req)
	if err != nil {
		t.Error(err)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Error(err)
	}
	res.Body.Close()

	var parsed map[string]map[string]string
	if err = json.Unmarshal(body, &parsed); err != nil {
		t.Error(err)
	}

	var expectedParsed map[string]string
	if err = json.Unmarshal(expected, &expectedParsed); err != nil {
		t.Error(err)
	}

	if _, ok := parsed["session"]; !ok {
		t.Error("JSON did not contain a 'preferences' key")
	}

	if !reflect.DeepEqual(parsed["session"], expectedParsed) {
		t.Errorf("Put returned %#v instead of %#v", parsed["session"], expectedParsed)
	}
}

func TestSessionsPostRequest(t *testing.T) {
	mock := NewMockDB()
	router := mux.NewRouter()
	n := NewSessionsApp(mock, router)
	ctx := context.Background()

	username := "test-user"
	expected := []byte(`{"one":"two"}`)

	mock.users[username] = true
	if err := mock.insertSession(ctx, username, string(expected)); err != nil {
		t.Error(err)
	}

	server := httptest.NewServer(n.router)
	defer server.Close()

	url := fmt.Sprintf("%s/%s", server.URL, "sessions/"+username)
	httpClient := &http.Client{}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(expected))
	if err != nil {
		t.Error(err)
	}

	res, err := httpClient.Do(req)
	if err != nil {
		t.Error(err)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Error(err)
	}
	res.Body.Close()

	var parsed map[string]map[string]string
	if err = json.Unmarshal(body, &parsed); err != nil {
		t.Error(err)
	}

	var expectedParsed map[string]string
	if err = json.Unmarshal(expected, &expectedParsed); err != nil {
		t.Error(err)
	}

	if _, ok := parsed["session"]; !ok {
		t.Error("JSON did not contain a 'preferences' key")
	}

	if !reflect.DeepEqual(parsed["session"], expectedParsed) {
		t.Errorf("POST requeted %#v instead of %#v", parsed["session"], expectedParsed)
	}
}

func TestSessionsDelete(t *testing.T) {
	username := "test-user"
	expected := []byte(`{"one":"two"}`)

	mock := NewMockDB()
	mock.users[username] = true
	router := mux.NewRouter()
	n := NewSessionsApp(mock, router)
	ctx := context.Background()

	if err := mock.insertSession(ctx, username, string(expected)); err != nil {
		t.Error(err)
	}

	server := httptest.NewServer(n.router)
	defer server.Close()

	url := fmt.Sprintf("%s/%s", server.URL, "sessions/"+username)
	httpClient := &http.Client{}
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Error(err)
	}

	res, err := httpClient.Do(req)
	if err != nil {
		t.Error(err)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Error(err)
	}
	res.Body.Close()

	if len(body) > 0 {
		t.Errorf("DELETE returned a body: %s", body)
	}

	expectedStatus := http.StatusOK
	actualStatus := res.StatusCode

	if actualStatus != expectedStatus {
		t.Errorf("DELETE status code was %d instead of %d", actualStatus, expectedStatus)
	}
}

func TestNewSessionsDB(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("error creating the mock db: %s", err)
	}
	defer db.Close()

	p := NewSessionsDB(db)
	if p == nil {
		t.Fatal("NewSessionsDB returned nil")
	}

	if db != p.db {
		t.Error("dbs did not match")
	}
}

func TestSessionsIsUser(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("error creating the mock db: %s", err)
	}
	defer db.Close()

	p := NewSessionsDB(db)
	if p == nil {
		t.Error("NewSessionsDB returned nil")
	}

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM \\( SELECT DISTINCT id FROM users").
		WithArgs("test-user").
		WillReturnRows(sqlmock.NewRows([]string{"check_user"}).AddRow(1))

	present, err := p.isUser(context.Background(), "test-user")
	if err != nil {
		t.Errorf("error calling isUser(): %s", err)
	}

	if err = mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations were not met: %s", err)
	}

	if !present {
		t.Error("test-user was not found")
	}
}

func TestHasSessions(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("error creating the mock db: %s", err)
	}
	defer db.Close()

	p := NewSessionsDB(db)
	if p == nil {
		t.Error("NewSessionsDB returned nil")
	}

	mock.ExpectQuery("SELECT COUNT\\(s.\\*\\) FROM user_sessions s, users u WHERE s.user_id = u.id").
		WithArgs("test-user").
		WillReturnRows(sqlmock.NewRows([]string{""}).AddRow("1"))

	hasSessions, err := p.hasSessions(context.Background(), "test-user")
	if err != nil {
		t.Errorf("error from hasSessions(): %s", err)
	}

	if err = mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations were not met: %s", err)
	}

	if !hasSessions {
		t.Error("hasSessions() returned false")
	}
}

func TestGetSessions(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("error creating the mock db: %s", err)
	}
	defer db.Close()

	p := NewSessionsDB(db)
	if p == nil {
		t.Error("NewSessionsDB returned nil")
	}

	mock.ExpectQuery("SELECT s.id AS id, s.user_id AS user_id, s.session AS session FROM user_sessions s, users u WHERE s.user_id = u.id AND u.username =").
		WithArgs("test-user").
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "session"}).AddRow("1", "2", "{}"))

	records, err := p.getSessions(context.Background(), "test-user")
	if err != nil {
		t.Errorf("error from getSessions(): %s", err)
	}

	if err = mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations were not met: %s", err)
	}

	if len(records) != 1 {
		t.Errorf("number of records returned was %d instead of 1", len(records))
	}

	session := records[0]
	if session.UserID != "2" {
		t.Errorf("user id was %s instead of 2", session.UserID)
	}

	if session.ID != "1" {
		t.Errorf("id was %s instead of 1", session.ID)
	}

	if session.Session != "{}" {
		t.Errorf("session was %s instead of '{}'", session.Session)
	}
}

func TestInsertSession(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("error creating the mock db: %s", err)
	}
	defer db.Close()

	p := NewSessionsDB(db)
	if p == nil {
		t.Error("NewSessionsDB returned nil")
	}

	mock.ExpectQuery("SELECT id FROM users WHERE username =").
		WithArgs("test-user").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("1"))

	mock.ExpectExec("INSERT INTO user_sessions \\(user_id, session\\) VALUES").
		WithArgs("1", "{}").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err = p.insertSession(context.Background(), "test-user", "{}"); err != nil {
		t.Errorf("error inserting session: %s", err)
	}

	if err = mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations were not met: %s", err)
	}
}

func TestUpdateSession(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("error creating the mock db: %s", err)
	}
	defer db.Close()

	p := NewSessionsDB(db)
	if p == nil {
		t.Error("NewSessionsDB returned nil")
	}

	mock.ExpectQuery("SELECT id FROM users WHERE username =").
		WithArgs("test-user").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("1"))

	mock.ExpectExec("UPDATE ONLY user_sessions SET session =").
		WithArgs("1", "{}").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err = p.updateSession(context.Background(), "test-user", "{}"); err != nil {
		t.Errorf("error updating session: %s", err)
	}

	if err = mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations were not met: %s", err)
	}
}

func TestDeleteSession(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("error creating the mock db: %s", err)
	}
	defer db.Close()

	p := NewSessionsDB(db)
	if p == nil {
		t.Error("NewSessionsDB returned nil")
	}

	mock.ExpectQuery("SELECT id FROM users WHERE username =").
		WithArgs("test-user").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("1"))

	mock.ExpectExec("DELETE FROM ONLY user_sessions WHERE user_id =").
		WithArgs("1").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err = p.deleteSession(context.Background(), "test-user"); err != nil {
		t.Errorf("error deleting session: %s", err)
	}

	if err = mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations were not met: %s", err)
	}
}

// -------- End Sessions --------

// -------- Start Searches --------
func (m *MockDB) hasSavedSearches(ctx context.Context, username string) (bool, error) {
	stored, ok := m.storage[username]
	if !ok {
		return false, nil
	}
	if stored == nil {
		return false, nil
	}
	searches, ok := m.storage[username]["saved_searches"].(string)
	if !ok {
		return false, nil
	}
	return len(searches) > 0, nil

}

func (m *MockDB) getSavedSearches(ctx context.Context, username string) ([]string, error) {
	return []string{m.storage[username]["saved_searches"].(string)}, nil
}

func (m *MockDB) deleteSavedSearches(ctx context.Context, username string) error {
	delete(m.storage, username)
	return nil
}

func (m *MockDB) insertSavedSearches(ctx context.Context, username, savedSearches string) error {
	if _, ok := m.storage[username]["saved_searches"]; !ok {
		m.storage[username] = make(map[string]interface{})
	}
	m.storage[username]["saved_searches"] = savedSearches
	return nil
}

func (m *MockDB) updateSavedSearches(ctx context.Context, username, savedSearches string) error {
	return m.insertSavedSearches(ctx, username, savedSearches)
}

func TestSearchesGreeting(t *testing.T) {
	mock := NewMockDB()
	router := mux.NewRouter()
	router.Handle("/debug/vars", http.DefaultServeMux)
	n := NewSearchesApp(mock, router)

	server := httptest.NewServer(n.router)
	defer server.Close()

	url := fmt.Sprintf("%s/%s", server.URL, "searches/")
	res, err := http.Get(url)
	if err != nil {
		t.Error(err)
	}

	expectedBody := []byte("Hello from saved-searches.\n")
	actualBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Error(err)
	}
	res.Body.Close()

	if !bytes.Equal(actualBody, expectedBody) {
		t.Errorf("Message was '%s' but should have been '%s'", actualBody, expectedBody)
	}

	expectedStatus := http.StatusOK
	actualStatus := res.StatusCode

	if actualStatus != expectedStatus {
		t.Errorf("Status code was %d but should have been %d", actualStatus, expectedStatus)
	}
}

func TestGetSavedSearchesForRequest(t *testing.T) {
	username := "test_user@test-domain.org"
	expectedBody := `{"search":"fake"}`
	ctx := context.Background()

	mock := NewMockDB()
	mock.users[username] = true
	if err := mock.insertSavedSearches(ctx, username, expectedBody); err != nil {
		t.Error(err)
	}

	router := mux.NewRouter()
	n := NewSearchesApp(mock, router)
	server := httptest.NewServer(n.router)
	defer server.Close()

	url := fmt.Sprintf("%s/%s", server.URL, "searches/"+username)
	res, err := http.Get(url)
	if err != nil {
		t.Error(err)
	}

	bodyBytes, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Error(err)
	}

	actualBody := string(bodyBytes)

	expectedStatus := http.StatusOK
	actualStatus := res.StatusCode

	if actualBody != expectedBody {
		t.Errorf("Body of the response was '%s' instead of '%s'", actualBody, expectedBody)
	}

	if actualStatus != expectedStatus {
		t.Errorf("Status of the response was %d instead of %d", actualStatus, expectedStatus)
	}
}

func TestPutInsertSavedSearchesForRequest(t *testing.T) {
	username := "test_user@test-domain.org"
	expectedBody := `{"search":"fake"}`

	mock := NewMockDB()
	mock.users[username] = true

	router := mux.NewRouter()
	n := NewSearchesApp(mock, router)
	server := httptest.NewServer(n.router)
	defer server.Close()

	url := fmt.Sprintf("%s/%s", server.URL, "searches/"+username)
	httpClient := &http.Client{}
	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(expectedBody))
	if err != nil {
		t.Error(err)
	}
	res, err := httpClient.Do(req)
	if err != nil {
		t.Error(err)
	}

	bodyBytes, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Error(err)
	}

	var parsed map[string]map[string]string
	err = json.Unmarshal(bodyBytes, &parsed)
	if err != nil {
		t.Error(err)
	}

	var expectedParsed map[string]string
	err = json.Unmarshal([]byte(expectedBody), &expectedParsed)
	if err != nil {
		t.Error(err)
	}

	if _, ok := parsed["saved_searches"]; !ok {
		t.Error("Parsed response did not have a top-level 'saved_searches' key")
	}

	if !reflect.DeepEqual(parsed["saved_searches"], expectedParsed) {
		t.Errorf("Put returned '%#v' as the saved search instead of '%#v'", parsed["saved_searches"], expectedBody)
	}
}

func TestPutUpdateSavedSearchesForRequest(t *testing.T) {
	username := "test_user@test-domain.org"
	expectedBody := `{"search":"fake"}`
	ctx := context.Background()

	mock := NewMockDB()
	mock.users[username] = true
	if err := mock.insertSavedSearches(ctx, username, expectedBody); err != nil {
		t.Error(err)
	}

	router := mux.NewRouter()
	n := NewSearchesApp(mock, router)
	server := httptest.NewServer(n.router)
	defer server.Close()

	url := fmt.Sprintf("%s/%s", server.URL, "searches/"+username)
	httpClient := &http.Client{}
	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(expectedBody))
	if err != nil {
		t.Error(err)
	}
	res, err := httpClient.Do(req)
	if err != nil {
		t.Error(err)
	}

	bodyBytes, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Error(err)
	}

	var parsed map[string]map[string]string
	err = json.Unmarshal(bodyBytes, &parsed)
	if err != nil {
		t.Error(err)
	}

	var expectedParsed map[string]string
	err = json.Unmarshal([]byte(expectedBody), &expectedParsed)
	if err != nil {
		t.Error(err)
	}

	if _, ok := parsed["saved_searches"]; !ok {
		t.Error("Parsed response did not have a top-level 'saved_searches' key")
	}

	if !reflect.DeepEqual(parsed["saved_searches"], expectedParsed) {
		t.Errorf("Put returned '%#v' as the saved search instead of '%#v'", parsed["saved_searches"], expectedBody)
	}
}

func TestPostInsertSavedSearchesForRequest(t *testing.T) {
	username := "test_user@test-domain.org"
	expectedBody := `{"search":"fake"}`

	mock := NewMockDB()
	mock.users[username] = true

	router := mux.NewRouter()
	n := NewSearchesApp(mock, router)
	server := httptest.NewServer(n.router)
	defer server.Close()

	url := fmt.Sprintf("%s/%s", server.URL, "searches/"+username)
	res, err := http.Post(url, "", strings.NewReader(expectedBody))
	if err != nil {
		t.Error(err)
	}

	bodyBytes, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Error(err)
	}

	var parsed map[string]map[string]string
	err = json.Unmarshal(bodyBytes, &parsed)
	if err != nil {
		t.Error(err)
	}

	var expectedParsed map[string]string
	err = json.Unmarshal([]byte(expectedBody), &expectedParsed)
	if err != nil {
		t.Error(err)
	}

	if _, ok := parsed["saved_searches"]; !ok {
		t.Error("Parsed response did not have a top-level 'saved_searches' key")
	}

	if !reflect.DeepEqual(parsed["saved_searches"], expectedParsed) {
		t.Errorf("Post returned '%#v' as the saved search instead of '%#v'", parsed["saved_searches"], expectedBody)
	}
}

func TestPostUpdateSavedSearchesForRequest(t *testing.T) {
	username := "test_user@test-domain.org"
	expectedBody := `{"search":"fake"}`
	ctx := context.Background()

	mock := NewMockDB()
	mock.users[username] = true
	if err := mock.insertSavedSearches(ctx, username, expectedBody); err != nil {
		t.Error(err)
	}

	router := mux.NewRouter()
	n := NewSearchesApp(mock, router)
	server := httptest.NewServer(n.router)
	defer server.Close()

	url := fmt.Sprintf("%s/%s", server.URL, "searches/"+username)
	res, err := http.Post(url, "", strings.NewReader(expectedBody))
	if err != nil {
		t.Error(err)
	}

	bodyBytes, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Error(err)
	}

	var parsed map[string]map[string]string
	err = json.Unmarshal(bodyBytes, &parsed)
	if err != nil {
		t.Error(err)
	}

	var expectedParsed map[string]string
	err = json.Unmarshal([]byte(expectedBody), &expectedParsed)
	if err != nil {
		t.Error(err)
	}

	if _, ok := parsed["saved_searches"]; !ok {
		t.Error("Parsed response did not have a top-level 'saved_searches' key")
	}

	if !reflect.DeepEqual(parsed["saved_searches"], expectedParsed) {
		t.Errorf("Post returned '%#v' as the saved search instead of '%#v'", parsed["saved_searches"], expectedBody)
	}
}

func TestDeleteSavedSearchesForRequest(t *testing.T) {
	username := "test_user@test-domain.org"
	expectedBody := `{"search":"fake"}`
	ctx := context.Background()

	mock := NewMockDB()
	mock.users[username] = true
	if err := mock.insertSavedSearches(ctx, username, expectedBody); err != nil {
		t.Error(err)
	}

	router := mux.NewRouter()
	n := NewSearchesApp(mock, router)
	server := httptest.NewServer(n.router)
	defer server.Close()

	url := fmt.Sprintf("%s/%s", server.URL, "searches/"+username)
	httpClient := &http.Client{}
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Error(err)
	}
	res, err := httpClient.Do(req)
	if err != nil {
		t.Error(err)
	}

	bodyBytes, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Error(err)
	}

	if len(bodyBytes) > 0 {
		t.Errorf("Delete returned a body when it should not have: %s", string(bodyBytes))
	}

	expectedStatus := http.StatusOK
	actualStatus := res.StatusCode

	if actualStatus != expectedStatus {
		t.Errorf("StatusCode was %d instead of %d", actualStatus, expectedStatus)
	}
}

func TestNewSearchesDB(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error occurred creating the mock db: %s", err)
	}
	defer db.Close()

	prefs := NewSearchesDB(db)
	if prefs == nil {
		t.Fatal("NewSearchesDB() returned nil")
	}

	if prefs.db != db {
		t.Error("dbs did not match")
	}
}

func TestSavedSearchesIsUser(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("error creating the mock db: %s", err)
	}
	defer db.Close()

	p := NewSearchesDB(db)
	if p == nil {
		t.Error("NewSearchesDB returned nil")
	}

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM \\( SELECT DISTINCT id FROM users").
		WithArgs("test-user").
		WillReturnRows(sqlmock.NewRows([]string{"check_user"}).AddRow(1))

	present, err := p.isUser(context.Background(), "test-user")
	if err != nil {
		t.Errorf("error calling isUser(): %s", err)
	}

	if !present {
		t.Error("test-user was not found")
	}
}

func TestHasSavedSearches(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("error creating the mock db: %s", err)
	}
	defer db.Close()

	p := NewSearchesDB(db)
	if p == nil {
		t.Error("NewSearchesDB returned nil")
	}

	mock.ExpectQuery("SELECT EXISTS\\( SELECT 1 FROM user_saved_searches s").
		WithArgs("test-user").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	exists, err := p.hasSavedSearches(context.Background(), "test-user")
	if err != nil {
		t.Errorf("error from hasSavedSearches(): %s", err)
	}

	if !exists {
		t.Error("hasSavedSearches() returned false")
	}

	if err = mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations were not met: %s", err)
	}
}

func TestGetSavedSearches(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("error creating the mock db: %s", err)
	}
	defer db.Close()

	p := NewSearchesDB(db)
	if p == nil {
		t.Error("NewSearchesDB returned nil")
	}

	mock.ExpectQuery("SELECT s.saved_searches saved_searches FROM user_saved_searches s,").
		WithArgs("test-user").
		WillReturnRows(sqlmock.NewRows([]string{"saved_searches"}).AddRow("{}"))

	retval, err := p.getSavedSearches(context.Background(), "test-user")
	if err != nil {
		t.Errorf("error from getSavedSearches(): %s", err)
	}

	if err = mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations were not met: %s", err)
	}

	if len(retval) != 1 {
		t.Errorf("length of retval was not 1: %d", len(retval))
	}

	if retval[0] != "{}" {
		t.Errorf("retval was %s instead of {}", retval)
	}
}

func TestInsertSavedSearches(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("error creating the mock db: %s", err)
	}
	defer db.Close()

	p := NewSearchesDB(db)
	if p == nil {
		t.Error("NewSearchesDB returned nil")
	}

	mock.ExpectQuery("SELECT id FROM users WHERE username =").
		WithArgs("test-user").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("1"))

	mock.ExpectExec("INSERT INTO user_saved_searches \\(user_id").
		WithArgs("1", "{}").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := p.insertSavedSearches(context.Background(), "test-user", "{}"); err != nil {
		t.Errorf("error inserting saved searches: %s", err)
	}

	if err = mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations were not met: %s", err)
	}
}

func TestUpdateSavedSearches(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("error creating the mock db: %s", err)
	}
	defer db.Close()

	p := NewSearchesDB(db)
	if p == nil {
		t.Error("NewSearchesDB returned nil")
	}

	mock.ExpectQuery("SELECT id FROM users WHERE username =").
		WithArgs("test-user").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("1"))

	mock.ExpectExec("UPDATE ONLY user_saved_searches SET saved_searches =").
		WithArgs("1", "{}").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := p.updateSavedSearches(context.Background(), "test-user", "{}"); err != nil {
		t.Errorf("error updating saved searches: %s", err)
	}

	if err = mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations were not met: %s", err)
	}
}

func TestDeleteSavedSearches(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("error creating the mock db: %s", err)
	}
	defer db.Close()

	p := NewSearchesDB(db)
	if p == nil {
		t.Error("NewSearchesDB returned nil")
	}

	mock.ExpectQuery("SELECT id FROM users WHERE username =").
		WithArgs("test-user").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("1"))

	mock.ExpectExec("DELETE FROM ONLY user_saved_searches WHERE user_id").
		WithArgs("1").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := p.deleteSavedSearches(context.Background(), "test-user"); err != nil {
		t.Errorf("error deleting saved searches: %s", err)
	}

	if err = mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations were not met: %s", err)
	}
}

// -------- End Searches --------

func TestFixAddrNoPrefix(t *testing.T) {
	expected := ":70000"
	actual := fixAddr("70000")
	if actual != expected {
		t.Fail()
	}
}

func TestFixAddrWithPrefix(t *testing.T) {
	expected := ":70000"
	actual := fixAddr(":70000")
	if actual != expected {
		t.Fail()
	}
}

func TestBadRequest(t *testing.T) {
	var (
		expectedMsg    = "test message\n"
		expectedStatus = http.StatusBadRequest
	)

	recorder := httptest.NewRecorder()
	badRequest(recorder, "test message")
	actualMsg := recorder.Body.String()
	actualStatus := recorder.Code

	if actualStatus != expectedStatus {
		t.Errorf("Status code was %d but should have been %d", actualStatus, expectedStatus)
	}

	if actualMsg != expectedMsg {
		t.Errorf("Message was '%s' but should have been '%s'", actualMsg, expectedMsg)
	}
}

func TestErrored(t *testing.T) {
	var (
		expectedMsg    = "test message\n"
		expectedStatus = http.StatusInternalServerError
	)

	recorder := httptest.NewRecorder()
	errored(recorder, "test message")
	actualMsg := recorder.Body.String()
	actualStatus := recorder.Code

	if actualStatus != expectedStatus {
		t.Errorf("Status code was %d but should have been %d", actualStatus, expectedStatus)
	}

	if actualMsg != expectedMsg {
		t.Errorf("Message was '%s' but should have been '%s'", actualMsg, expectedMsg)
	}
}

func TestDeleteUnstored(t *testing.T) {
	username := "test-user"
	mock := NewMockDB()
	mock.users[username] = true
	router := mux.NewRouter()
	np := NewPrefsApp(mock, router)
	ns1 := NewSessionsApp(mock, router)
	ns2 := NewSearchesApp(mock, router)

	serverPrefs := httptest.NewServer(np.router)
	serverSessions := httptest.NewServer(ns1.router)
	serverSearches := httptest.NewServer(ns2.router)
	defer serverPrefs.Close()
	defer serverSessions.Close()
	defer serverSearches.Close()

	urlPrefs := fmt.Sprintf("%s/%s", serverPrefs.URL, "preferences/"+username)
	urlSessions := fmt.Sprintf("%s/%s", serverSessions.URL, "sessions/"+username)
	urlSearches := fmt.Sprintf("%s/%s", serverSearches.URL, "searches/"+username)
	httpClient := &http.Client{}
	reqPrefs, errPrefs := http.NewRequest(http.MethodDelete, urlPrefs, nil)
	if errPrefs != nil {
		t.Error(errPrefs)
	}
	reqSessions, errSessions := http.NewRequest(http.MethodDelete, urlSessions, nil)
	if errSessions != nil {
		t.Error(errSessions)
	}
	reqSearches, errSearches := http.NewRequest(http.MethodDelete, urlSearches, nil)
	if errSearches != nil {
		t.Error(errSearches)
	}

	resPrefs, errPrefs := httpClient.Do(reqPrefs)
	if errPrefs != nil {
		t.Error(errPrefs)
	}
	resSessions, errSessions := httpClient.Do(reqSessions)
	if errSessions != nil {
		t.Error(errSessions)
	}
	resSearches, errSearches := httpClient.Do(reqSearches)
	if errSearches != nil {
		t.Error(errSearches)
	}

	bodyPrefs, errPrefs := ioutil.ReadAll(resPrefs.Body)
	if errPrefs != nil {
		t.Error(errPrefs)
	}
	resPrefs.Body.Close()

	bodySessions, errSessions := ioutil.ReadAll(resSessions.Body)
	if errSessions != nil {
		t.Error(errSessions)
	}
	resSessions.Body.Close()

	bodySearches, errSearches := ioutil.ReadAll(resSearches.Body)
	if errSearches != nil {
		t.Error(errSearches)
	}
	resSearches.Body.Close()

	if len(bodyPrefs) > 0 {
		t.Errorf("DELETE returned a body: %s", bodyPrefs)
	}
	if len(bodySessions) > 0 {
		t.Errorf("DELETE returned a body: %s", bodySessions)
	}
	if len(bodySearches) > 0 {
		t.Errorf("DELETE returned a body: %s", bodySearches)
	}

	expectedStatus := http.StatusOK
	actualStatusPrefs := resPrefs.StatusCode
	actualStatusSessions := resSessions.StatusCode
	actualStatusSearches := resSearches.StatusCode

	if actualStatusPrefs != expectedStatus {
		t.Errorf("DELETE status code was %d instead of %d", actualStatusPrefs, expectedStatus)
	}
	if actualStatusSessions != expectedStatus {
		t.Errorf("DELETE status code was %d instead of %d", actualStatusSessions, expectedStatus)
	}
	if actualStatusSearches != expectedStatus {
		t.Errorf("DELETE status code was %d instead of %d", actualStatusSearches, expectedStatus)
	}
}

func TestRootGreeting(t *testing.T) {
	router := makeRouter()
	router.Handle("/debug/vars", http.DefaultServeMux)

	server := httptest.NewServer(router)
	defer server.Close()

	url := fmt.Sprintf("%s/%s", server.URL, "/")
	res, err := http.Get(url)
	if err != nil {
		t.Log(url)
		t.Log(res)
		t.Error(err)
	}

	expectedBody := []byte("Hello from user-info.\n")
	actualBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Log(url)
		t.Error(err)
	}
	res.Body.Close()

	if !bytes.Equal(actualBody, expectedBody) {
		t.Errorf("Message was '%s' but should have been '%s'", actualBody, expectedBody)
	}

	expectedStatus := http.StatusOK
	actualStatus := res.StatusCode

	if actualStatus != expectedStatus {
		t.Errorf("Status code was %d but should have been %d", actualStatus, expectedStatus)
	}
}

func TestGetAllAlerts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	router := mux.NewRouter()
	alertsApp := NewAlertsApp(NewAlertsDB(db), router)

	rows := sqlmock.NewRows([]string{"start_date", "end_date", "alert"}).
		AddRow(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), "test alert")

	mock.ExpectQuery("SELECT start_date, end_date, alert FROM global_alerts").
		WillReturnRows(rows)

	server := httptest.NewServer(alertsApp.router)
	defer server.Close()

	url := fmt.Sprintf("%s/%s", server.URL, "alerts/all")
	res, err := http.Get(url)
	if err != nil {
		t.Error(err)
	}

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK but got %v", res.StatusCode)
	}

	var alerts []GlobalAlertRecord
	err = json.NewDecoder(res.Body).Decode(&alerts)
	if err != nil {
		t.Error(err)
	}

	if len(alerts) != 1 {
		t.Errorf("Expected 1 alert but got %d", len(alerts))
	}

	if alerts[0].Alert != "test alert" {
		t.Errorf("Expected alert text 'test alert' but got '%s'", alerts[0].Alert)
	}
}

func TestCreateAlert(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	router := mux.NewRouter()
	alertsApp := NewAlertsApp(NewAlertsDB(db), router)

	mock.ExpectExec("INSERT INTO global_alerts").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "test alert").
		WillReturnResult(sqlmock.NewResult(1, 1))

	server := httptest.NewServer(alertsApp.router)
	defer server.Close()

	alert := GlobalAlertRecord{
		StartDate: time.Now(),
		EndDate:   time.Now().Add(24 * time.Hour),
		Alert:     "test alert",
	}

	body, err := json.Marshal(alert)
	if err != nil {
		t.Fatal(err)
	}

	url := fmt.Sprintf("%s/%s", server.URL, "alerts/")
	res, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Error(err)
	}

	if res.StatusCode != http.StatusCreated {
		t.Errorf("Expected status Created but got %v", res.StatusCode)
	}
}

func TestDeleteAlert(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	router := mux.NewRouter()
	alertsApp := NewAlertsApp(NewAlertsDB(db), router)

	mock.ExpectExec("DELETE FROM ONLY global_alerts").
		WithArgs(sqlmock.AnyArg(), "test alert").
		WillReturnResult(sqlmock.NewResult(1, 1))

	server := httptest.NewServer(alertsApp.router)
	defer server.Close()

	params := struct {
		EndDate time.Time `json:"end_date"`
		Alert   string    `json:"alert"`
	}{
		EndDate: time.Now(),
		Alert:   "test alert",
	}

	body, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}

	url := fmt.Sprintf("%s/%s", server.URL, "alerts/")
	req, err := http.NewRequest(http.MethodDelete, url, bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Error(err)
	}

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK but got %v", res.StatusCode)
	}
}
