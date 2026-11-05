package oauthflow

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Keep server-output security assertions independent of loopback middleware
// that can rewrite HTML outside Mender. Network callback and token exchange
// remain covered by the OAuth integration tests.
func TestCallbackPagesNeverEchoInput(t *testing.T) {
	t.Parallel()
	const state = "PRIVATE_EXPECTED_STATE"
	const code = "code-PRIVATE_AUTHORIZATION_CODE"
	const issuer = "https://issuer.invalid/PRIVATE_ISSUER"
	for _, tc := range []struct {
		name, method, path, suppliedState, suppliedCode, upstreamError string
		status                                                         int
		page                                                           string
		delivered, failed                                              bool
	}{
		{name: "success", suppliedState: state, suppliedCode: code, status: http.StatusOK, page: pageSuccessHTML, delivered: true},
		{name: "forged state", suppliedState: "forged-by-attacker", suppliedCode: code, status: http.StatusBadRequest, page: pageFailureHTML, failed: true},
		{name: "missing code", suppliedState: state, status: http.StatusOK, page: pageFailureHTML, failed: true},
		{name: "upstream error", suppliedState: state, upstreamError: "<script>PRIVATE_ERROR</script>", status: http.StatusOK, page: pageFailureHTML, failed: true},
		{name: "unknown path", path: "/PRIVATE_UNKNOWN", suppliedState: state, suppliedCode: code, status: http.StatusNotFound, page: pageUnknownHTML},
		{name: "wrong method", method: http.MethodPost, suppliedState: state, suppliedCode: code, status: http.StatusNotFound, page: pageUnknownHTML},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			f := &flow{ctx: ctx, cancel: cancel, now: time.Now, status: FlowPending, expectedState: state, resultCh: make(chan callbackResult, 1)}
			path, method := tc.path, tc.method
			if path == "" {
				path = callbackPath
			}
			if method == "" {
				method = http.MethodGet
			}
			query := url.Values{
				"state": {tc.suppliedState}, "code": {tc.suppliedCode}, "iss": {issuer}, "error": {tc.upstreamError},
				"access_token": {testAccessToken}, "refresh_token": {testRefreshToken},
			}
			request := httptest.NewRequest(method, "http://127.0.0.1"+path+"?"+query.Encode(), nil)
			recorder := httptest.NewRecorder()
			f.serveCallback(recorder, request)
			if recorder.Code != tc.status || recorder.Body.String() != tc.page {
				t.Fatalf("callback output = %d %q, want %d %q", recorder.Code, recorder.Body.String(), tc.status, tc.page)
			}
			for _, forbidden := range []string{state, code, issuer, "forged-by-attacker", "PRIVATE_ERROR", testAccessToken, testRefreshToken} {
				if strings.Contains(recorder.Body.String(), forbidden) {
					t.Fatalf("callback output contains private input %q", forbidden)
				}
			}
			for name, expected := range map[string]string{
				"Content-Type": "text/html; charset=utf-8", "Cache-Control": "no-store", "X-Content-Type-Options": "nosniff",
				"Content-Length": strconv.Itoa(len(tc.page)),
			} {
				if actual := recorder.Header().Get(name); actual != expected {
					t.Fatalf("%s = %q, want %q", name, actual, expected)
				}
			}
			select {
			case result := <-f.resultCh:
				if !tc.delivered || result.code != code || result.iss != issuer {
					t.Fatalf("unexpected authorization result: %+v", result)
				}
			default:
				if tc.delivered {
					t.Fatal("valid callback did not deliver authorization result")
				}
			}
			if (f.currentStatus() == FlowFailed) != tc.failed {
				t.Fatalf("failure state = %s, failed=%v", f.currentStatus(), tc.failed)
			}
		})
	}
}

type observedCallbackWriter struct {
	*httptest.ResponseRecorder
	onFlush func()
}

func (w observedCallbackWriter) Flush() {
	w.onFlush()
	w.ResponseRecorder.Flush()
}

func TestCallbackFlushesBeforeEndingFlow(t *testing.T) {
	t.Parallel()
	for _, state := range []string{"expected-state", "forged-state"} {
		t.Run(state, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			f := &flow{ctx: ctx, cancel: cancel, now: time.Now, status: FlowPending, expectedState: "expected-state", resultCh: make(chan callbackResult, 1)}
			flushed := 0
			writer := observedCallbackWriter{
				ResponseRecorder: httptest.NewRecorder(),
				onFlush: func() {
					flushed++
					if f.currentStatus() != FlowPending || ctx.Err() != nil || len(f.resultCh) != 0 {
						t.Fatal("flow could close its listener before the callback response was flushed")
					}
				},
			}
			request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/callback?state="+state+"&code=private-code", nil)
			f.serveCallback(writer, request)
			if flushed != 1 || !writer.Flushed {
				t.Fatalf("callback response flushes = %d, want one before terminal signaling", flushed)
			}
		})
	}
}

type disconnectedCallbackWriter struct{ header http.Header }

func (w disconnectedCallbackWriter) Header() http.Header     { return w.header }
func (disconnectedCallbackWriter) WriteHeader(int)           {}
func (disconnectedCallbackWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func (disconnectedCallbackWriter) FlushError() error         { return io.ErrClosedPipe }

func TestBrowserWriteFailureDoesNotChangeCallbackAuthorization(t *testing.T) {
	for _, state := range []string{"expected-state", "forged-state"} {
		t.Run(state, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			f := &flow{ctx: ctx, cancel: cancel, now: time.Now, status: FlowPending, expectedState: "expected-state", resultCh: make(chan callbackResult, 1)}
			f.serveCallback(disconnectedCallbackWriter{header: make(http.Header)},
				httptest.NewRequest(http.MethodGet, "http://127.0.0.1/callback?state="+state+"&code=private-code", nil))
			if state == "expected-state" {
				if len(f.resultCh) != 1 || f.currentStatus() != FlowPending {
					t.Fatal("browser write failure lost a validated authorization code")
				}
			} else if len(f.resultCh) != 0 || f.currentStatus() != FlowFailed || ctx.Err() == nil {
				t.Fatal("browser write failure changed invalid-state rejection")
			}
		})
	}
}
