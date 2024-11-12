package fixture

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	respondsWithin = 5 * time.Second
	respondsTick   = 20 * time.Millisecond
)

// AssertJSONResponse asserts that the recorded response has the expected status code and JSON body.
func AssertJSONResponse(t *testing.T, expectedCode int, expectedBody any, rec *httptest.ResponseRecorder) {
	t.Helper()

	data, err := json.Marshal(expectedBody)
	require.NoError(t, err)
	AssertRawJSONResponse(t, expectedCode, string(data), rec)
}

// AssertRawJSONResponse asserts that the recorded response has the expected status code and raw JSON body.
func AssertRawJSONResponse(t *testing.T, expectedCode int, expectedJSON string, rec *httptest.ResponseRecorder) {
	t.Helper()

	assert.Equal(t, expectedCode, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.JSONEq(t, expectedJSON, rec.Body.String())
}

// AssertResponse asserts that the recorded response has the expected status code, content type, and body.
func AssertResponse(t *testing.T, expectedCode int, expectedContentType, expectedBody string,
	rec *httptest.ResponseRecorder,
) {
	t.Helper()

	assert.Equal(t, expectedCode, rec.Code)
	assert.Equal(t, expectedContentType, rec.Header().Get("Content-Type"))
	assert.Equal(t, expectedBody, rec.Body.String())
}

// AssertEventuallyResponds polls url with GET requests until it answers with the expected status code.
// Use it to wait for a server started in the background to become reachable.
func AssertEventuallyResponds(t *testing.T, url string, expectedCode int) {
	t.Helper()

	assert.Eventually(t, func() bool { return statusCodeOf(url) == expectedCode }, respondsWithin, respondsTick,
		"%s never answered with status code %d", url, expectedCode)
}

// statusCodeOf returns the status code of a GET request on url, or 0 when the request fails.
func statusCodeOf(url string) int {
	ctx, cancel := context.WithTimeout(context.Background(), respondsWithin)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0
	}
	defer func() { _ = response.Body.Close() }()
	return response.StatusCode
}
