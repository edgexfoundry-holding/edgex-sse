//
// Copyright (C) 2025 Eaton
//
// SPDX-License-Identifier: Apache-2.0
//

/*
Package submgr manages subscriptions to event topics.

Summary: A subscription is identified by a randomly-generated string.
It contains an include list, an exclude list, and a channel.
Topic strings that begin with something in the include list,
and don't begin with something in the exclude list, match the
subscription.

We can give the subscription manager a topic string, and it
will return a (possibly empty) slice of channels that belong
to all subscriptions where the topic meets the include/exclude
criteria.

We can use this for EdgeX event processing - managing event
bus topic subscriptions with these APIs, then sending each event
to all channels returned from the match list above.
*/
package submgr

import (
	"github.com/edgexfoundry-holding/edgex-sse/token"
	"errors"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Struct ChannelMessage defines the messages to be sent through the managed channels.
type ChannelMessage struct {
	// EventType is either "edgex" for EdgeX Events, or "" for anything else.
	EventType string
	// Payload is the text of the event.
	Payload string
}

// Struct SubscriptionInfo collects the information we track for each subscription.
type SubscriptionInfo struct {
	// Included topic list - access under lock
	includes []string
	// Excluded topic list - access under lock
	excludes []string
	// Contains the subscription id string
	SubId string
	// Is anyone receiving on the channel? Access under lock
	active bool
	// Is anyone processing on the subscription? Access under lock
	process bool
	// If active is false, when to auto-delete this subscription? Access under lock
	expiration time.Time
	lock   *sync.RWMutex
	// The channel to send events for this subscription
	channel chan ChannelMessage
	// if channel is closed, make the flag true
	IsClosedChan bool
}

/*
Type SubscriptionManager collects the list of subscriptions, and limit configuration.

It also has methods (a Go "interface") to make it useful, like an object.
*/
type SubscriptionManager struct {
	// List of subscriptions keyed by ID - access under lock
	subscriptions map[string]*SubscriptionInfo
	// List of subscriptions, unkeyed - access under lock
	subscriptionList []*SubscriptionInfo
	lock             sync.RWMutex
	// Number of subscriptions - access with atomic functions
	numSubscriptions uint32
	// Limit on number of simultaneous subscriptions.
	subscriptionLimit uint32
	// Limit on number of items in a single subscription's include and exclude lists.
	includeExcludeLimit uint
	// Buffer size of created channels
	chanBufferSize uint
	// How long to keep subscriptions around when nobody is listening
	maxIdleSubscriptionAge time.Duration
	// How often to check for idle subscriptions
	idleSubscriptionCheckInterval time.Duration
	// Channel to tell age-out task when to stop
	stopIdleCheck chan bool
}

// Utility functions

// endWithSlash adds a slash to the end of the string, unless it's empty or already ends with a slash.
func endWithSlash(in *string) {
	if in != nil && *in != "" && !strings.HasSuffix(*in, "/") {
		*in += "/"
	}
}

// stringSliceRemove removes the given string from a slice of strings, returing the updated slice.
func stringSliceRemove(in *[]string, toRemove string) []string {
	rv := make([]string, 0, len(*in)-1)
	for _, s := range *in {
		if s != toRemove {
			rv = append(rv, s)
		}
	}
	return rv
}

// Type byLength contains the methods to sort string slices by string length using Sort().
type byLength []string

func (s byLength) Len() int {
	return len(s)
}
func (s byLength) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s byLength) Less(i, j int) bool {
	return len(s[i]) < len(s[j])
}

// SubscriptionManager methods

// getAgeOutList (an internal API) returns a list of subscription IDs that
// have been inactive too long. Is its own function so it can lock then defer unlock - 
// we cannot delete subscriptions while holding that lock.
func (s *SubscriptionManager) getAgeOutList() ([]string) {
	rv := make([]string, 0, atomic.LoadUint32(&s.numSubscriptions))
	checkTime := time.Now() // gets both wall-clock and monotonic, uses the appropriate one
	s.lock.RLock()
	defer s.lock.RUnlock()
	for subid, sub := range s.subscriptions {
		sub.lock.RLock()
		if (!sub.active) && (!sub.process) && (!sub.expiration.IsZero()) && (checkTime.After(sub.expiration)) {
			rv = append(rv, subid)
		}
		sub.lock.RUnlock()
	}
	return rv
}

// ageOutCheck (an internal API) deletes any subscriptions that have had nobody
// listening for a while.
func (s *SubscriptionManager) ageOutCheck() {
	idList := s.getAgeOutList()
	for _, subid := range idList {
		s.DeleteSubscription(subid)
	}
}

// ageOutTask (an internal API) runs in the background to periodically ageOutCheck().
func (s *SubscriptionManager) ageOutTask() {
	ticker := time.NewTicker(s.idleSubscriptionCheckInterval)
	for {
		select {
		case <-ticker.C:
			s.ageOutCheck()
		case <-s.stopIdleCheck:
			ticker.Stop()
			return
		}
	}
}

/*
Init sets up SubscriptionManager.

It initializes the storage, saves away the limit values passed in,
and starts a background task to prune inactive subscriptions.

  sublimit: Number of simultaneous subscriptions allowed.
  inexclimit: Number of simultaneous entries allowed in each subscription's include
  and exclude topic lists (the limit applies separately to each list).
  bufsize: Number of messages buffered on each channel. This is a balance between memory
  usage and blocking at high event volumes.
  maxage: How long a subscription can have nobody listening before it is auto-deleted.
  checkinterval: How often to check for auto-deletion.
*/
func (s *SubscriptionManager) Init(sublimit uint32, incexclimit uint, bufsize uint, maxage time.Duration, checkinterval time.Duration) {
	s.subscriptions = make(map[string]*SubscriptionInfo)
	s.subscriptionList = make([]*SubscriptionInfo, 0)
	s.subscriptionLimit = sublimit
	s.includeExcludeLimit = incexclimit
	s.chanBufferSize = bufsize
	s.maxIdleSubscriptionAge = maxage
	s.idleSubscriptionCheckInterval = checkinterval
	s.stopIdleCheck = make(chan bool, 2)
	go s.ageOutTask()
}

/*
Close stops SubscriptionManager.

The age-out task is stopped, and all subscriptions are deleted.
*/
func (s *SubscriptionManager) Close() {
	s.stopIdleCheck <- true
	s.lock.Lock()
	defer s.lock.Unlock()
	for _, sub := range s.subscriptionList {
		sub.lock.Lock()
		defer sub.lock.Unlock()
		sub.active = false
		sub.process = false
		close(sub.channel)
		sub.IsClosedChan = true
		sub.SubId = ""
	}
	s.subscriptionList = make([]*SubscriptionInfo, 0)
	s.subscriptions = make(map[string]*SubscriptionInfo)
	atomic.StoreUint32(&s.numSubscriptions, 0)
}

// NumSubscriptions returns the current number of subscriptions (with proper locking).
func (s *SubscriptionManager) NumSubscriptions() uint32 {
	return atomic.LoadUint32(&s.numSubscriptions)
}

/*
NewSubscription creates a new subscription and associated channel, subscribed to nothing.

It returns the randomly-generated string ID to use in other APIs to refer to
that subscription. Error is returned instead if the limit is reached,
or if there is a problem generating the ID.
*/
func (s *SubscriptionManager) NewSubscription() (string, error) {
	current_num := atomic.LoadUint32(&s.numSubscriptions)
	if current_num >= s.subscriptionLimit {
		return "", errors.New("subscription limit reached")
	}
	newid, err := token.GenerateToken()
	if err != nil {
		return "", err
	}
	newsub := new(SubscriptionInfo)
	newsub.SubId = newid
	newsub.includes = make([]string, 0)
	newsub.excludes = make([]string, 0)
	newsub.active = false
	newsub.process = false
	newsub.channel = make(chan ChannelMessage, s.chanBufferSize)
	newsub.IsClosedChan = false
	newsub.expiration = time.Now().Add(s.maxIdleSubscriptionAge)
	newsub.lock = new(sync.RWMutex)
	s.lock.Lock()
	defer s.lock.Unlock()
	s.subscriptions[newid] = newsub
	s.subscriptionList = append(s.subscriptionList, newsub)
	atomic.AddUint32(&s.numSubscriptions, 1)
	return newid, nil
}

/*
DeleteSubscription deletes the subscription identified by the given string.

The associated channel is closed.

No status is returned. If the subscription does not exist, no action is taken.
*/
func (s *SubscriptionManager) DeleteSubscription(subid string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	sub, ok := s.subscriptions[subid]
	if !ok {
		return
	}
	sub.lock.Lock()
	defer sub.lock.Unlock()
	sub.active = false
	sub.process = false
	sub.SubId = ""
	close(sub.channel)
	sub.IsClosedChan = true
	delete(s.subscriptions, subid)
	newsublist := make([]*SubscriptionInfo, 0, len(s.subscriptionList))
	for _, s := range s.subscriptionList {
		if s != sub {
			newsublist = append(newsublist, s)
		}
	}
	s.subscriptionList = newsublist
	atomic.StoreUint32(&s.numSubscriptions, uint32(len(s.subscriptions)))
}

// subscription (an internal API) returns a pointer to that subscription's information structure.
func (s *SubscriptionManager) Subscription(subid string) *SubscriptionInfo {
	s.lock.Lock()
	defer s.lock.Unlock()
	rv, ok := s.subscriptions[subid]
	if !ok {
		return nil
	}
	return rv
}

// allSubscriptions (an internal API) returns pointers to all the subscriptions' information structures.
func (s *SubscriptionManager) AllSubscriptions() []*SubscriptionInfo {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.subscriptionList
}

// Whenever subscription is deleted, subscription string of subscription info is set to empty.
// Hence below function checks whether subscription is deleted. 
func (s *SubscriptionManager) IsSubscriptionDeleted(subInfo *SubscriptionInfo) bool {
	subInfo.lock.Lock()
	defer subInfo.lock.Unlock()
	InfoId := subInfo.SubId
	if InfoId == "" {
		return true
	}
	return false
}

// Whenever subscription channel is closed, channel closed flag of subscription info is set to empty.
// Hence below function checks whether channel is closed.
func (s *SubscriptionManager) IsChannelClosed(subInfo *SubscriptionInfo) bool {
	subInfo.lock.Lock()
	defer subInfo.lock.Unlock()
	InfoStatus := subInfo.IsClosedChan
	if InfoStatus {
		return true
	}
	return false
}

/*
SubscriptionInfo returns a subscription's include/exclude lists.

Returns: the include list, the exclude list, a flag true if the subscription exists (false if not).
*/
func (s *SubscriptionManager) SubscriptionInfo(subInfo *SubscriptionInfo) ([]string, []string, bool) {
	includes := make([]string, 0)
	excludes := make([]string, 0)
	if subInfo == nil {
		return includes, excludes, false
	}
	subInfo.lock.Lock()
	defer subInfo.lock.Unlock()
	includes = subInfo.includes
	excludes = subInfo.excludes
	return includes, excludes, true
}

/*
ReceiveChannel returns the receive-end of a subscription's channel.

Error is returned if the subscription does not exist.

The send-end of the channel is returned by GetSubscribedChannels() for
topics that meet the subscription's include/exclude criteria.
*/
func (s *SubscriptionManager) ReceiveChannel(subInfo *SubscriptionInfo) (<-chan ChannelMessage, error) {
	if subInfo == nil {
		return nil, errors.New("subscription not found")
	}
	subInfo.lock.Lock()
	defer subInfo.lock.Unlock()
	return subInfo.channel, nil
}

/*
Include adds a topic prefix to a subscription's include list.

Error is returned if the subscription ID does not exist, or if the
limit on number of include/exclude list entries is reached.

Entries are coalesced - a prefix replaces all other include-list entries
that it "covers" (entries that begin with the new prefix). If a prefix
is given that is in the exclude list, that exclude-list entry is removed.

An include-list entry of "" (empty string) covers everything.
*/
func (s *SubscriptionManager) Include(subInfo *SubscriptionInfo, topicPrefix string) error {
	if subInfo == nil {
		return errors.New("subscription not found")
	}
	endWithSlash(&topicPrefix)
	// Coalescence: If this exact prefix is in the exclude list, just remove it
	subInfo.lock.Lock()
	defer subInfo.lock.Unlock()
	for _, e := range subInfo.excludes {
		if e == topicPrefix {
			subInfo.excludes = stringSliceRemove(&subInfo.excludes, topicPrefix)
			// No need to re-sort, removal will not change order
			return nil
		}
	}
	// If this "covers" entries in the include list, remove them and replace with this
	includesToRemove := make([]string, 0)
	for _, i := range subInfo.includes {
		if i == topicPrefix {
			return nil // already present
		}
		if strings.HasPrefix(i, topicPrefix) {
			includesToRemove = append(includesToRemove, i)
		}
	}
	// This is inefficient when there are multiples, that's OK
	for _, i := range includesToRemove {
		subInfo.includes = stringSliceRemove(&subInfo.includes, i)
	}
	if len(subInfo.includes) >= int(s.includeExcludeLimit) {
		return errors.New("include limit reached")
	}
	subInfo.includes = append(subInfo.includes, topicPrefix)
	sort.Sort(byLength(subInfo.includes))
	return nil
}

/*
Exclude adds a topic prefix to a subscription's exclude list.

Error is returned if the subscription ID does not exist, or if the
limit on number of include/exclude list entries is reached.

Entries are coalesced - a prefix replaces all other exclude-list entries
that it "covers" (entries that begin with the new prefix). If a prefix
is given that is in the include list, that include-list entry is removed.
*/
func (s *SubscriptionManager) Exclude(subInfo *SubscriptionInfo, topicPrefix string) error {
	if subInfo == nil {
		return errors.New("subscription not found")
	}
	endWithSlash(&topicPrefix)
	// Coalescence: If this exact prefix is in the include list, just remove it
	subInfo.lock.Lock()
	defer subInfo.lock.Unlock()
	for _, i := range subInfo.includes {
		if i == topicPrefix {
			subInfo.includes = stringSliceRemove(&subInfo.includes, topicPrefix)
			return nil
		}
	}
	// If this "covers" entries in the exclude list, remove them and replace with this
	excludesToRemove := make([]string, 0)
	for _, e := range subInfo.excludes {
		if e == topicPrefix {
			return nil // already present
		}
		if strings.HasPrefix(e, topicPrefix) {
			excludesToRemove = append(excludesToRemove, e)
		}
	}
	for _, e := range excludesToRemove {
		subInfo.excludes = stringSliceRemove(&subInfo.excludes, e)
	}
	if len(subInfo.excludes) >= int(s.includeExcludeLimit) {
		return errors.New("exclude limit reached")
	}
	subInfo.excludes = append(subInfo.excludes, topicPrefix)
	sort.Sort(byLength(subInfo.excludes))
	return nil
}

/*
SetActive tells the subscription manager if someone is listening on the
receive end of that subscription's channel.

New subscriptions default to false. If false, the subscription will not
show up in SubscribedChannels() - we don't want the event pipeline sending
events to it if nobody is listening.
*/
func (s *SubscriptionManager) SetActive(subInfo *SubscriptionInfo, isActive bool) {
	if subInfo == nil {
		return
	}
	subInfo.lock.Lock()
	defer subInfo.lock.Unlock()
	subInfo.active = isActive
	if subInfo.active {
		subInfo.expiration = time.Time{}
	} else {
		subInfo.expiration = time.Now().Add(s.maxIdleSubscriptionAge)
	}
}

/*
SetProcess tells the subscription manager if someone is processing on the
subscription.

New subscriptions default to false. It is set to true whenever ProcessSubscriptionRequest is called.
*/
func (s *SubscriptionManager) SetProcess(subInfo *SubscriptionInfo, isProcess bool) {
	if subInfo == nil {
		return
	}
	subInfo.lock.Lock()
	defer subInfo.lock.Unlock()
	subInfo.process = isProcess
	if subInfo.process {
		subInfo.expiration = time.Time{}
	} else {
		subInfo.expiration = time.Now().Add(s.maxIdleSubscriptionAge)
	}
}

/*
SubscribedChannels, given a topic string, returns the send-side of the
channels of all subscriptions that match that topic.

This is used in the event pipeline - the service will check the topic
of every event with this function, sending the event to the returned
channels if any.
*/
func (s *SubscriptionManager) SubscribedChannels(topic string) []chan<- ChannelMessage {
	currentNumSubscriptions := s.NumSubscriptions()
	// First easy, common case: nobody is subscribed to anything
	if currentNumSubscriptions == 0 {
		return nil
	}
	rv := make([]chan<- ChannelMessage, 0, currentNumSubscriptions)
	sublist := s.AllSubscriptions()
	endWithSlash(&topic)
	for _, sub := range sublist {
		useThisSub := false
		sub.lock.RLock()
		if !sub.active {
			sub.lock.RUnlock()
			continue
		}
		for _, i := range sub.includes {
			if len(i) > len(topic) {
				// List is sorted by length, once we get here it can't be a prefix
				break
			}
			if strings.HasPrefix(topic, i) {
				useThisSub = true
				// Found an include, verify we are not excluded
				for _, e := range sub.excludes {
					if len(e) > len(topic) {
						break
					}
					if strings.HasPrefix(topic, e) {
						useThisSub = false
						break
					}
				}
				break
			}
		}
		if useThisSub {
			rv = append(rv, sub.channel)
		}
		sub.lock.RUnlock()
	}
	return rv
}
