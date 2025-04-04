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
	"github.com/edgexfoundry/go-mod-core-contracts/v4/dtos/requests"
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
	var dstAer requests.AddEventRequest

	var msg submgr.ChannelMessage
	topic, ok := ctx.GetValue(interfaces.RECEIVEDTOPIC)
	if !ok {
		p.lc.Error("Message received with no topic, ignoring")
		return true, incoming_data
	}
	data, ok := incoming_data.([]byte)
	if !ok {
		p.lc.Error("Received function call with something not []byte, something is wrong")
		return true, incoming_data
	}
	chanlist := p.subscriptions.SubscribedChannels(topic)
	p.lc.Tracef("Message received on topic %s, %d active subscriptions", topic, len(chanlist))
	// Short-circuit since it's rather likely nobody is subscribed to this, don't bother casting,
	// marshalling, etc.
	if len(chanlist) == 0 {
		return true, incoming_data
	}
	if ctx.InputContentType() != "application/json" {
		if !p.warnedAboutJson {
			p.lc.Warnf("Messages other than JSON not supported, saw one of Content-Type %s, ignoring", ctx.InputContentType());
			p.warnedAboutJson = true
		}
		return true, incoming_data
	}
	msg.Payload = string(data)
	if msg.Payload == "" {
		return true, incoming_data
	}
	// Go figure out what type of event this is.
	msg.EventType = ""
	// If this is an AddEventRequest structure, extract the "event" member and re-encode
	err := json.Unmarshal(data, &dstAer)
	if (err == nil) {
		err := common.Validate(dstAer)
		if err == nil {
			msg.EventType = "edgex"
			mpBytes, err := json.Marshal(dstAer.Event)
			if (err == nil) {
				msg.Payload = string(mpBytes)
			}
		}
	}
	// Otherwise, pass along the payload as-is, indicating if it's an EdgeX Event or not
	if (msg.EventType == "") {
		err := json.Unmarshal(data, &dstEvent)
		if err == nil {
			err := common.Validate(dstEvent)
			if (err == nil) {
				msg.EventType = "edgex"
			}
		}
	}
	for _, ch := range chanlist {
		ch <- msg
	}
	return true, incoming_data
}
