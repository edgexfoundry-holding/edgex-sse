//
// Copyright (C) 2025 Eaton
//
// SPDX-License-Identifier: Apache-2.0
//

package configuration

import (
	"testing"
)

func TestDefaults(t *testing.T) {
	var dut Config
	dut.SetDefaults()
	if dut.SSE.SubscriptionLimit != 50 {
		t.Fatalf("Wrong default subscription limit: %d", dut.SSE.SubscriptionLimit)
	}
	if dut.SSE.PrefixesLimit != 100 {
		t.Fatalf("Wrong default prefixes limit: %d", dut.SSE.PrefixesLimit)
	}
	if dut.SSE.EventBuffer != 100 {
		t.Fatalf("Wrong default event buffer: %d", dut.SSE.EventBuffer)
	}
	if dut.SSE.EventsPort != 59748 {
		t.Fatalf("Wrong default EventsPort: %d", dut.SSE.EventsPort)
	}
	if dut.SSE.EventsAddr != "127.0.0.1" {
		t.Fatalf("Wrong default EventsAddr: %s", dut.SSE.EventsAddr)
	}
	if dut.SSE.SubscriptionIdleExpiration != "1m" {
		t.Fatalf("Wrong default SubscriptionIdleExpiration: %s", dut.SSE.SubscriptionIdleExpiration)
	}
	if dut.SSE.SubscriptionExpirationCheckInterval != "5s" {
		t.Fatalf("Wrong default SubscriptionExpirationCheckInterval: %s", dut.SSE.SubscriptionExpirationCheckInterval)		
	}
}

type rawercfg struct {
	Foo uint32
	Bar uint
}

func TestUpdate(t *testing.T) {
	var dut Config
	var u1 rawercfg
	u1.Foo = 13
	u1.Bar = 27
	if dut.UpdateFromRaw(&u1) {
		t.Fatal("UpdateFromRaw with invalid config unexpectedly succeeded")
	}
	var u2 Config
	u2.SSE.EventBuffer = 131
	u2.SSE.PrefixesLimit = 222
	u2.SSE.SubscriptionLimit = 4
	u2.SSE.EventsAddr = "localhost"
	u2.SSE.EventsPort = 12345
	u2.SSE.SubscriptionExpirationCheckInterval = "10s"
	u2.SSE.SubscriptionIdleExpiration = "5m"
	if !dut.UpdateFromRaw(&u2) {
		t.Fatal("UpdateFromRaw failed")
	}
	if dut.SSE.EventBuffer != 131 {
		t.Fatalf("EventBuffer wrong value %d", dut.SSE.EventBuffer)
	}
	if dut.SSE.PrefixesLimit != 222 {
		t.Fatalf("PrefixesLimit wrong value %d", dut.SSE.PrefixesLimit)
	}
	if dut.SSE.SubscriptionLimit != 4 {
		t.Fatalf("SubscriptionLimit wrong value %d", dut.SSE.SubscriptionLimit)
	}
	if dut.SSE.EventsAddr != "localhost" {
		t.Fatalf("EventsAddr wrong value %s", dut.SSE.EventsAddr)
	}
	if dut.SSE.EventsPort != 12345 {
		t.Fatalf("EventsPort wrong value %d", dut.SSE.EventsPort)
	}
	if dut.SSE.SubscriptionIdleExpiration != "5m" {
		t.Fatalf("SubscriptionIdleExpiration wrong value %s", dut.SSE.SubscriptionIdleExpiration)
	}
	if dut.SSE.SubscriptionExpirationCheckInterval != "10s" {
		t.Fatalf("SubscriptionExpirationCheckInterval wrong value %s", dut.SSE.SubscriptionExpirationCheckInterval)
	}
}

func TestValidation(t *testing.T) {
	var dut Config
	dut.SetDefaults()
	err := dut.Validate()
	if err != nil {
		t.Fatalf("Validate() of defaults failed: %s", err.Error())
	}
	dut.SSE.EventBuffer = 9
	err = dut.Validate()
	if err == nil {
		t.Fatal("Validate() succeeded with EventBuffer < 10")
	}
	dut.SetDefaults()
	dut.SSE.PrefixesLimit = 0
	err = dut.Validate()
	if err == nil {
		t.Fatal("Validate() succeeded with PrefixesLimit = 0")
	}
	dut.SetDefaults()
	dut.SSE.SubscriptionLimit = 0
	err = dut.Validate()
	if err == nil {
		t.Fatal("Validate() succeeded with SubscriptionLimit = 0")
	}
	dut.SetDefaults()
	// Underscores not valid in DNS names
	dut.SSE.EventsAddr = "not_a_valid_hostname_or_ip"
	err = dut.Validate()
	if err == nil {
		t.Fatal("Validate() succeeded with invalid EventsAddr")
	}
	dut.SetDefaults()
	dut.SSE.EventsPort = 0
	err = dut.Validate()
	if err == nil {
		t.Fatal("Validate() succeeded with EventsPort = 0")
	}
	dut.SetDefaults()
	dut.SSE.EventsPort = 1023
	err = dut.Validate()
	if err == nil {
		t.Fatal("Validate() succeeded with EventsPort = 1023")
	}
	dut.SetDefaults()
	dut.SSE.EventsPort = 1024
	err = dut.Validate()
	if err != nil {
		t.Fatal("Validate() failed with EventsPort = 1024")
	}
	dut.SetDefaults()
	dut.SSE.EventsPort = 65536
	err = dut.Validate()
	if err == nil {
		t.Fatal("Validate() succeeded with EventsPort > 65535")
	}
	dut.SetDefaults()
	dut.SSE.SubscriptionIdleExpiration = "1.21GW"
	err = dut.Validate()
	if err == nil {
		t.Fatal("Validate() succeeded with SubscriptionIdleExpiration 1.21GW")
	}
	dut.SetDefaults()
	dut.SSE.SubscriptionIdleExpiration = "3s"
	err = dut.Validate()
	if err == nil {
		t.Fatal("Validate() succeeded with SubscriptionIdleExpiration 3s")
	}
	dut.SetDefaults()
	dut.SSE.SubscriptionIdleExpiration = "72s"
	err = dut.Validate()
	if err != nil {
		t.Fatal("Validate() failed with SubscriptionIdleExpiration 72s")
	}
	dut.SetDefaults()
	dut.SSE.SubscriptionExpirationCheckInterval = "abc"
	err = dut.Validate()
	if err == nil {
		t.Fatal("Validate() succeeded with SubscriptionExpirationCheckInterval abc")
	}
	dut.SetDefaults()
	dut.SSE.SubscriptionExpirationCheckInterval = "0s"
	err = dut.Validate()
	if err == nil {
		t.Fatal("Validate() succeeded with SubscriptionExpirationCheckInterval 0s")
	}
	dut.SetDefaults()
	dut.SSE.SubscriptionExpirationCheckInterval = "3s"
	err = dut.Validate()
	if err != nil {
		t.Fatal("Validate() failed with SubscriptionExpirationCheckInterval 3s")
	}
	dut.SetDefaults()
	dut.SSE.SubscriptionIdleExpiration = "30s"
	dut.SSE.SubscriptionExpirationCheckInterval = "20s"
	err = dut.Validate()
	if err == nil {
		t.Fatal("Validate() succeeded with SubscriptionExpirationCheckInterval more than half of SubscriptionIdleExpiration")
	}
}
