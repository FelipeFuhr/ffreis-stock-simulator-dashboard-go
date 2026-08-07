package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleHealthz_ReturnsOKStatus(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	handleHealthz(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("body = %v, want status=ok", body)
	}
}

func TestIsSwaggerAuthorized(t *testing.T) {
	cases := []struct {
		name    string
		token   string
		headers map[string]string
		want    bool
	}{
		{
			name: "no token configured allows any request",
			want: true,
		},
		{
			name:    "matching X-Swagger-Token header",
			token:   "secret",
			headers: map[string]string{"X-Swagger-Token": "secret"},
			want:    true,
		},
		{
			name:    "matching bearer authorization header",
			token:   "secret",
			headers: map[string]string{"Authorization": "Bearer secret"},
			want:    true,
		},
		{
			name:    "bearer scheme is case-insensitive",
			token:   "secret",
			headers: map[string]string{"Authorization": "bearer secret"},
			want:    true,
		},
		{
			name:    "mismatched token rejected",
			token:   "secret",
			headers: map[string]string{"X-Swagger-Token": "wrong"},
			want:    false,
		},
		{
			name:  "missing credentials rejected when token configured",
			token: "secret",
			want:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &app{swaggerToken: tc.token}
			req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			if got := a.isSwaggerAuthorized(req); got != tc.want {
				t.Fatalf("isSwaggerAuthorized = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAuthorizeSwaggerOr401_UnauthorizedSetsChallengeHeader(t *testing.T) {
	a := &app{swaggerToken: "secret"}
	req := httptest.NewRequest(http.MethodGet, "/swagger", nil)
	rr := httptest.NewRecorder()

	if a.authorizeSwaggerOr401(rr, req) {
		t.Fatal("expected authorizeSwaggerOr401 to return false")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	if rr.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("expected WWW-Authenticate header to be set")
	}
}

func TestAuthorizeSwaggerOr401_AuthorizedReturnsTrueWithoutWriting(t *testing.T) {
	a := &app{swaggerToken: "secret"}
	req := httptest.NewRequest(http.MethodGet, "/swagger", nil)
	req.Header.Set("X-Swagger-Token", "secret")
	rr := httptest.NewRecorder()

	if !a.authorizeSwaggerOr401(rr, req) {
		t.Fatal("expected authorizeSwaggerOr401 to return true")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want unwritten 200 default", rr.Code)
	}
}

func TestHandleOpenAPI_ReturnsSpecWhenAuthorized(t *testing.T) {
	a := &app{openAPISpec: []byte("openapi: 3.0.0")}
	req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	rr := httptest.NewRecorder()

	a.handleOpenAPI(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if rr.Body.String() != "openapi: 3.0.0" {
		t.Fatalf("body = %q, want spec passthrough", rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/yaml") {
		t.Fatalf("Content-Type = %q, want application/yaml", ct)
	}
}

func TestHandleOpenAPI_RequiresAuthWhenTokenSet(t *testing.T) {
	a := &app{openAPISpec: []byte("openapi: 3.0.0"), swaggerToken: "secret"}
	req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	rr := httptest.NewRecorder()

	a.handleOpenAPI(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 without credentials", rr.Code)
	}
}

func TestHandleSwaggerUI_ReturnsHTMLWhenAuthorized(t *testing.T) {
	a := &app{}
	req := httptest.NewRequest(http.MethodGet, "/swagger", nil)
	rr := httptest.NewRecorder()

	a.handleSwaggerUI(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", ct)
	}
	if !strings.Contains(rr.Body.String(), "Stock Dashboard OpenAPI") {
		t.Fatalf("body missing expected title, got: %s", rr.Body.String())
	}
}

func TestHandleSwaggerUI_RequiresAuthWhenTokenSet(t *testing.T) {
	a := &app{swaggerToken: "secret"}
	req := httptest.NewRequest(http.MethodGet, "/swagger", nil)
	rr := httptest.NewRecorder()

	a.handleSwaggerUI(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 without credentials", rr.Code)
	}
}

func TestToEncodedAction(t *testing.T) {
	limitPrice := 123.45

	cases := []struct {
		name    string
		in      stepFormRequest
		want    encodedAction
		wantErr string
	}{
		{
			name: "defaults to hold market when side and order_type are empty",
			in:   stepFormRequest{},
			want: encodedAction{SideCode: 0, Units: 0, OrderTypeCode: 0},
		},
		{
			name: "buy market with units",
			in:   stepFormRequest{Side: "buy", OrderType: "market", Units: 10},
			want: encodedAction{SideCode: 1, Units: 10, OrderTypeCode: 0},
		},
		{
			name: "sell limit requires and carries limit price",
			in:   stepFormRequest{Side: "sell", OrderType: "limit", Units: 5, LimitPrice: &limitPrice},
			want: encodedAction{SideCode: -1, Units: 5, OrderTypeCode: 1, HasLimitPrice: true, LimitPrice: &limitPrice},
		},
		{
			name:    "invalid side rejected",
			in:      stepFormRequest{Side: "hodl"},
			wantErr: "side must be one of",
		},
		{
			name:    "invalid order_type rejected",
			in:      stepFormRequest{Side: "buy", OrderType: "stop", Units: 1},
			wantErr: "order_type must be one of",
		},
		{
			name:    "negative units rejected",
			in:      stepFormRequest{Side: "buy", Units: -1},
			wantErr: "units must be >= 0",
		},
		{
			name:    "zero units rejected for buy",
			in:      stepFormRequest{Side: "buy", Units: 0},
			wantErr: "units must be > 0",
		},
		{
			name:    "limit order without limit_price rejected",
			in:      stepFormRequest{Side: "buy", OrderType: "limit", Units: 1},
			wantErr: "limit_price is required",
		},
		{
			name: "hold forces zero units and clears limit price even if provided",
			in:   stepFormRequest{Side: "hold", OrderType: "limit", Units: 10, LimitPrice: &limitPrice},
			want: encodedAction{SideCode: 0, Units: 0, OrderTypeCode: 0},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := toEncodedAction(tc.in)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.SideCode != tc.want.SideCode || got.Units != tc.want.Units || got.OrderTypeCode != tc.want.OrderTypeCode || got.HasLimitPrice != tc.want.HasLimitPrice {
				t.Fatalf("toEncodedAction = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestHandleStep_ForwardsEncodedActionAndReturnsUpstreamBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/step_many" {
			http.NotFound(w, r)
			return
		}
		var payload stepManyRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("upstream: decode step_many payload: %v", err)
		}
		if len(payload.Actions) != 1 || payload.Actions[0].SideCode != 1 {
			t.Errorf("upstream: unexpected actions payload: %+v", payload.Actions)
		}
		writeRawJSON(w, http.StatusOK, []byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	a := newTestApp(upstream)
	dashboard := newDashboardServer(a)
	defer dashboard.Close()

	body := []byte(`{"side":"buy","units":3,"order_type":"market"}`)
	resp, respBody := mustRequest(t, dashboard.URL, http.MethodPost, "/api/step", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, respBody)
	}
	if !strings.Contains(string(respBody), `"ok":true`) {
		t.Fatalf("unexpected body: %s", respBody)
	}
}

func TestHandleStep_InvalidJSON_Returns400(t *testing.T) {
	a := &app{maxBodyBytes: 1 << 20}
	req := httptest.NewRequest(http.MethodPost, "/api/step", strings.NewReader(`{not-json}`))
	rr := httptest.NewRecorder()

	a.handleStep(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHandleStep_OversizedBody_Returns413(t *testing.T) {
	a := &app{maxBodyBytes: 4}
	req := httptest.NewRequest(http.MethodPost, "/api/step", strings.NewReader(`{"side":"buy","units":3}`))
	rr := httptest.NewRecorder()

	a.handleStep(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rr.Code)
	}
}

func TestHandleStep_InvalidSide_Returns400(t *testing.T) {
	a := &app{maxBodyBytes: 1 << 20}
	req := httptest.NewRequest(http.MethodPost, "/api/step", strings.NewReader(`{"side":"nonsense"}`))
	rr := httptest.NewRecorder()

	a.handleStep(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHandleReset_Success_ForwardsToUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/reset" {
			http.NotFound(w, r)
			return
		}
		writeRawJSON(w, http.StatusOK, []byte(`{"reset":true}`))
	}))
	defer upstream.Close()

	a := newTestApp(upstream)
	dashboard := newDashboardServer(a)
	defer dashboard.Close()

	resp, body := mustRequest(t, dashboard.URL, http.MethodPost, "/api/reset", []byte(`{"seed":42}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"reset":true`) {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestHandleState_EngineReady_IncludesObservation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/readyz":
			writeRawJSON(w, http.StatusOK, []byte(`{"status":"ready","engine_enabled":true,"engine_ready":true}`))
		case "/v1/observe":
			writeRawJSON(w, http.StatusOK, []byte(`{"observation":{"market_window_handle":{"start":0,"end":10,"t":1,"current_price":100.5},"portfolio_vector":[1,2],"order_summary_vector":[0],"done":false}}`))
		case "/v1/flows":
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	a := newTestApp(upstream)
	dashboard := newDashboardServer(a)
	defer dashboard.Close()

	resp, body := mustRequest(t, dashboard.URL, http.MethodGet, "/api/state", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var env stateEnvelope
	mustUnmarshal(t, body, &env)
	if env.Observation == nil {
		t.Fatal("expected observation to be populated when engine is ready")
	}
	if env.Observation.MarketWindowHandle.CurrentPrice != 100.5 {
		t.Fatalf("unexpected observation: %+v", env.Observation)
	}
}

func TestHandleState_ReadyzServerError_ReturnsBadGatewayWithLastError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/readyz" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	a := newTestApp(upstream)
	a.upstream.MaxAttempts = 1
	dashboard := newDashboardServer(a)
	defer dashboard.Close()

	resp, body := mustRequest(t, dashboard.URL, http.MethodGet, "/api/state", nil)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502, body = %s", resp.StatusCode, body)
	}
	var env stateEnvelope
	mustUnmarshal(t, body, &env)
	if env.LastError == "" {
		t.Fatal("expected LastError to be populated on readyz failure")
	}
}

func TestHandleState_ObserveError_ReturnsBadGatewayWithLastError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/readyz":
			writeRawJSON(w, http.StatusOK, []byte(`{"status":"ready","engine_enabled":true,"engine_ready":true}`))
		case "/v1/observe":
			w.WriteHeader(http.StatusBadRequest)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	a := newTestApp(upstream)
	a.upstream.MaxAttempts = 1
	dashboard := newDashboardServer(a)
	defer dashboard.Close()

	resp, body := mustRequest(t, dashboard.URL, http.MethodGet, "/api/state", nil)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502, body = %s", resp.StatusCode, body)
	}
	var env stateEnvelope
	mustUnmarshal(t, body, &env)
	if env.LastError == "" {
		t.Fatal("expected LastError to be populated when /v1/observe errors")
	}
}
