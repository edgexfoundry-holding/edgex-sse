//
// Copyright (C) 2025 Eaton
//
// SPDX-License-Identifier: Apache-2.0
//

// Package for our pipeline functions
package functions

import (
	"github.com/edgexfoundry-holding/edgex-sse/submgr"
	"encoding/json"

	"github.com/edgexfoundry/app-functions-sdk-go/v4/pkg/interfaces"
	"github.com/edgexfoundry/go-mod-core-contracts/v4/clients/logger"
	"github.com/edgexfoundry/go-mod-core-contracts/v4/dtos"
	"github.com/edgexfoundry/go-mod-core-contracts/v4/common"
)

// Object to hold the functions and the state they need
type Processor struct {
	lc            logger.LoggingClient
	subscriptions *submgr.SubscriptionManager
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
func (p *Processor) Publish(ctx interfaces.AppFunctionContext, incoming_data interface{}) (bool, interface{}) {
	var dstEvent dtos.Event
	var msg submgr.ChannelMessage

	topic, ok := ctx.GetValue(interfaces.RECEIVEDTOPIC)
	if !ok {
		p.lc.Error("Message received with no topic, ignoring")
		return true, incoming_data
	}
	chanlist := p.subscriptions.SubscribedChannels(topic)
	p.lc.Tracef("Message received on topic %s, %d active subscriptions", topic, len(chanlist))
	// Short-circuit since it's rather likely nobody is subscribed to this, don't bother casting,
	// marshalling, etc.
	if len(chanlist) == 0 {
		return true, incoming_data
	}
	
	data, ok := incoming_data.(map[string]any)
	if !ok {
		p.lc.Error("Received function call that was not an unmarshaled message, something is wrong")
		return true, incoming_data
	}

	event, ok := data["event"]
	// If this has an "event" member then it is likely an AddEventRequest, we want to return the Event
	// contained therein.
	if (ok) {
		intermediate, err := json.Marshal(event)
		if err == nil {
			err := json.Unmarshal(intermediate, &dstEvent)
			if err == nil {
				err := common.Validate(dstEvent)
				if err == nil {
					msg.Payload = string(intermediate)
					msg.EventType = "edgex"
				}
			}
		}
	}

	if msg.EventType == "" {
		// Still unsure. See if it is an event in itself.
		_, ok := data["readings"]
		if ok {
			event_bytes, err := json.Marshal(data)
			if err == nil {
				err := json.Unmarshal(event_bytes, &dstEvent)
				if err == nil {
					err := common.Validate(dstEvent)
					if err == nil {
						msg.Payload = string(event_bytes)
						msg.EventType = "edgex"
					}
				}
			}
		}
	}

	if msg.EventType == "" {
		// Not an EdgeX event, just put together the JSON string
		event_bytes, err := json.Marshal(data)
		if err != nil {
			return true, incoming_data
		}
		msg.Payload = string(event_bytes)
	}

	for _, ch := range chanlist {
		ch <- msg
	}

	return true, incoming_data
}
