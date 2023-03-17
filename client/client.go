// Copyright 2022 PolyCrypt GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package client

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"log"
	"math/big"
	"net/url"
	"perun.network/go-perun/channel"
	"perun.network/go-perun/client"
	"perun.network/go-perun/watcher/local"
	"perun.network/go-perun/wire"
	"perun.network/go-perun/wire/net/simple"
	channel2 "perun.network/perun-cardano-backend/channel"
	wallet2 "perun.network/perun-cardano-backend/wallet"
	"polycry.pt/poly-go/sync"
	"strconv"
	"time"
)

type Observer interface {
	UpdateState(string)
	UpdateBalance(string)
	GetID() uuid.UUID
}

type Subject interface {
	Register(observer Observer)
	Deregister(observer Observer)
	notifyAllState(from, to *channel.State)
	notifyAllBalance()
}

// PaymentClient is a payment channel client.
type PaymentClient struct {
	observerMutex sync.Mutex
	balanceMutex  sync.Mutex
	Name          string
	PerunClient   *client.Client // The core Perun client.
	Channel       *PaymentChannel
	Account       wallet2.RemoteAccount // The Account we use for on-chain and off-chain transactions.
	wAddr         wire.Address          // The address we use for off-chain communication.
	currency      channel.Asset         // The currency we expect to get paid in.
	channels      chan *PaymentChannel  // Accepted payment channels.
	onUpdate      func(from, to *channel.State)
	observers     []Observer
	WalletURL     *url.URL
	balance       int64
}

func (c *PaymentClient) Register(observer Observer) {
	log.Printf("Registering observer %s on client %s", observer.GetID().String(), c.Name)
	c.observerMutex.Lock()
	defer c.observerMutex.Unlock()
	c.observers = append(c.observers, observer)
	if c.Channel != nil {
		observer.UpdateState(FormatState(c.Channel, c.Channel.ch.State().Clone()))
	}
	observer.UpdateBalance(FormatBalance(c.GetBalance()))
}

func (c *PaymentClient) Deregister(observer Observer) {
	c.observerMutex.Lock()
	defer c.observerMutex.Unlock()
	for i, o := range c.observers {
		if o.GetID().String() == observer.GetID().String() {
			c.observers[i] = c.observers[len(c.observers)-1]
			c.observers = c.observers[:len(c.observers)-1]
		}

	}
}

func (c *PaymentClient) notifyAllState(_, to *channel.State) {
	c.observerMutex.Lock()
	defer c.observerMutex.Unlock()
	str := FormatState(c.Channel, to)
	for _, o := range c.observers {
		o.UpdateState(str)
	}
}

func FormatBalance(bal int64) string {
	balBig := big.NewInt(bal)
	balAda, _ := LovelaceToAda(balBig).Float64()
	balString := strconv.FormatFloat(balAda, 'f', 6, 64)
	return balString
}

func (c *PaymentClient) notifyAllBalance(bal int64) {
	str := FormatBalance(bal)
	for _, o := range c.observers {
		o.UpdateBalance(str)
	}
}

func (c *PaymentClient) PollBalances() {
	for {
		time.Sleep(1 * time.Second)
		bal, err := c.QueryBalance()
		if err != nil {
			log.Println("Error getting balance: ", err)
			continue
		}
		c.balanceMutex.Lock()
		if bal != c.balance {
			c.balance = bal
			c.notifyAllBalance(bal)
		}
		c.balanceMutex.Unlock()
	}
}

func (c *PaymentClient) GetBalance() int64 {
	c.balanceMutex.Lock()
	defer c.balanceMutex.Unlock()
	return c.balance
}

// SetupPaymentClient creates a new payment client.
func SetupPaymentClient(
	name string,
	bus wire.Bus, // bus is used of off-chain communication.
	acc wallet2.RemoteAccount, // acc is the address of the Account to be used for signing transactions.
	pabHost string,
	wallet *wallet2.RemoteWallet,
	asset channel.Asset,
	cardanoWalletServerURL string,
) (*PaymentClient, error) {
	walletUrl, err := url.Parse(cardanoWalletServerURL)
	if err != nil {
		return nil, fmt.Errorf("unable to parse cardano wallet server url: %w", err)
	}
	pab, err := channel2.NewPAB(pabHost, acc)
	// Setup funder

	funder := channel2.NewFunder(pab)

	// Setup adjudicator.
	adjudicator := channel2.NewAdjudicator(pab)

	// Setup dispute watcher.
	watcher, err := local.NewWatcher(adjudicator)
	if err != nil {
		return nil, fmt.Errorf("intializing watcher: %w", err)
	}

	wAddr := simple.NewAddress(acc.Address().String())
	// Setup Perun client.
	perunClient, err := client.New(wAddr, bus, funder, adjudicator, wallet, watcher)
	if err != nil {
		return nil, errors.WithMessage(err, "creating client")
	}

	// Create client and start request handler.
	c := &PaymentClient{
		Name:        name,
		PerunClient: perunClient,
		Account:     acc,
		wAddr:       wAddr,
		currency:    asset,
		channels:    make(chan *PaymentChannel, 1),
		WalletURL:   walletUrl,
		balance:     0,
	}
	go c.PollBalances()
	go perunClient.Handle(c, c)

	return c, nil
}

// OpenChannel opens a new channel with the specified peer and funding.
func (c *PaymentClient) OpenChannel(peer wire.Address, amount float64) *PaymentChannel {
	// We define the channel participants. The proposer always has index 0. Here
	// we use the on-chain addresses as off-chain addresses, but we could also
	// use different ones.
	log.Println("OpenChannel called")
	participants := []wire.Address{c.WireAddress(), peer}

	// We create an initial allocation which defines the starting balances.
	initAlloc := channel.NewAllocation(2, c.currency)
	initAlloc.SetAssetBalances(c.currency, []channel.Bal{
		AdaToLovelace(big.NewFloat(amount)), // Our initial balance.
		AdaToLovelace(big.NewFloat(amount)), // Peer's initial balance.
	})
	log.Println("Created Allocation")

	// Prepare the channel proposal by defining the channel parameters.
	challengeDuration := uint64(10) // On-chain challenge duration in seconds.
	proposal, err := client.NewLedgerChannelProposal(
		challengeDuration,
		c.Account.Address(),
		initAlloc,
		participants,
	)
	if err != nil {
		panic(err)
	}

	log.Println("Created Proposal")

	// Send the proposal.
	ch, err := c.PerunClient.ProposeChannel(context.TODO(), proposal)
	if err != nil {
		panic(err)
	}

	log.Println("Sent Channel")

	// Start the on-chain event watcher. It automatically handles disputes.
	c.startWatching(ch)

	log.Println("Started Watching")

	c.Channel = newPaymentChannel(ch, c.currency)
	c.Channel.ch.OnUpdate(c.notifyAllState)
	c.notifyAllState(nil, ch.State())
	return c.Channel
}

// startWatching starts the dispute watcher for the specified channel.
func (c *PaymentClient) startWatching(ch *client.Channel) {
	go func() {
		err := ch.Watch(c)
		if err != nil {
			fmt.Printf("Watcher returned with error: %v", err)
		}
	}()
}

// AcceptedChannel returns the next accepted channel.
func (c *PaymentClient) AcceptedChannel() *PaymentChannel {
	c.Channel = <-c.channels
	c.Channel.ch.OnUpdate(c.notifyAllState)
	c.notifyAllState(nil, c.Channel.ch.State())
	return c.Channel
}

// Shutdown gracefully shuts down the client.
func (c *PaymentClient) Shutdown() {
	c.PerunClient.Close()
}
