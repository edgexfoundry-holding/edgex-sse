//
// Copyright (C) 2025 Eaton
//
// SPDX-License-Identifier: Apache-2.0
//

// Package to handle the custom section of our config file.
// SDK automatically handles things in the other sections
// (e.g. MQTT subscription, log level)

package configuration

import (
	"errors"
	"net"
	"time"
)

// Structure of our config file section
type SseConfig struct {
	SubscriptionLimit                   uint32
	PrefixesLimit                       uint
	EventBuffer                         uint
	EventsAddr                          string
	EventsPort                          uint
	SubscriptionIdleExpiration          string
	SubscriptionExpirationCheckInterval string
}

// Must be wrapped in a struct with element named the same as the section name
// LoggingClient added so we can be passed in the log parameters
type Config struct {
	SSE SseConfig
}

func (c *Config) SetDefaults() {
	c.SSE.SubscriptionLimit = 50
	c.SSE.PrefixesLimit = 100
	c.SSE.EventBuffer = 100
	c.SSE.EventsAddr = "127.0.0.1"
	c.SSE.EventsPort = 59748
	c.SSE.SubscriptionIdleExpiration = "1m"
	c.SSE.SubscriptionExpirationCheckInterval = "5s"
}

func (c *Config) UpdateFromRaw(rawConfig interface{}) bool {
	config, ok := rawConfig.(*Config)
	if !ok {
		return false
	}
	*c = *config
	return true
}

func (c *Config) Validate() error {
	if c.SSE.EventBuffer < 10 {
		return errors.New("EventBuffer must be at least 10 events")
	}
	if c.SSE.SubscriptionLimit == 0 || c.SSE.PrefixesLimit == 0 {
		return errors.New("limits must be greater than zero")
	}
	if c.SSE.EventsPort < 1024 || c.SSE.EventsPort > 65535 {
		return errors.New("EventsPort must be a valid non-reserved TCP port number, 1024-65535")
	}
	ip := net.ParseIP(c.SSE.EventsAddr)
	if ip == nil {
		_, err := net.LookupHost(c.SSE.EventsAddr)
		if err != nil {
			return errors.New("EventsAddr must be a valid IP address or hostname")
		}
	}
	d, err := time.ParseDuration(c.SSE.SubscriptionIdleExpiration)
	if err != nil {
		return errors.New("SubscriptionIdleExpiration must be in the form of a duration, e.g. '30s'")
	}
	if d.Seconds() < 5 {
		return errors.New("SubscriptionIdleExpiration must be at least 5 seconds")
	}
	di, err := time.ParseDuration(c.SSE.SubscriptionExpirationCheckInterval)
	if err != nil {
		return errors.New("SubscriptionExpirationCheckInterval must be in the form of a duration, e.g. '30s'")
	}
	if di.Seconds() <= 0 {
		return errors.New("SubscriptionExpirationCheckInterval must be longer than zero")
	}
	if di.Seconds() * 2 > d.Seconds() {
		return errors.New("SubscriptionIdleExpiration must be at least twice SubscriptionExpirationCheckInterval")
	}
	return nil
}
