//
// Copyright (C) 2025 Eaton
//
// SPDX-License-Identifier: Apache-2.0
//

// Package for managing HTTP requests / responses
package web

import (
	"github.com/edgexfoundry-holding/edgex-sse/interfaces"
	"github.com/edgexfoundry-holding/edgex-sse/submgr"
	"encoding/json"
	"github.com/edgexfoundry/go-mod-core-contracts/v4/common"
	commonDTO "github.com/edgexfoundry/go-mod-core-contracts/v4/dtos/common"
	"github.com/labstack/echo/v4"
	"net/http"
	"strings"
	"sync"
)

var g_subscriptions map[string]*submgr.SubscriptionInfo

var lockmgt   sync.RWMutex

func sendResponse(w http.ResponseWriter, r *http.Request, response interface{}, statusCode int) {
	correlationID := r.Header.Get(common.CorrelationHeader)

	w.Header().Set(common.CorrelationHeader, correlationID)
	w.Header().Set(common.ContentType, common.ContentTypeJSON)

	data, err := json.Marshal(response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(statusCode)
	w.Write(data)
}

func respondBase(w http.ResponseWriter, r *http.Request, requestId string, statusCode int, message string) {
	br := commonDTO.NewBaseResponse(requestId, message, statusCode)
	sendResponse(w, r, br, statusCode)
}

func addSubscription(w http.ResponseWriter, r *http.Request) {
	type postReturn struct {
		commonDTO.BaseResponse `json:",inline"`
		SubscriptionId         string `json:"subscriptionId"`
	}
	lc := interfaces.App.Logger
	subs := interfaces.App.Subs
	subid, err := subs.NewSubscription()
	if err != nil {
		lc.Infof("Subscription creation request error: %s", err.Error())
		respondBase(w, r, "", http.StatusServiceUnavailable, err.Error())
		return
	}
	rv := postReturn{}
	rv.BaseResponse = commonDTO.NewBaseResponse("", "Subscription created", http.StatusCreated)
	rv.SubscriptionId = subid
	lockmgt.Lock()	
	if g_subscriptions == nil {
		g_subscriptions = make(map[string]*submgr.SubscriptionInfo)
	}
	subInfo := subs.Subscription(subid)
	if subInfo == nil {
		w.WriteHeader(http.StatusNotFound)
		lockmgt.Unlock()
		return
	}
	g_subscriptions[subid] = subInfo
	lockmgt.Unlock()	
	sendResponse(w, r, rv, http.StatusCreated)
}

func deleteSubscription(w http.ResponseWriter, r *http.Request, subid string) {
	lc := interfaces.App.Logger
	subs := interfaces.App.Subs
	lc.Debugf("Deleting subscription %s", subid)
	subs.DeleteSubscription(subid)
	respondBase(w, r, "", http.StatusOK, "Subscription deleted")
}

func getSubscription(w http.ResponseWriter, r *http.Request, includes []string, excludes []string) {
	type getReturn struct {
		commonDTO.BaseResponse `json:",inline"`
		Include                []string `json:"include"`
		Exclude                []string `json:"exclude"`
	}
	rv := getReturn{}
	rv.BaseResponse = commonDTO.NewBaseResponse("", "", http.StatusOK)
	rv.Include = includes
	rv.Exclude = excludes
	sendResponse(w, r, rv, http.StatusOK)
}

func putSubscription(w http.ResponseWriter, r *http.Request, subInfo *submgr.SubscriptionInfo, existing_includes []string, existing_excludes []string) {
	// Delete everything, then do the same processing as "patch"
	lc := interfaces.App.Logger
	subs := interfaces.App.Subs
	someError := false
	for _, e := range existing_excludes {
		err := subs.Include(subInfo, e)
		if err != nil {
			lc.Errorf("Error deleting exclude %s from subscription during PUT: %s", e, err.Error())
			someError = true
		}
	}
	for _, i := range existing_includes {
		err := subs.Exclude(subInfo, i)
		if err != nil {
			lc.Errorf("Error deleting include %s from subscription during PUT: %s", i, err.Error())
			someError = true
		}
	}
	if someError {
		respondBase(w, r, "", http.StatusInternalServerError, "Error deleting existing subscription list items")
		return
	}
	patchSubscription(w, r, subInfo)
}

func patchSubscription(w http.ResponseWriter, r *http.Request, subInfo *submgr.SubscriptionInfo) {
	lc := interfaces.App.Logger
	subs := interfaces.App.Subs
	type subreq struct {
		commonDTO.BaseRequest `json:",inline"`
		Include               []string `json:"include"`
		Exclude               []string `json:"exclude"`
	}
	var request subreq
	defer func() {
		_ = r.Body.Close()
	}()
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		respondBase(w, r, "", http.StatusBadRequest, err.Error())
		return
	}
	for _, i := range request.Include {
		err := subs.Include(subInfo, i)
		if err != nil {
			lc.Infof("Error including topic %s for subscription: %s", i, err.Error())
			respondBase(w, r, "", http.StatusServiceUnavailable, err.Error())
			return
		}
	}
	for _, e := range request.Exclude {
		err := subs.Exclude(subInfo, e)
		if err != nil {
			lc.Infof("Error excluding topic %s from subscription: %s", e, err.Error())
			respondBase(w, r, "", http.StatusServiceUnavailable, err.Error())
			return
		}
	}
	respondBase(w, r, "", http.StatusOK, "Subscription updated.")
}

func ProcessSubscriptionRequest(c echo.Context) error {
	lc := interfaces.App.Logger
	subs := interfaces.App.Subs
	w := c.Response()
	r := c.Request()

	lc.Tracef("Processing subscription management %s at %s", r.Method, r.URL.Path)
	// We don't know our path leading up to /subscription, so remove
	// /subscription and everything before it
	idx := strings.Index(r.URL.Path, "/subscription")
	if idx < 0 {
		w.WriteHeader(http.StatusNotFound)
		return nil
	}
	idx = idx + len("/subscription")
	subpath := r.URL.Path[idx:len(r.URL.Path)]
	lc.Tracef("subpath: %s", subpath)
	if subpath == "" || subpath == "/" {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return nil
		}
		addSubscription(w, r)
		return nil
	}
	subid := c.Param("subscriptionid")
	if subid == "" {
		w.WriteHeader(http.StatusNotFound)
		return nil
	}
	lockmgt.RLock()
	subInfo, ok := g_subscriptions[subid]
	if !ok {
		http.Error(w, "Subscription not found", http.StatusNotFound)
		lockmgt.RUnlock()
		return nil
	}
	lockmgt.RUnlock()
	subs.SetProcess(subInfo, true)
	check1 := subs.IsSubscriptionDeleted(subInfo)
	if check1 {
		w.WriteHeader(http.StatusNotFound)
		return nil
	}	
	check2 := subs.IsChannelClosed(subInfo)
	if check2 {
		w.WriteHeader(http.StatusNotFound)
		return nil
	}
	includes, excludes, ok := subs.SubscriptionInfo(subInfo)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return nil
	}
	switch r.Method {
	case http.MethodGet:
		getSubscription(w, r, includes, excludes)
		subs.SetProcess(subInfo, false)
		return nil
	case http.MethodDelete:
		deleteSubscription(w, r, subid)
		return nil
	case http.MethodPut:
		putSubscription(w, r, subInfo, includes, excludes)
		subs.SetProcess(subInfo, false)
		return nil
	case http.MethodPatch:
		patchSubscription(w, r, subInfo)
		subs.SetProcess(subInfo, false)
		return nil
	default:
		respondBase(w, r, "", http.StatusMethodNotAllowed, "Method not allowed")
		subs.SetProcess(subInfo, false)
		return nil
	}
}
