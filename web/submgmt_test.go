//
// Copyright (C) 2025 Eaton
//
// SPDX-License-Identifier: Apache-2.0
//

package web

import (
	"bytes"
	"github.com/edgexfoundry-holding/edgex-sse/configuration"
	"github.com/edgexfoundry-holding/edgex-sse/interfaces"
	"github.com/edgexfoundry-holding/edgex-sse/submgr"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/edgexfoundry/go-mod-core-contracts/v4/clients/logger"
	commonDTO "github.com/edgexfoundry/go-mod-core-contracts/v4/dtos/common"
	"github.com/labstack/echo/v4"
)

type subCreateResponse struct {
	commonDTO.BaseResponse `json:",inline"`
	SubscriptionId         string `json:"subscriptionId"`
}

type subInfoResponse struct {
	commonDTO.BaseResponse `json:",inline"`
	Include                []string `json:"include"`
	Exclude                []string `json:"exclude"`
}

const sub_limit = 4
const incexc_limit = 3
const buffer = 25
const ageout = 90*time.Second
const ageout_check = 10*time.Second
const uri_base = "/api/v3/subscription"

func managerInit() {
	interfaces.App.Config = &configuration.Config{}
	interfaces.App.Config.SetDefaults()
	interfaces.App.Subs = &submgr.SubscriptionManager{}
	interfaces.App.Logger = logger.NewMockClient()
	interfaces.App.Subs.Init(sub_limit, incexc_limit, buffer, ageout, ageout_check)
}

func managerClose() {
	interfaces.App.Subs.Close()
}

func doRequest(t *testing.T, method string, uri string, body_in string) (code int, body string, contenttype string) {
	req, err := http.NewRequest(method, uri, bytes.NewBuffer([]byte(body_in)))
	if err != nil {
		t.Fatalf("Error constructing request: %s", err.Error())
	}
	rr := httptest.NewRecorder()
	router := echo.New()
	router.POST("/api/v3/subscription", ProcessSubscriptionRequest)
	router.GET("/api/v3/subscription/id/:subscriptionid", ProcessSubscriptionRequest)
	router.PUT("/api/v3/subscription/id/:subscriptionid", ProcessSubscriptionRequest)
	router.PATCH("/api/v3/subscription/id/:subscriptionid", ProcessSubscriptionRequest)
	router.DELETE("/api/v3/subscription/id/:subscriptionid", ProcessSubscriptionRequest)
	router.ServeHTTP(rr, req)
	code = rr.Code
	body = rr.Body.String()
	contenttypelist, ok := rr.Header()["Content-Type"]
	if ok && len(contenttypelist) > 0 {
		contenttype = contenttypelist[0]
	} else {
		contenttype = ""
	}
	return
}

func checkRequest(t *testing.T, method string, uri string, body_in string, exp_code int, exp_ct string) (body string) {
	code, body, contenttype := doRequest(t, method, uri, body_in)
	if exp_code != 0 && exp_code != code {
		t.Fatalf("Bad status from %s %s, got %d, expected %d", method, uri, code, exp_code)
	}
	if exp_ct != "" && exp_ct != contenttype {
		t.Fatalf("Bad Content-Type from %s %s, got %s, expected %s", method, uri, contenttype, exp_ct)
	}
	if contenttype == "application/json" {
		var resp commonDTO.BaseResponse
		err := json.Unmarshal([]byte(body), &resp)
		if err != nil {
			t.Fatalf("Could not parse %s %s response as BaseResponse: %s", method, uri, err.Error())
		}
		if ( resp.ApiVersion != "v3" && resp.ApiVersion != "" ) || ( resp.StatusCode != code && resp.StatusCode != 0 ) {
			t.Fatalf("Problem in BaseResponse to %s %s request, apiVersion %s, statusCode %d", method, uri, resp.ApiVersion, resp.StatusCode)
		}
	}
	return
}

func checkCreateRequest(t *testing.T, exp_code int) (subid string) {
	exp_ct := "application/json"
	if exp_code != http.StatusCreated {
		exp_ct = ""
	}
	body := checkRequest(t, http.MethodPost, uri_base, "", exp_code, exp_ct)
	subid = ""
	var resp subCreateResponse
	if exp_code == http.StatusCreated {
		err := json.Unmarshal([]byte(body), &resp)
		if err != nil {
			t.Fatalf("Could not parse response [%s] as JSON of POST /subscription, error %s", body, err.Error())
		}
		subid = resp.SubscriptionId
		if subid == "" {
			t.Fatal("Empty subscription ID returned from successful POST")
		}
	}
	return
}

func checkGetRequest(t *testing.T, subid string, exp_code int) (resp subInfoResponse) {
	exp_ct := ""
	resp = subInfoResponse{}
	if exp_code == http.StatusOK {
		exp_ct = "application/json"
	}
	body := checkRequest(t, http.MethodGet, uri_base+"/id/"+subid, "", exp_code, exp_ct)
	if exp_code == http.StatusOK {
		err := json.Unmarshal([]byte(body), &resp)
		if err != nil {
			t.Fatalf("Could not parse response [%s] as JSON of GET /subscription/id/%s, error %s", body, subid, err.Error())
		}
	}
	return
}

func TestCreateDelete(t *testing.T) {
	managerInit()
	subid := checkCreateRequest(t, http.StatusCreated)
	_ = checkGetRequest(t, subid+"badsuffix", http.StatusNotFound)
	contents := checkGetRequest(t, subid, http.StatusOK)
	if len(contents.Include) != 0 || len(contents.Exclude) != 0 {
		t.Fatal("Unexpected include/exclude present in new subscription")
	}
	_ = checkRequest(t, http.MethodDelete, uri_base+"/id/"+subid, "", http.StatusOK, "application/json")
	_ = checkGetRequest(t, subid, http.StatusNotFound)
	managerClose()
}

func TestNotAllowed(t *testing.T) {
	disallow_top := [...]string{http.MethodGet, http.MethodDelete, http.MethodPut, http.MethodPatch}
	disallow_subid := [...]string{http.MethodPost}
	managerInit()
	subid := checkCreateRequest(t, http.StatusCreated)
	_ = checkRequest(t, http.MethodPut, uri_base+"/id/"+subid, "{\"apiVersion\":\"v3\", \"requestId\":\"284115e7-d047-4553-8339-97ffa6b1934b\", \"include\":[\"edgex/events/device\"]}", http.StatusOK, "application/json")

	contents := checkGetRequest(t, subid, http.StatusOK)
	if len(contents.Exclude) != 0 || len(contents.Include) != 1 {
		t.Fatalf("Subscription had %d/%d includes/excludes, expected 1/0", len(contents.Include), len(contents.Exclude))
	}
	if contents.Include[0] != "edgex/events/device/" {
		t.Fatalf("Include list %v wrong, expected include edgex/events/device/ only", contents.Include)
	}
	// Now that we're set up, try all the disallowed methods
	for _, m := range disallow_top {
		_ = checkRequest(t, m, uri_base, "", http.StatusMethodNotAllowed, "")
	}
	for _, m := range disallow_subid {
		_ = checkRequest(t, m, uri_base+"/id/"+subid, "", http.StatusMethodNotAllowed, "")
	}
	managerClose()
}

func TestTheLimits(t *testing.T) {
	managerInit()
	subid := checkCreateRequest(t, http.StatusCreated)
	for i := 1; i < sub_limit; i++ {
		_ = checkCreateRequest(t, http.StatusCreated)
	}
	_ = checkCreateRequest(t, http.StatusServiceUnavailable)
	var topicNum int64 = 0
	// Constructing JSON text here because we want to test that it's getting unmarshaled correctly,
	// don't want to create it by marshalling
	req := "{\"apiVersion\": \"v3\", \"include\":["
	for i := 0; i < incexc_limit; i++ {
		req += "\"a/b/c/" + strconv.FormatInt(topicNum, 10) + "\","
		topicNum++
	}
	req += "\"a/b/c/" + strconv.FormatInt(topicNum, 10) + "\"]}"
	_ = checkRequest(t, http.MethodPut, uri_base+"/id/"+subid, req, http.StatusServiceUnavailable, "application/json")
	exc_req := strings.Replace(req, "include", "exclude", 1)
	// This resets the subscription back to 0
	_ = checkRequest(t, http.MethodPatch, uri_base+"/id/"+subid, exc_req, http.StatusOK, "application/json")
	// Adding the excludes again should hit the limit
	_ = checkRequest(t, http.MethodPatch, uri_base+"/id/"+subid, exc_req, http.StatusServiceUnavailable, "application/json")
	// Unparseable
	_ = checkRequest(t, http.MethodPut, uri_base+"/id/"+subid, "this is not json", http.StatusBadRequest, "application/json")
	managerClose()
}

func TestBadUri(t *testing.T) {
	managerInit()
	_ = checkRequest(t, http.MethodGet, "/some/uri", "", http.StatusNotFound, "")
	_ = checkRequest(t, http.MethodGet, "/api/v3/subscriptionmanager", "", http.StatusNotFound, "")
	managerClose()
}

func TestReplacement(t *testing.T) {
	managerInit()
	subid := checkCreateRequest(t, http.StatusCreated)
	req := "{\"apiVersion\":\"v3\", \"include\":[\"edgex/events/device/ProfileA\", \"edgex/events/device/ProfileB\"], \"exclude\":[\"edgex/events/device/ProfileA/DeviceC\"]}"
	_ = checkRequest(t, http.MethodPut, uri_base+"/id/"+subid, req, http.StatusOK, "application/json")
	contents := checkGetRequest(t, subid, http.StatusOK)
	if len(contents.Include) != 2 || len(contents.Exclude) != 1 {
		t.Fatalf("Wrong number of includes/excludes %d/%d, expected 2/1", len(contents.Include), len(contents.Exclude))
	}
	if contents.Exclude[0] != "edgex/events/device/ProfileA/DeviceC/" {
		t.Fatalf("Wrong exclude contents %s, expected edgex/events/device/ProfileA/DeviceC/", contents.Exclude[0])
	}
	if contents.Include[0] == "edgex/events/device/ProfileA/" {
		if contents.Include[1] != "edgex/events/device/ProfileB/" {
			t.Fatalf("Wrong include list: %v", contents.Include)
		}
	} else if contents.Include[0] == "edgex/events/device/ProfileB/" {
		if contents.Include[1] != "edgex/events/device/ProfileA/" {
			t.Fatalf("Wrong include list: %v", contents.Include)
		}
	} else {
		t.Fatalf("Wrong include list: %v", contents.Include)
	}
	req = "{\"apiVersion\":\"v3\", \"include\":[\"edgex/events/device/ProfileC/\"]}"
	_ = checkRequest(t, http.MethodPatch, uri_base+"/id/"+subid, req, http.StatusOK, "application/json")
	contents = checkGetRequest(t, subid, http.StatusOK)
	if len(contents.Include) != 3 || len(contents.Exclude) != 1 {
		t.Fatalf("Wrong number of includes/excludes %d/%d, expected 3/1", len(contents.Include), len(contents.Exclude))
	}
	_ = checkRequest(t, http.MethodPut, uri_base+"/id/"+subid, req, http.StatusOK, "application/json")
	contents = checkGetRequest(t, subid, http.StatusOK)
	if len(contents.Include) != 1 || len(contents.Exclude) != 0 {
		t.Fatalf("Wrong number of includes/excludes %d/%d, expected 1/0", len(contents.Include), len(contents.Exclude))
	}
	if contents.Include[0] != "edgex/events/device/ProfileC/" {
		t.Fatalf("Wrong include entry %s, expected edgex/events/device/ProfileC/", contents.Include[0])
	}
	managerClose()
} 
