//
// Copyright (C) 2025 Eaton
//
// SPDX-License-Identifier: Apache-2.0
//

// httptest.Recorder uses a non-concurrency-safe bytes.Buffer, don't create unnecessary failures
// +build !race
//go:build !race

package web

import (
	"context"
	"github.com/edgexfoundry-holding/edgex-sse/interfaces"
	"github.com/edgexfoundry-holding/edgex-sse/submgr"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

const url_prefix = "/api/v3/events/"

// Create object to handle managing a connection
type checkEventReq struct {
	rr      *httptest.ResponseRecorder
	req     *http.Request
	rc      chan string
	ec      chan error
	reqdone chan bool
	cancel  context.CancelFunc
}

// Function to run ProcessEventRequest, notifying a channel when it is done
// Call this as a goroutine
func (c *checkEventReq) processReq(w http.ResponseWriter, r *http.Request) {
	ProcessEventsRequest(w, r)
	c.reqdone <- true
}

func (c *checkEventReq) beginReq(subid string, exp_status int) {
	c.rc = make(chan string, 64)
	c.ec = make(chan error, 64)
	c.reqdone = make(chan bool)
	defer close(c.rc)
	defer close(c.ec)
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url_prefix+subid, nil)
	if err != nil {
		c.ec <- err
		return
	}
	c.req = req
	c.rr = httptest.NewRecorder()
	go c.processReq(c.rr, c.req)
	reqDone := false
	for !reqDone {
		select {
		case <-c.reqdone:
			reqDone = true
		default:
			for c.rr.Body.Len() != 0 {
				s, err := c.rr.Body.ReadString('\n')
				if err == nil {
					c.rc <- s
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	// Handler has finished when we get here
	if exp_status != c.rr.Code {
		c.ec <- fmt.Errorf("Wrong status code %d in response, expected %d", c.rr.Code, exp_status)
		return
	}
	if exp_status != http.StatusOK {
		return
	}
	val, ok := c.rr.Header()["Content-Type"]
	if !ok || len(val) < 1 {
		c.ec <- errors.New("Missing Content-Type header")
		return
	}
	if val[0] != "text/event-stream" {
		c.ec <- fmt.Errorf("Wrong Content-Type header: %s", val[0])
		return
	}
	val, ok = c.rr.Header()["Cache-Control"]
	if !ok || len(val) < 1 {
		c.ec <- errors.New("Missing Cache-Control header")
		return
	}
	if val[0] != "no-cache" {
		c.ec <- fmt.Errorf("Wrong Cache-Control header: %s", val[0])
		return
	}
	val, ok = c.rr.Header()["Connection"]
	if !ok || len(val) < 1 {
		c.ec <- errors.New("Missing Connection header")
		return
	}
	if val[0] != "keep-alive" {
		c.ec <- fmt.Errorf("Wrong Connection header: %s", val[0])
		return
	}
	val, ok = c.rr.Header()["Transfer-Encoding"]
	if !ok || len(val) < 1 {
		c.ec <- errors.New("Missing Transfer-Encoding header")
		return
	}
	if val[0] != "chunked" {
		c.ec <- fmt.Errorf("Wrong Transfer-Encoding header: %s", val[0])
		return
	}
	val, ok = c.rr.Header()["Access-Control-Allow-Origin"]
	if !ok || len(val) < 1 {
		c.ec <- errors.New("Missing Access-Control-Allow-Origin header")
		return
	}
	if val[0] != "*" {
		c.ec <- fmt.Errorf("Wrong Access-Control-Allow-Origin header: %s", val[0])
		return
	}
	// Did it return the proper events? Another function has to read c.rc to check that
}

func (c *checkEventReq) getNextEvent(t *testing.T) (event_type string, event interface{}) {
	event_done := false
	data_started := false
	var event_buf string
	event_type = ""
	for !event_done {
		select {
		case thisline, ok := <-c.rc:
			if !ok {
				if !event_done {
					t.Fatal("Output stopped mid-event")
				}
				return
			}
			if data_started {
				if thisline == "" || thisline == "\n" {
					event_done = true
					if !c.rr.Flushed {
						t.Fatal("Output did not get flushed by handler")
					}
				} else {
					event_buf = event_buf + thisline
				}
			} else {
				if strings.HasPrefix(thisline, "data:") {
					data_started = true
					event_buf = strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(thisline, "data:")), "\n")
				} else if strings.HasPrefix(thisline, "event:") {
					event_type = strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(thisline, "event:")), "\n")
				} else {
					t.Fatalf("Unexpected event-stream text: %s", thisline)
				}
			}
		case err := <-c.ec:
			t.Fatalf("Error processing request: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatalf("Timeout getting event")
		}
	}
	err := json.Unmarshal([]byte(event_buf), &event)
	if err != nil {
		t.Fatalf("Received event did not parse as JSON, text is: %s", event_buf)
	}
	return
}

func TestBadSubId(t *testing.T) {
	managerInit()
	c := checkEventReq{}
	// Not running in background because we expect failure
	c.beginReq("inexist", http.StatusNotFound)
	select {
	case err, ok := <-c.ec:
		if ok {
			t.Fatalf("Request error: %v", err)
		}
	default:
		return
	}
}

func TestOneEvent(t *testing.T) {
	managerInit()
	c := checkEventReq{}
	if g_subscriptions == nil {
		g_subscriptions = make(map[string]*submgr.SubscriptionInfo)
	}
	subid, err := interfaces.App.Subs.NewSubscription()
	if err != nil || subid == "" {
		t.Fatal("Could not add a subscription")
	}
	subinfo := interfaces.App.Subs.Subscription(subid)
	if subinfo == nil {
		t.Fatal("Subscription not found")
	}
	g_subscriptions[subid] = subinfo
	go c.beginReq(subid, http.StatusOK)
	time.Sleep(500 * time.Millisecond)
	err = interfaces.App.Subs.Include(subinfo, "a/b")
	if err != nil {
		t.Fatalf("Could not add include: %v", err)
	}
	chans := interfaces.App.Subs.SubscribedChannels("a/b")
	if len(chans) != 1 {
		t.Fatalf("Expected 1 subscribed channel, got %d", len(chans))
	}
	msg := submgr.ChannelMessage{}
	msg.EventType = ""
	msg.Payload = "{\"a\":\"b\", \"c\": {\"d\": 3 }}"
	chans[0] <- msg
	event_type, event := c.getNextEvent(t)
	if event_type != "" {
		t.Fatalf("Unexpected event type %s", event_type)
	}
	var exp_event interface{}
	err = json.Unmarshal([]byte(msg.Payload), &exp_event)
	if err != nil || !reflect.DeepEqual(event, exp_event) {
		t.Fatalf("Event returned is not what we expect, got: %v", event)
	}
}

func TestDisconnect(t *testing.T) {
	managerInit()
	c := checkEventReq{}
	if g_subscriptions == nil {
		g_subscriptions = make(map[string]*submgr.SubscriptionInfo)
	}
	subid, err := interfaces.App.Subs.NewSubscription()
	if err != nil || subid == "" {
		t.Fatal("Could not add a subscription")
	}
	subinfo := interfaces.App.Subs.Subscription(subid)
	if subinfo == nil {
		t.Fatal("Subscription not found")
	}
	g_subscriptions[subid] = subinfo
	go c.beginReq(subid, http.StatusOK)
	time.Sleep(500 * time.Millisecond)
	err = interfaces.App.Subs.Include(subinfo, "a/b")
	if err != nil {
		t.Fatalf("Could not add include: %v", err)
	}
	chans := interfaces.App.Subs.SubscribedChannels("a/b")
	if len(chans) != 1 {
		t.Fatalf("Expected 1 subscribed channel, got %d", len(chans))
	}
	c.cancel()
	time.Sleep(500 * time.Millisecond)
	chans = interfaces.App.Subs.SubscribedChannels("a/b")
	if len(chans) != 0 {
		t.Fatalf("Expected 0 subscribed channels, got %d", len(chans))
	}
}

// Test closing the channel.
func TestDeleteSubscription(t *testing.T) {
	managerInit()
	c := checkEventReq{}
	if g_subscriptions == nil {
		g_subscriptions = make(map[string]*submgr.SubscriptionInfo)
	}
	subid, err := interfaces.App.Subs.NewSubscription()
	if err != nil || subid == "" {
		t.Fatal("Could not add a subscription")
	}
	subinfo := interfaces.App.Subs.Subscription(subid)
	if subinfo == nil {
		t.Fatal("Subscription not found")
	}
	g_subscriptions[subid] = subinfo
	go c.beginReq(subid, http.StatusOK)
	time.Sleep(500 * time.Millisecond)
	err = interfaces.App.Subs.Include(subinfo, "a/b")
	if err != nil {
		t.Fatalf("Could not add include: %v", err)
	}
	chans := interfaces.App.Subs.SubscribedChannels("a/b")
	if len(chans) != 1 {
		t.Fatalf("Expected 1 subscribed channel, got %d", len(chans))
	}
	interfaces.App.Subs.DeleteSubscription(subid)
	time.Sleep(500 * time.Millisecond)
	chans = interfaces.App.Subs.SubscribedChannels("a/b")
	if len(chans) != 0 {
		t.Fatalf("Expected 0 subscribed channels, got %d", len(chans))
	}
}

func TestBadRequests(t *testing.T) {
	managerInit()
	rr := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPost, url_prefix+"subid", nil)
	if err != nil {
		t.Fatalf("Could not construct request: %v", err)
	}
	ProcessEventsRequest(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("Got wrong status %d instead of Method Not Allowed", rr.Code)
	}
	req, err = http.NewRequest(http.MethodGet, "/inexist", nil)
	if err != nil {
		t.Fatalf("Could not construct request: %v", err)
	}
	rr = httptest.NewRecorder()
	ProcessEventsRequest(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("Got wrong status %d instead of 404", rr.Code)
	}
	req, err = http.NewRequest(http.MethodGet, url_prefix, nil)
	if err != nil {
		t.Fatalf("Could not construct request: %v", err)
	}
	rr = httptest.NewRecorder()
	ProcessEventsRequest(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("Got wrong status %d instead of 404", rr.Code)
	}
	req, err = http.NewRequest(http.MethodGet, url_prefix+"a/b/c", nil)
	if err != nil {
		t.Fatalf("Could not construct request: %v", err)
	}
	rr = httptest.NewRecorder()
	ProcessEventsRequest(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("Got wrong status %d instead of 404", rr.Code)
	}
}

// Last bit of coverage: mix EdgeX and non-EdgeX events
func TestMixedEvents(t *testing.T) {
	managerInit()
	c := checkEventReq{}
	if g_subscriptions == nil {
		g_subscriptions = make(map[string]*submgr.SubscriptionInfo)
	}
	subid, err := interfaces.App.Subs.NewSubscription()
	if err != nil || subid == "" {
		t.Fatal("Could not add a subscription")
	}
	subinfo := interfaces.App.Subs.Subscription(subid)
	if subinfo == nil {
		t.Fatal("Subscription not found")
	}
	g_subscriptions[subid] = subinfo
	go c.beginReq(subid, http.StatusOK)
	time.Sleep(500 * time.Millisecond)
	err = interfaces.App.Subs.Include(subinfo, "edgex/events/device/")
	if err != nil {
		t.Fatalf("Could not add edgex/events/device include: %v", err)
	}
	err = interfaces.App.Subs.Include(subinfo, "ble/events/alarms")
	if err != nil {
		t.Fatalf("Could not add ble/events/alarms include: %v", err)
	}
	chans := interfaces.App.Subs.SubscribedChannels("edgex/events/device/Bacon-Cape/Virtual-Bacon-Cape-04/mPercentLoad")
	if len(chans) != 1 {
		t.Fatalf("Expected 1 subscribed channel, got %d", len(chans))
	}
	msg := submgr.ChannelMessage{}
	msg.EventType = "edgex"
	// actual event copypasta
	msg.Payload = "{\"apiVersion\":\"v3\",\"requestId\":\"94512292-e68b-458d-9dff-bb7efa7dfe94\",\"event\":{\"apiVersion\":\"v3\",\"id\":\"7d3d60c0-5279-436b-b99d-6ab1de0eb600\",\"deviceName\":\"Virtual-Bacon-Cape-04\",\"profileName\":\"Bacon-Cape\",\"sourceName\":\"mPercentLoad\",\"origin\":1661535695202033126,\"readings\":[{\"id\":\"b4f7b655-5dac-4f34-8dc7-caa2f8c1a34d\",\"origin\":1661535695202033126,\"deviceName\":\"Virtual-Bacon-Cape-04\",\"resourceName\":\"mPercentLoad\",\"profileName\":\"Bacon-Cape\",\"valueType\":\"Uint32\",\"binaryValue\":null,\"mediaType\":\"\",\"value\":\"74\"}]}}"
	chans[0] <- msg
	event_type, event := c.getNextEvent(t)
	if event_type != "edgex" {
		t.Fatalf("Unexpected event type %s", event_type)
	}
	var exp_event interface{}
	err = json.Unmarshal([]byte(msg.Payload), &exp_event)
	if err != nil || !reflect.DeepEqual(event, exp_event) {
		t.Fatalf("Event returned is not what we expect, got: %v", event)
	}
	chans = interfaces.App.Subs.SubscribedChannels("ble/events/alarms")
	if len(chans) != 1 {
		t.Fatalf("Expected 1 subscribed channel, got %d", len(chans))
	}
	msg = submgr.ChannelMessage{}
	msg.EventType = ""
	msg.Payload = "{\"deviceId\":1, \"state\": \"CLOSED\"}"
	chans[0] <- msg
	event_type, event = c.getNextEvent(t)
	if event_type != "" {
		t.Fatalf("Unexpected event type %s", event_type)
	}
	err = json.Unmarshal([]byte(msg.Payload), &exp_event)
	if err != nil || !reflect.DeepEqual(event, exp_event) {
		t.Fatalf("Event returned is not what we expect, got: %v", event)
	}
}
