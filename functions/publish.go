//
// Copyright (C) 2025 Eaton
//
// SPDX-License-Identifier: Apache-2.0
//

// Package for our pipeline functions
package functions

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/edgexfoundry-holding/edgex-sse/submgr"

	"github.com/edgexfoundry/app-functions-sdk-go/v4/pkg/interfaces"
	"github.com/edgexfoundry/go-mod-core-contracts/v4/clients/logger"
	"github.com/edgexfoundry/go-mod-core-contracts/v4/common"
	"github.com/edgexfoundry/go-mod-core-contracts/v4/dtos"
	"github.com/edgexfoundry/go-mod-core-contracts/v4/dtos/requests"
)

// Object to hold the functions and the state they need
type Processor struct {
	lc              logger.LoggingClient
	subscriptions   *submgr.SubscriptionManager
	warnedAboutJson bool
}

// Factory function
func NewProcessor(logger logger.LoggingClient, mgr *submgr.SubscriptionManager) Processor {
	p := Processor{}
	p.lc = logger
	p.subscriptions = mgr
	p.warnedAboutJson = false
	return p
}

// Event pipeline function.
func (p *Processor) Publish(ctx interfaces.AppFunctionContext, incoming_data any) (bool, any) {
	var msg submgr.ChannelMessage

	topic, ok := ctx.GetValue(interfaces.RECEIVEDTOPIC)
	if !ok {
		p.lc.Error("Message received with no topic, ignoring")
		return true, incoming_data
	}

	// Data will be a base64-encoded string. TODO: verify this, handle other cases
	str_data, ok := incoming_data.(string)
	if !ok {
		p.lc.Errorf("Message received with Go datatype %T, ignoring", incoming_data)
		return true, incoming_data
	}

	chanlist := p.subscriptions.SubscribedChannels(topic)
	p.lc.Tracef("Message received on topic %s, %d active subscriptions", topic, len(chanlist))
	// Short-circuit since it's rather likely nobody is subscribed to this, don't bother casting,
	// marshalling, etc.
	if len(chanlist) == 0 {
		return true, incoming_data
	}

	if str_data == "" {
		return true, incoming_data
	}

	// Experiment in stripping parsing down: support only JSON, and try to just pass along the
	// string, unprocessed, if possible.

	if ctx.InputContentType() != "application/json" {
		if !p.warnedAboutJson {
			p.lc.Warnf("Messages other than JSON not supported, saw one of Content-Type %s, ignoring", ctx.InputContentType())
			p.warnedAboutJson = true
		}
	}

	data_bytes, err := base64.StdEncoding.DecodeString(str_data)
	if err != nil {
		p.lc.Errorf("Failed to decode base64 string: %s", err.Error())
		return true, incoming_data
	}

	// Determine what kind of event this is.
	// If this does not contain a "readings" JSON then it is neither an Event nor AddEventRequest
	// so we would need no further parsing

	msg.Payload = string(data_bytes)
	msg.EventType = ""
	if strings.Contains(msg.Payload, `"readings"`) {
		var dstAer requests.AddEventRequest
		if err := json.Unmarshal(data_bytes, &dstAer); err == nil {
			if err := common.Validate(dstAer); err == nil {
				msg.EventType = "edgex"
				mpBytes, err := json.Marshal(dstAer.Event)
				if err == nil {
					msg.Payload = string(mpBytes)
				}
			}
		} else {
			var dstEvent dtos.Event
			if err := json.Unmarshal(data_bytes, &dstEvent); err == nil {
				if err := common.Validate(dstEvent); err == nil {
					msg.EventType = "edgex"
				}
			}
		}
	}

	for _, ch := range chanlist {
		ch <- msg
	}

	return true, incoming_data
}
