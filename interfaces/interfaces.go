//
// Copyright (C) 2025 Eaton
//
// SPDX-License-Identifier: Apache-2.0
//

// Structures go here that need to be referenced by multiple files.
// This way we don't end up with too many cross-dependencies.

package interfaces

import (
	"github.com/edgexfoundry-holding/edgex-sse/configuration"
	"github.com/edgexfoundry-holding/edgex-sse/submgr"
	appint "github.com/edgexfoundry/app-functions-sdk-go/v4/pkg/interfaces"
	"github.com/edgexfoundry/go-mod-core-contracts/v4/clients/logger"
)

// Object to hold our global data and main methods.
type MyApp struct {
	// App-service object from the SDK
	Service appint.ApplicationService
	// Our custom configuration file section
	Config *configuration.Config
	// SDK will configure this logging client from config file/Consul
	Logger logger.LoggingClient
	// Subscription manager
	Subs *submgr.SubscriptionManager
}

// Global instance of this structure
var App MyApp
