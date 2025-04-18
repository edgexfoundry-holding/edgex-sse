//
// Copyright (C) 2025 Eaton
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"github.com/edgexfoundry-holding/edgex-sse/configuration"
	"github.com/edgexfoundry-holding/edgex-sse/interfaces"
	"github.com/edgexfoundry-holding/edgex-sse/submgr"
	"github.com/edgexfoundry-holding/edgex-sse/web"
	"github.com/edgexfoundry-holding/edgex-sse/functions"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/edgexfoundry/app-functions-sdk-go/v4/pkg"
	appint "github.com/edgexfoundry/app-functions-sdk-go/v4/pkg/interfaces"
)

const (
	// Identifies us in the Registry
	serviceKey = "edgex-sse"
)

// Like the SDK's app service template, we move main() functionality into its own
// function for unit testing. If we add unit tests someday, that might help.
func main() {
	code := CreateAndRunAppService(serviceKey, pkg.NewAppServiceWithTargetType)
	os.Exit(code)
}

// CreateAndRunAppService wraps what would normally be in main() so that it can be unit tested
func CreateAndRunAppService(serviceKey string, newServiceFactory func(string, any) (appint.ApplicationService, bool)) int {
	var ok bool
	// Asking the messaging client for an "any" will give us the generic un-marshaling of map[string]any
	var desiredBuffer any
	interfaces.App.Service, ok = newServiceFactory(serviceKey, &desiredBuffer)
	if !ok {
		return -1
	}

	interfaces.App.Logger = interfaces.App.Service.LoggingClient()
	interfaces.App.Config = &configuration.Config{}
	interfaces.App.Config.SetDefaults()
	interfaces.App.Subs = &submgr.SubscriptionManager{}

	// Aliases for shorter lines below
	cfg := interfaces.App.Config
	lc := interfaces.App.Logger
	svc := interfaces.App.Service
	subs := interfaces.App.Subs

	// Load our custom config object from the "SSE" config-file/Consul section
	// We are not yet set up to listen for run-time config changes
	if err := svc.LoadCustomConfig(cfg, "SSE"); err != nil {
		lc.Errorf("failed loading SSE configuration section: %s", err.Error())
		return -1
	}
	if err := cfg.Validate(); err != nil {
		lc.Errorf("SSE configuration section failed validation: %s", err.Error())
		return -1
	}

	ageout, err := time.ParseDuration(cfg.SSE.SubscriptionIdleExpiration)
	ageoutInterval, err2 := time.ParseDuration(cfg.SSE.SubscriptionExpirationCheckInterval)
	if (err != nil) || (err2 != nil) {  // probably cannot happen, checked in Validate()
		lc.Error("Could not parse SubscriptionIdleExpiration and/or SubscriptionExpirationCheckInterval")
		return -1
	}
	lc.Tracef("Starting subscription manager, limits: %d subs, %d entries/sub, event buffer %d, ageout %v check every %v", cfg.SSE.SubscriptionLimit, cfg.SSE.PrefixesLimit, cfg.SSE.EventBuffer, ageout, ageoutInterval)
	subs.Init(cfg.SSE.SubscriptionLimit, cfg.SSE.PrefixesLimit, cfg.SSE.EventBuffer, ageout, ageoutInterval)

	// Create function pipeline - all events we see are ran through these
	// functions, in order.
	processor := functions.NewProcessor(lc, subs)
	err = svc.SetDefaultFunctionsPipeline(processor.Publish)
	if err != nil {
		lc.Errorf("SetDefaultFunctionsPipeline returned error: %s", err.Error())
		return -1
	}

	// Register our custom REST endpoints
	err = svc.AddCustomRoute("/api/v3/subscription", appint.Authenticated, web.ProcessSubscriptionRequest, http.MethodPost)
	if err != nil {
		lc.Errorf("Could not register /subscription endpoint: %s", err.Error())
		return -1
	}
	err = svc.AddCustomRoute("/api/v3/subscription/id/:subscriptionid", appint.Authenticated, web.ProcessSubscriptionRequest, http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch)
	if err != nil {
		lc.Errorf("Could not register /subscription/id/{subscriptionid} endpoint: %s", err.Error())
		return -1
	}

	// EdgeX app SDK uses HTTP server with TimeoutHandler so requests can time out.
	// This is fine for most things, but does not play well with SSE.
	// net.http.Flusher() is not implemented for that handler, it doesn't make sense.
	// Our solution: serve /events on another port using the regular handler
	// so the SSE GETs don't time out.
	eventmux := http.NewServeMux()
	eventmux.HandleFunc("/api/v3/events/", web.ProcessEventsRequest)
	listenaddr := cfg.SSE.EventsAddr + ":" + strconv.FormatUint(uint64(cfg.SSE.EventsPort), 10)
	// Run in the background
	go http.ListenAndServe(listenaddr, eventmux)
	lc.Infof("Listening for EventSource GETs at %s", listenaddr)

	// This doesn't return until program catches a signal to exit
	if err := svc.Run(); err != nil {
		lc.Errorf("MakeItRun returned error: %s", err.Error())
		return -1
	}

	subs.Close()
	lc.Info("Service exiting")

	return 0
}
