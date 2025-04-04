//
// Copyright (C) 2025 Eaton
//
// SPDX-License-Identifier: Apache-2.0
//

package submgr

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestAddRemove(t *testing.T) {
	var dut SubscriptionManager
	dut.Init(2, 3, 4, 300*time.Second, 30*time.Second)
	defer dut.Close()
	this_num := dut.NumSubscriptions()
	if this_num != 0 {
		t.Fatalf("Wrong subscription count: %v instead of 0", this_num)
	}
	sublist := dut.AllSubscriptions()
	if len(sublist) != 0 {
		t.Fatalf("Wrong subscription count: %v instead of 0", len(sublist))
	}
	if dut.chanBufferSize != 4 || dut.includeExcludeLimit != 3 || dut.subscriptionLimit != 2 {
		t.Fatalf("Problem in init: sub/inc-exc limit / bufsize %d/%d/%d, expected 2/3/4", dut.subscriptionLimit, dut.includeExcludeLimit, dut.chanBufferSize)
	}
	if dut.idleSubscriptionCheckInterval != (30 * time.Second) || dut.maxIdleSubscriptionAge != (300 * time.Second) {
		t.Fatalf("Problem in init: intervals %v/%v, expected 30s/300s", dut.idleSubscriptionCheckInterval, dut.maxIdleSubscriptionAge)
	}
	first, err := dut.NewSubscription()
	if err != nil {
		t.Fatalf("Error adding first subscription: %v", err)
	}
	this_num = dut.NumSubscriptions()
	if this_num != 1 {
		t.Fatalf("Wrong subscription count: %v instead of 1", this_num)
	}
	sublist = dut.AllSubscriptions()
	if len(sublist) != 1 {
		t.Fatalf("Wrong subscription count: %v instead of 1", len(sublist))
	}
	second, err := dut.NewSubscription()
	if err != nil {
		t.Fatalf("Error adding second subscription: %v", err)
	}
	this_num = dut.NumSubscriptions()
	if this_num != 2 {
		t.Fatalf("Wrong subscription count: %v instead of 2", this_num)
	}
	sublist = dut.AllSubscriptions()
	if len(sublist) != 2 {
		t.Fatalf("Wrong subscription count: %v instead of 2", len(sublist))
	}
	third, err := dut.NewSubscription()
	if err == nil || third != "" {
		t.Fatalf("Successfully added third subscription (ID %s), expected failure", third)
	}
	this_num = dut.NumSubscriptions()
	if this_num != 2 {
		t.Fatalf("Wrong subscription count: %v instead of 2", this_num)
	}
	if first == second || first == "" || second == "" {
		t.Fatalf("Subscriptions got same ID or one got no ID (%s/%s)", first, second)
	}
	dut.DeleteSubscription("This is not a valid subscription ID")
	this_num = dut.NumSubscriptions()
	if this_num != 2 {
		t.Fatalf("Wrong subscription count: %v instead of 2", this_num)
	}
	dut.DeleteSubscription(first)
	this_num = dut.NumSubscriptions()
	if this_num != 1 {
		t.Fatalf("Wrong subscription count: %v instead of 1", this_num)
	}
	sublist = dut.AllSubscriptions()
	if len(sublist) != 1 {
		t.Fatalf("Wrong subscription count: %v instead of 1", len(sublist))
	}
	dut.DeleteSubscription(second)
	this_num = dut.NumSubscriptions()
	if this_num != 0 {
		t.Fatalf("Wrong subscription count: %v instead of 0", this_num)
	}
	sublist = dut.AllSubscriptions()
	if len(sublist) != 0 {
		t.Fatalf("Wrong subscription count: %v instead of 0", len(sublist))
	}
}

func TestEndWithSlash(t *testing.T) {
	var s1 string
	var s2 string
	var s3 string
	var s4 string
	s1 = "edgex/events/device"
	s2 = "edgex/events/device/"
	s3 = "/"
	s4 = ""
	endWithSlash(&s1)
	endWithSlash(&s2)
	endWithSlash(&s3)
	endWithSlash(&s4)
	if s1 != "edgex/events/device/" {
		t.Fatalf("Failure, %s unexpected", s1)
	}
	if s2 != "edgex/events/device/" {
		t.Fatalf("Failure, %s unexpected", s2)
	}
	if s3 != "/" {
		t.Fatalf("Failure, %s unexpected", s3)
	}
	if s4 != "" {
		t.Fatalf("Failure, %s unexpected", s4)
	}
}

func TestInclude(t *testing.T) {
	var dut SubscriptionManager
	dut.Init(2, 3, 4, 300*time.Second, 30*time.Second)
	subid, err := dut.NewSubscription()
	if err != nil {
		t.Fatalf("Error creating subscription: %v", err)
	}
	subinfo := dut.Subscription(subid)
	if subinfo == nil {
		t.Fatal("Subscription not found")
	}
	includes, excludes, exists := dut.SubscriptionInfo(subinfo)
	if !exists {
		t.Fatal("New subscription not found")
	}
	if len(includes) != 0 || len(excludes) != 0 {
		t.Fatal("New subscription contains includes/excludes already")
	}
	err1 := dut.Include(subinfo, "edgex/events/device/Profile1/Device1/Command1/")
	// Should be equivalent and not count towards limit
	err2 := dut.Include(subinfo, "edgex/events/device/Profile1/Device1/Command1")
	err3 := dut.Include(subinfo, "edgex/events/device/Profile1/Device1/Command2")
	err4 := dut.Include(subinfo, "edgex/events/device/Profile1/Device1/Command3")
	// should hit the limit
	err5 := dut.Include(subinfo, "edgex/events/device/Profile1/Device1/Command4")
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		t.Fatal("Unexpected error adding includes")
	}
	if err5 == nil {
		t.Fatal("Unexpected success going over include limit")
	}
	includes, excludes, exists = dut.SubscriptionInfo(subinfo)
	if !exists {
		t.Fatal("Subscription not found")
	}
	if len(includes) != 3 || len(excludes) != 0 {
		t.Fatalf("Subscription contains %d/%d includes/excludes, expected 3/0", len(includes), len(excludes))
	}
	allsubs := dut.AllSubscriptions()
	if len(allsubs) != 1 {
		t.Fatalf("Wrong number of subscriptions %d, expected 1", len(allsubs))
	}
	if allsubs[0] == nil || !reflect.DeepEqual(includes, allsubs[0].includes) {
		t.Fatal("Include list from getAllSubscriptions inconsistent with GetSubscriptionInfo")
	}
	// Convert to map for easy checking
	incs := make(map[string]int, 0)
	for _, i := range includes {
		incs[i] = 1
	}
	// they should all end in slashes
	_, ok := incs["edgex/events/device/Profile1/Device1/Command1"]
	if ok {
		t.Fatal("Includes included item not ending in slash")
	}
	_, ok = incs["edgex/events/device/Profile1/Device1/Command1/"]
	if !ok {
		t.Fatal("Command1 not found in includes")
	}
	_, ok = incs["edgex/events/device/Profile1/Device1/Command2/"]
	if !ok {
		t.Fatal("Command2 not found in includes")
	}
	_, ok = incs["edgex/events/device/Profile1/Device1/Command3/"]
	if !ok {
		t.Fatal("Command3 not found in includes")
	}
	err = dut.Include(subinfo, "edgex/events/device")
	if err != nil {
		t.Fatalf("Error inserting superceding include: %v", err)
	}
	includes, excludes, exists = dut.SubscriptionInfo(subinfo)
	if !exists {
		t.Fatal("Subscription not found")
	}
	if len(includes) != 1 || len(excludes) != 0 {
		t.Fatalf("Subscription contains %d/%d includes/excludes, expected 1/0", len(includes), len(excludes))
	}
	if includes[0] != "edgex/events/device/" {
		t.Fatalf("Include list contained '%s' instead of 'edgex/events/device/", includes[0])
	}
	// Add test of Close() clearing
	dut.Close()
	// _, _, exists = dut.SubscriptionInfo(subinfo)
	check := dut.IsChannelClosed(subinfo)
	if !check {
		t.Fatal("Subscription still there after Close()")
	}
	check = dut.IsSubscriptionDeleted(subinfo)
	if !check {
		t.Fatal("Subscription not deleted after Close()")
	}
	if dut.NumSubscriptions() != 0 {
		t.Fatal("Nonzero number of subscriptions after Close()")
	}
}

func TestExclude(t *testing.T) {
	var dut SubscriptionManager
	dut.Init(2, 3, 4, 300*time.Second, 30*time.Second)
	defer dut.Close()
	subid, err := dut.NewSubscription()
	if err != nil {
		t.Fatalf("Error creating subscription: %v", err)
	}
	subinfo := dut.Subscription(subid)
	if subinfo == nil {
		t.Fatal("Subscription not found")
	}
	includes, excludes, exists := dut.SubscriptionInfo(subinfo)
	if !exists {
		t.Fatal("New subscription not found")
	}
	if len(includes) != 0 || len(excludes) != 0 {
		t.Fatal("New subscription contains includes/excludes already")
	}
	err1 := dut.Exclude(subinfo, "edgex/events/device/Profile1/Device1/Command1/")
	// Should be equivalent and not count towards limit
	err2 := dut.Exclude(subinfo, "edgex/events/device/Profile1/Device1/Command1")
	err3 := dut.Exclude(subinfo, "edgex/events/device/Profile1/Device1/Command2")
	err4 := dut.Exclude(subinfo, "edgex/events/device/Profile1/Device1/Command3")
	// should hit the limit
	err5 := dut.Exclude(subinfo, "edgex/events/device/Profile1/Device1/Command4")
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		t.Fatal("Unexpected error adding excludes")
	}
	if err5 == nil {
		t.Fatal("Unexpected success going over exclude limit")
	}
	excs := make(map[string]int, 0)
	includes, excludes, exists = dut.SubscriptionInfo(subinfo)
	if !exists {
		t.Fatal("Subscription not found")
	}
	if len(includes) != 0 || len(excludes) != 3 {
		t.Fatalf("Subscription contains %d/%d includes/excludes, expected 0/3", len(includes), len(excludes))
	}
	allsubs := dut.AllSubscriptions()
	if len(allsubs) != 1 {
		t.Fatalf("Wrong number of subscriptions %d, expected 1", len(allsubs))
	}
	if allsubs[0] == nil || !reflect.DeepEqual(excludes, allsubs[0].excludes) {
		t.Fatal("Include list from getAllSubscriptions inconsistent with GetSubscriptionInfo")
	}
	for _, e := range excludes {
		excs[e] = 1
	}
	_, ok := excs["edgex/events/device/Profile1/Device1/Command1"]
	if ok {
		t.Fatal("Excludes included item not ending in slash")
	}
	_, ok = excs["edgex/events/device/Profile1/Device1/Command1/"]
	if !ok {
		t.Fatal("Command1 not found in excludes")
	}
	_, ok = excs["edgex/events/device/Profile1/Device1/Command2/"]
	if !ok {
		t.Fatal("Command2 not found in excludes")
	}
	_, ok = excs["edgex/events/device/Profile1/Device1/Command3/"]
	if !ok {
		t.Fatal("Command3 not found in excludes")
	}
	err = dut.Exclude(subinfo, "edgex/events/device")
	if err != nil {
		t.Fatalf("Error inserting superceding exclude: %v", err)
	}
	includes, excludes, exists = dut.SubscriptionInfo(subinfo)
	if !exists {
		t.Fatal("Subscription not found")
	}
	if len(includes) != 0 || len(excludes) != 1 {
		t.Fatalf("Subscription contains %d/%d includes/excludes, expected 0/1", len(includes), len(excludes))
	}
	if excludes[0] != "edgex/events/device/" {
		t.Fatalf("Exclude list contained '%s' instead of 'edgex/events/device/", excludes[0])
	}
}

func TestIncludeExclude(t *testing.T) {
	var dut SubscriptionManager
	dut.Init(2, 3, 4, 300*time.Second, 30*time.Second)
	defer dut.Close()
	subid, err := dut.NewSubscription()
	if err != nil {
		t.Fatalf("Error creating subscription: %v", err)
	}
	subinfo := dut.Subscription(subid)
	if subinfo == nil {
		t.Fatal("Subscription not found")
	}
	// Test nonexistent-subscription code path while we're here
	_, _, ok := dut.SubscriptionInfo(nil)
	if ok {
		t.Fatal("Failed to catch that a subscription Info did not exist")
	}
	err = dut.Include(nil, "a/b/c")
	if err == nil {
		t.Fatal("Failed to catch that a subscription Info did not exist")
	}
	err = dut.Exclude(nil, "a/b/c/d")
	if err == nil {
		t.Fatal("Failed to catch that a subscription Info did not exist")
	}
	err = dut.Include(subinfo, "a/b/c")
	if err != nil {
		t.Fatalf("Include unexpectedly failed: %v", err)
	}
	err = dut.Exclude(subinfo, "a/b/c/d")
	if err != nil {
		t.Fatalf("Exclude unexpectedly failed: %v", err)
	}
	includes, excludes, exists := dut.SubscriptionInfo(subinfo)
	if !exists {
		t.Fatalf("Subscription not found")
	}
	if len(includes) != 1 || len(excludes) != 1 {
		t.Fatalf("Wrong number of includes/excludes (%d/%d), expected 1/1", len(includes), len(excludes))
	}
	if includes[0] != "a/b/c/" {
		t.Fatalf("Wrong include '%s', expected 'a/b/c/", includes[0])
	}
	if excludes[0] != "a/b/c/d/" {
		t.Fatalf("Wrong exclude '%s', expected 'a/b/c/d/", excludes[0])
	}
	err = dut.Include(subinfo, "a/b/c/d")
	if err != nil {
		t.Fatalf("Include unexpectedly failed: %v", err)
	}
	includes, excludes, exists = dut.SubscriptionInfo(subinfo)
	if !exists {
		t.Fatalf("Subscription not found")
	}
	if len(includes) != 1 || len(excludes) != 0 {
		t.Fatalf("Wrong number of includes/excludes (%d/%d), expected 1/0", len(includes), len(excludes))
	}
	if includes[0] != "a/b/c/" {
		t.Fatalf("Wrong include '%s', expected 'a/b/c/", includes[0])
	}
	err = dut.Exclude(subinfo, "a/b/c")
	if err != nil {
		t.Fatalf("Exclude unexpectedly failed: %v", err)
	}
	includes, excludes, exists = dut.SubscriptionInfo(subinfo)
	if !exists {
		t.Fatalf("Subscription not found")
	}
	if len(includes) != 0 || len(excludes) != 0 {
		t.Fatalf("Wrong number of includes/excludes (%d/%d), expected 0/0", len(includes), len(excludes))
	}
}

func TestSortition(t *testing.T) {
	var dut SubscriptionManager
	dut.Init(10, 10, 10, 300*time.Second, 30*time.Second)
	defer dut.Close()
	subid, err := dut.NewSubscription()
	if err != nil {
		t.Fatalf("Error creating subscription: %v", err)
	}
	subinfo := dut.Subscription(subid)
	if subinfo == nil {
		t.Fatal("Subscription not found")
	}
	err = dut.Include(subinfo, "a/b/c")
	if err != nil {
		t.Fatalf("Include unexpectedly failed: %v", err)
	}
	err = dut.Include(subinfo, "x/y")
	if err != nil {
		t.Fatalf("Include unexpectedly failed: %v", err)
	}
	err = dut.Include(subinfo, "b")
	if err != nil {
		t.Fatalf("Include unexpectedly failed: %v", err)
	}
	err = dut.Include(subinfo, "foo/bar/baz/quux")
	if err != nil {
		t.Fatalf("Include unexpectedly failed: %v", err)
	}
	err = dut.Exclude(subinfo, "w/x/y")
	if err != nil {
		t.Fatalf("Exclude unexpectedly failed: %v", err)
	}
	err = dut.Exclude(subinfo, "d/e")
	if err != nil {
		t.Fatalf("Exclude unexpectedly failed: %v", err)
	}
	includes, excludes, exists := dut.SubscriptionInfo(subinfo)
	if !exists {
		t.Fatalf("Subscription not found")
	}
	if len(includes) != 4 || len(excludes) != 2 {
		t.Fatalf("Wrong number of includes/excludes (%d/%d), expected 4/2", len(includes), len(excludes))
	}
	if includes[0] != "b/" || includes[1] != "x/y/" || includes[2] != "a/b/c/" || includes[3] != "foo/bar/baz/quux/" {
		t.Fatalf("Includes sort incorrect: %v", includes)
	}
	if excludes[0] != "d/e/" || excludes[1] != "w/x/y/" {
		t.Fatalf("Excludes sort incorrect: %v", excludes)
	}
}

func receive(c <-chan ChannelMessage, e chan<- error, expected []ChannelMessage) {
	var numReceived int = 0
	for msg := range c {
		for _, exp := range expected {
			if msg.EventType == exp.EventType && msg.Payload == exp.Payload {
				numReceived++
			}
		}
	}
	// Channel c has now been closed from the sending side
	// Can't call Fatal() from another goroutine, so tell main routine whether we succeeded
	if numReceived != len(expected) {
		e <- errors.New("expected message(s) not received")
	} else {
		e <- nil
	}
}

func TestChannelSimple(t *testing.T) {
	var dut SubscriptionManager
	dut.Init(2, 3, 4, 300*time.Second, 30*time.Second)
	// Test no-subscription case
	badchanlist := dut.SubscribedChannels("a/b/c/d")
	if len(badchanlist) != 0 {
		t.Fatal("Got channels somehow from empty subscription manager")
	}
	subid, err := dut.NewSubscription()
	if err != nil {
		t.Fatalf("Error creating subscription: %v", err)
	}
	subinfo := dut.Subscription(subid)
	if subinfo == nil {
		t.Fatal("Subscription not found")
	}
	err = dut.Include(subinfo, "a/b/c")
	if err != nil {
		t.Fatalf("Include unexpectedly failed: %v", err)
	}
	rxchan, err1 := dut.ReceiveChannel(subinfo)
	if rxchan == nil || err1 != nil {
		t.Fatal("GetReceiveChannel unexpectedly failed")
	}
	chanlist := dut.SubscribedChannels("a/b/c/d")
	if len(chanlist) != 0 {
		t.Fatal("Found channel despite subscription not being active yet")
	}
	dut.SetActive(nil, false)
	dut.SetActive(subinfo, true)
	testchan, err2 := dut.ReceiveChannel(nil)
	if len(testchan) > 0 || err2 == nil {
		t.Fatal("Unexpected success getting channels for nonexistent subscription")
	}
	errs := make(chan error, 1)
	var testMessage ChannelMessage
	testMessage.EventType = "edgex"
	testMessage.Payload = "First Event"
	exp_msgs := make([]ChannelMessage, 0, 1)
	exp_msgs = append(exp_msgs, testMessage)
	go receive(rxchan, errs, exp_msgs)
	chanlist = dut.SubscribedChannels("a/b/c/d")
	if len(chanlist) == 0 {
		t.Fatal("Channel unexpectedly not found")
	}
	// Send to the channel
	for _, c := range chanlist {
		c <- testMessage
	}
	dut.SetActive(subinfo, false)
	chanlist = dut.SubscribedChannels("a/b/c/d")
	if len(chanlist) != 0 {
		t.Fatal("Found channel despite subscription being set inactive")
	}
	// Deleting all the subscriptions will close the channels
	// then receive() will terminate
	dut.Close()
	// Wait for and get the result from receive()
	err = <-errs
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
}

// Structure to track messages/topics
type SendVector struct {
	topic string
	msg   ChannelMessage
}

var sv = [...]SendVector{
	{topic: "edgex/events/device/Bacon-Cape/Virtual-Bacon-Cape-01/mPercentLoad", msg: ChannelMessage{EventType: "edgex", Payload: "Event 1"}},
	{topic: "edgex/events/device/Bacon-Cape/Virtual-Bacon-Cape-01/mACIA", msg: ChannelMessage{EventType: "edgex", Payload: "Event 2"}},
	{topic: "edgex/events/device/Bacon-Cape/Virtual-Bacon-Cape-03/mACIA", msg: ChannelMessage{EventType: "edgex", Payload: "Event 3"}},
	{topic: "edgex/events/control/Shutdown", msg: ChannelMessage{EventType: "", Payload: "Event 4"}},
	{topic: "edgex/events/device/Bacon-Cape/Virtual-Bacon-Cape-02/mWA", msg: ChannelMessage{EventType: "edgex", Payload: "Event 5"}},
}

func TestChannelComplex(t *testing.T) {
	var dut SubscriptionManager
	dut.Init(10, 10, 10, 300*time.Second, 30*time.Second)
	defer dut.Close()
	sub1, err1 := dut.NewSubscription()
	if err1 != nil {
		t.Fatal("Unexpected failure for new subscription")
	}
	sub2, err2 := dut.NewSubscription()
	if err2 != nil {
		t.Fatal("Unexpected failure for new subscription")
	}
	subinfo1 := dut.Subscription(sub1)
	if subinfo1 == nil {
		t.Fatal("Subscription not found")
	}
	subinfo2 := dut.Subscription(sub2)
	if subinfo2 == nil {
		t.Fatal("Subscription not found")
	}
	// Subscription 1: everything except one device
	err := dut.Include(subinfo1, "")
	if err != nil {
		t.Fatal("Unexpected failure in Include()")
	}
	err = dut.Exclude(subinfo1, "edgex/events/device/Bacon-Cape/Virtual-Bacon-Cape-03")
	if err != nil {
		t.Fatal("Unexpected failure in Exclude()")
	}
	// Subscription 2: 3 devices, minus one channel
	err = dut.Include(subinfo2, "edgex/events/device/Bacon-Cape/Virtual-Bacon-Cape-01")
	if err != nil {
		t.Fatal("Unexpected failure in Include()")
	}
	err = dut.Include(subinfo2, "edgex/events/device/Bacon-Cape/Virtual-Bacon-Cape-02")
	if err != nil {
		t.Fatal("Unexpected failure in Include()")
	}
	err = dut.Include(subinfo2, "edgex/events/device/Bacon-Cape/Virtual-Bacon-Cape-03")
	if err != nil {
		t.Fatal("Unexpected failure in Include()")
	}
	err = dut.Exclude(subinfo2, "edgex/events/device/Bacon-Cape/Virtual-Bacon-Cape-01/mACIA")
	if err != nil {
		t.Fatal("Unexpected failure in Exclude()")
	}
	// Set up events to send and expect
	exp1 := make([]ChannelMessage, 0, 5)
	exp2 := make([]ChannelMessage, 0, 5)
	exp1 = append(exp1, sv[0].msg)
	exp1 = append(exp1, sv[1].msg)
	exp1 = append(exp1, sv[4].msg)
	exp2 = append(exp2, sv[0].msg)
	exp2 = append(exp2, sv[2].msg)
	exp2 = append(exp2, sv[4].msg)
	rchan1, errc1 := dut.ReceiveChannel(subinfo1)
	if errc1 != nil {
		t.Fatal("Failed to get receive channel 1")
	}
	rchan2, errc2 := dut.ReceiveChannel(subinfo2)
	if errc2 != nil {
		t.Fatal("Failed to get receive channel 2")
	}
	errs := make(chan error, 2)
	go receive(rchan1, errs, exp1)
	go receive(rchan2, errs, exp2)
	dut.SetActive(subinfo1, true)
	dut.SetActive(subinfo2, true)
	for _, s := range sv {
		chanlist := dut.SubscribedChannels(s.topic)
		for _, c := range chanlist {
			c <- s.msg
		}
	}
	dut.DeleteSubscription(sub2) // closes channel
	rxerr2 := <-errs
	if rxerr2 != nil {
		t.Fatalf("Error receiving in subscription 2: %v", rxerr2)
	}
	dut.DeleteSubscription(sub1)
	rxerr1 := <-errs
	if rxerr1 != nil {
		t.Fatalf("Error receiving in subscription 1: %v", rxerr1)
	}
}

func BenchmarkLookups(b *testing.B) {
	var dut SubscriptionManager
	dut.Init(10, 10, 10, 300*time.Second, 30*time.Second)
	defer dut.Close()
	sub1, _ := dut.NewSubscription()
	sub2, _ := dut.NewSubscription()
	subinfo1 := dut.Subscription(sub1)
	if subinfo1 == nil {
		b.Fatal("Subscription not found")
	}
	subinfo2 := dut.Subscription(sub2)
	if subinfo2 == nil {
		b.Fatal("Subscription not found")
	}
	_ = dut.Include(subinfo1, "")
	_ = dut.Exclude(subinfo1, "edgex/events/device/Bacon-Cape/Virtual-Bacon-Cape-03")
	_ = dut.Include(subinfo2, "edgex/events/device/Bacon-Cape/Virtual-Bacon-Cape-01")
	_ = dut.Include(subinfo2, "edgex/events/device/Bacon-Cape/Virtual-Bacon-Cape-02")
	_ = dut.Include(subinfo2, "edgex/events/device/Bacon-Cape/Virtual-Bacon-Cape-03")
	_ = dut.Exclude(subinfo2, "edgex/events/device/Bacon-Cape/Virtual-Bacon-Cape-01/mACIA")
	dut.SetActive(subinfo1, true)
	dut.SetActive(subinfo2, true)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = dut.SubscribedChannels(sv[i%4].topic)
	}
}

// Goroutine helper functions for concurrency test

// Constantly add and remove an include or exclude
func frobIncExc(s *SubscriptionManager, subid string, prefix string, include bool, stopme <-chan bool) {
	var wantToStop bool
	wantToStop = false
	subinfo := s.Subscription(subid)
	for !wantToStop {
		if include {
			s.Include(subinfo, prefix)
			time.Sleep(time.Millisecond)
			s.Exclude(subinfo, prefix)
		} else {
			s.Exclude(subinfo, prefix)
			time.Sleep(time.Millisecond)
			s.Include(subinfo, prefix)
		}
		select {
		case <-stopme:
			wantToStop = true
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

// Constantly add and remove subscriptions
func frobSubs(s *SubscriptionManager, prefix string, stopme <-chan bool) {
	var wantToStop bool
	wantToStop = false
	subid, err := s.NewSubscription()
	subinfo := s.Subscription(subid)
	for !wantToStop {
		if err == nil {
			s.SetActive(subinfo, true)
			s.Include(subinfo, prefix)
			time.Sleep(47 * time.Millisecond)
			s.DeleteSubscription(subid)
		}
		select {
		case <-stopme:
			wantToStop = true
		default:
			// Do nothing if no message received
		}
	}
}

// Meant to run with go test -race to ensure we locked everything correctly
func TestConcurrency(t *testing.T) {
	var dut SubscriptionManager
	dut.Init(10, 10, 10, 300*time.Second, 30*time.Second)
	defer dut.Close()
	subid, err := dut.NewSubscription()
	if err != nil {
		t.Fatal("Unexpected error creating subscription")
	}
	subinfo := dut.Subscription(subid)
	if subinfo == nil {
		t.Fatal("Subscription not found")
	}
	dut.Include(subinfo, "a/b")
	dut.Exclude(subinfo, "a/b/c")
	dut.SetActive(subinfo, true)
	stopchans := make([]chan bool, 4)
	for i := range stopchans {
		stopchans[i] = make(chan bool)
	}
	var wantToStop bool
	wantToStop = false
	go frobSubs(&dut, "x/y/", stopchans[0])
	go frobSubs(&dut, "", stopchans[1])
	go frobIncExc(&dut, subid, "x/y", true, stopchans[2])
	go frobIncExc(&dut, subid, "x/y/z/", false, stopchans[3])
	timer := time.NewTimer(10 * time.Second)
	for !wantToStop {
		list := dut.SubscribedChannels("a/b/c/d")
		if len(list) > 1 {
			t.Fatal("a/b/c/d returned more than 1 subscription")
		}
		list = dut.SubscribedChannels("x/y/z")
		if len(list) > 3 {
			t.Fatal("x/y/z returned more than 2 subscriptions")
		}
		list = dut.SubscribedChannels("1/2")
		if len(list) > 1 {
			t.Fatal("1/2 returned more than 1 subscription")
		}
		select {
		case <-timer.C:
			wantToStop = true
		default:
			// No sleep here, hammer continuously
		}
	}
	for _, c := range stopchans {
		c <- true
	}
}

func TestAging(t *testing.T) {
	var dut SubscriptionManager
	dut.Init(10, 10, 10, 3*time.Second, 500*time.Millisecond)
	defer dut.Close()
	subid1, _ := dut.NewSubscription()
	subid2, _ := dut.NewSubscription()
	subid3, _ := dut.NewSubscription()
	subinfo1 := dut.Subscription(subid1)
	if subinfo1 == nil {
		t.Fatal("Subscription not found")
	}
	subinfo2 := dut.Subscription(subid2)
	if subinfo2 == nil {
		t.Fatal("Subscription not found")
	}
	subinfo3 := dut.Subscription(subid3)
	if subinfo3 == nil {
		t.Fatal("Subscription not found")
	}
	dut.SetActive(subinfo2, true)
	dut.SetActive(subinfo3, true)
	time.Sleep(4*time.Second)
	exists := dut.IsSubscriptionDeleted(subinfo1)
	// _, _, exists := dut.SubscriptionInfo(subinfo1)
	if !exists {
		t.Fatal("Never-active subscription did not age out")
	}
	exists = dut.IsSubscriptionDeleted(subinfo2)
	// _, _, exists = dut.SubscriptionInfo(subinfo2)
	if exists {
		t.Fatal("Active subscription 2 aged out")
	}
	exists = dut.IsSubscriptionDeleted(subinfo3)
	// _, _, exists = dut.SubscriptionInfo(subinfo3)
	if exists {
		t.Fatal("Active subscription 3 aged out")
	}
	dut.SetActive(subinfo2, false)
	time.Sleep(4*time.Second)
	// _, _, exists = dut.SubscriptionInfo(subinfo2)
	exists = dut.IsSubscriptionDeleted(subinfo2)
	if !exists {
		t.Fatal("Inactive subscription 2 did not age out")
	}
	exists = dut.IsSubscriptionDeleted(subinfo3)
	// _, _, exists = dut.SubscriptionInfo(subinfo3)
	if exists {
		t.Fatal("Active subscription 3 aged out")
	}
}
