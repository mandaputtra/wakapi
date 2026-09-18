package imports

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/muety/wakapi/config"
	"github.com/muety/wakapi/models"
	"github.com/stretchr/testify/suite"
)

type WakatimeImporterTestSuite struct {
	suite.Suite
	conf *config.Config
}

func TestWakatimeImporterTestSuite(t *testing.T) {
	suite.Run(t, new(WakatimeImporterTestSuite))
}

func (suite *WakatimeImporterTestSuite) SetupTest() {
	suite.conf = config.Empty()
	suite.conf.Server.PublicNetUrl, _ = url.Parse("https://wakapi.dev")
	config.Set(suite.conf)
}

func (suite *WakatimeImporterTestSuite) TestCheckUrl() {
	importer := NewWakatimeImporter("test-key", false)

	testCases := []struct {
		name      string
		whitelist []string
		url       string
		wantErr   bool
		errText   string
	}{
		{
			name:      "no whitelist - allowed",
			whitelist: []string{},
			url:       "https://api.wakatime.com/api/v1",
			wantErr:   false,
			errText:   "",
		},
		{
			name:      "on whitelist - allowed",
			whitelist: []string{"wakatime.com"},
			url:       "https://wakatime.com/api/v1",
			wantErr:   false,
			errText:   "",
		},
		{
			name:      "on whitelist wildcard - allowed",
			whitelist: []string{"*.wakatime.com"},
			url:       "https://api.wakatime.com/api/v1",
			wantErr:   false,
			errText:   "",
		},
		{
			name:      "not on whitelist - denied",
			whitelist: []string{"wakatime.com"},
			url:       "https://evil.com/api/v1",
			wantErr:   true,
			errText:   "not allowed",
		},
		{
			name:      "on whitelist, but private - not allowed",
			whitelist: []string{"wakatime.com"},
			url:       "https://localhost:3000/api/v1",
			wantErr:   true,
			errText:   "cannot use private ip",
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.conf.App.ImportHostsWhitelist = tc.whitelist

			err := importer.Validate(&models.User{WakatimeApiUrl: tc.url})
			if tc.wantErr {
				suite.Error(err)
				suite.Contains(err.Error(), tc.errText)
			} else {
				suite.NoError(err)
			}
		})
	}
}

func (suite *WakatimeImporterTestSuite) TestRedirectValidation() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, r.URL.Query().Get("target"), http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	importers := []struct {
		name       string
		httpClient *http.Client
	}{
		{"heartbeats importer", NewWakatimeHeartbeatImporter("test-key").httpClient},
		{"dump importer", NewWakatimeDumpImporter("test-key").httpClient},
	}

	testCases := []struct {
		name    string
		env     string
		target  string
		wantErr bool
		errText string
	}{
		{
			name:    "redirect to own instance - denied",
			env:     "dev",
			target:  "https://wakapi.dev/api",
			wantErr: true,
			errText: "cannot use reference to own instance",
		},
		{
			name:    "redirect to private ip - denied",
			env:     "prod",
			target:  "https://127.0.0.1/api",
			wantErr: true,
			errText: "cannot use private ip",
		},
		{
			name:    "redirect to raw ip - denied",
			env:     "prod",
			target:  "https://8.8.8.8/api",
			wantErr: true,
			errText: "cannot use raw ip",
		},
		{
			name:    "redirect to loopback test server in dev - allowed",
			env:     "dev",
			target:  srv.URL + "/done",
			wantErr: false,
		},
	}

	for _, importer := range importers {
		for _, t := range testCases {
			suite.Run(importer.name+" - "+t.name, func() {
				suite.conf.Env = t.env
				config.Set(suite.conf)

				req, err := http.NewRequest(http.MethodGet, srv.URL+"/start?target="+url.QueryEscape(t.target), nil)
				suite.Require().NoError(err)

				res, err := importer.httpClient.Do(req)

				if t.wantErr {
					suite.Error(err)
					suite.Contains(err.Error(), t.errText)
				} else {
					suite.Require().NoError(err)
					defer res.Body.Close()
					suite.Equal(http.StatusOK, res.StatusCode)
				}
			})
		}
	}
}
