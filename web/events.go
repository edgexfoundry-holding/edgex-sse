//
// Copyright (C) 2025 Eaton
//
// SPDX-License-Identifier: Apache-2.0
//

package web

import (
	"github.com/edgexfoundry-holding/edgex-sse/interfaces"
	"io"
	"net/http"
	"strings"
)


func ProcessEventsRequest(w http.ResponseWriter, r *http.Request) {
	lc := interfaces.App.Logger
	subs := interfaces.App.Subs

	if r.Method != http.MethodGet {
		http.Error(w, "Only GET is allowed", http.StatusMethodNotAllowed)
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/api/v3/events/") {
		http.Error(w, "Improper request path", http.StatusNotFound)
		return
	}
	subid := strings.TrimPrefix(r.URL.Path, "/api/v3/events/")
	if subid == "" || strings.ContainsRune(subid, '/') {
		http.Error(w, "Subscription ID required", http.StatusNotFound)
		return
	}
	lc.Debugf("Got /events request for subscription %s", subid)
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE unsupported", http.StatusInternalServerError)
		return
	}
	lockmgt.RLock()
	subInfo, ok := g_subscriptions[subid]
	if !ok {
		http.Error(w, "Subscription not found", http.StatusNotFound)
		lockmgt.RUnlock()
		return
	}
	lockmgt.RUnlock()
	
	check1 := subs.IsSubscriptionDeleted(subInfo)
	if check1 {
		w.WriteHeader(http.StatusNotFound)
		return
	}	
	check2 := subs.IsChannelClosed(subInfo)
	if check2 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	rxchan, err := subs.ReceiveChannel(subInfo)
	if err != nil || rxchan == nil {
		http.Error(w, "Subscription not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	flusher.Flush()
	subs.SetActive(subInfo, true)
	defer subs.SetActive(subInfo, false)
	done := false
	for !done {
		select {
		case msg, ok := <-rxchan:
			if !ok {
				// Channel has been closed, exit loop
				done = true
			} else {
				if msg.EventType == "edgex" {
					io.WriteString(w, "event: edgex\n")
				}
				io.WriteString(w, "data: "+msg.Payload+"\n\n")
				flusher.Flush()
			}
		case <-r.Context().Done():
			done = true
		}
	}
	// End loop, we are done processing, the connection will close
}
