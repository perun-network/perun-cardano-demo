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
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"log"
	"math/big"
	ethchannel "perun.network/go-perun/backend/ethereum/channel"
	ethwallet "perun.network/go-perun/backend/ethereum/wallet"
	swallet "perun.network/go-perun/backend/ethereum/wallet/simple"
	"perun.network/go-perun/channel"
	"perun.network/go-perun/client"
	"perun.network/go-perun/wallet"
	"perun.network/go-perun/watcher/local"
	"perun.network/go-perun/wire"
	"polycry.pt/poly-go/sync"
)

const (
	txFinalityDepth = 1 // Number of blocks required to confirm a transaction.
)

type Observer interface {
	Update(string)
	GetID() uuid.UUID
}

type Subject interface {
	Register(observer Observer)
	Deregister(observer Observer)
	notifyAll(from, to *channel.State)
}

// PaymentClient is a payment channel client.
type PaymentClient struct {
	mutex       sync.Mutex
	ec          *ethclient.Client
	Name        string
	PerunClient *client.Client // The core Perun client.
	Channel     *PaymentChannel
	Account     wallet.Address       // The Account we use for on-chain and off-chain transactions.
	currency    channel.Asset        // The currency we expect to get paid in.
	channels    chan *PaymentChannel // Accepted payment channels.
	onUpdate    func(from, to *channel.State)
	observers   []Observer
}

func (c *PaymentClient) Register(observer Observer) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.observers = append(c.observers, observer)
}

func (c *PaymentClient) Deregister(observer Observer) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for i, o := range c.observers {
		if o.GetID().String() == observer.GetID().String() {
			c.observers[i] = c.observers[len(c.observers)-1]
			c.observers = c.observers[:len(c.observers)-1]
		}

	}
}

func newEthClient(chainURL string) (*ethclient.Client, error) {
	c, err := ethclient.Dial(chainURL)
	return c, err
}

func (c *PaymentClient) notifyAll(from, to *channel.State) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	str := FormatState(c.Channel, to)
	for _, o := range c.observers {
		o.Update(str)
	}
}

// SetupPaymentClient creates a new payment client.
func SetupPaymentClient(
	name string,
	bus wire.Bus, // bus is used of off-chain communication.
	w *swallet.Wallet, // w is the wallet used for signing transactions.
	acc common.Address, // acc is the address of the Account to be used for signing transactions.
	nodeURL string, // nodeURL is the URL of the blockchain node.
	chainID uint64, // chainID is the identifier of the blockchain.
	adjudicator common.Address, // adjudicator is the address of the adjudicator.
	asset ethwallet.Address, // asset is the address of the asset holder for our payment channels.
) (*PaymentClient, error) {
	ec, err := newEthClient(nodeURL)
	if err != nil {
		return nil, err
	}

	// Create Ethereum client and contract backend.
	cb, err := CreateContractBackend(nodeURL, chainID, w)
	if err != nil {
		return nil, fmt.Errorf("creating contract backend: %w", err)
	}

	// Validate contracts.
	err = ethchannel.ValidateAdjudicator(context.TODO(), cb, adjudicator)
	if err != nil {
		return nil, fmt.Errorf("validating adjudicator: %w", err)
	}
	err = ethchannel.ValidateAssetHolderETH(context.TODO(), cb, common.Address(asset), adjudicator)
	if err != nil {
		return nil, fmt.Errorf("validating adjudicator: %w", err)
	}

	// Setup funder.
	funder := ethchannel.NewFunder(cb)
	dep := ethchannel.NewETHDepositor()
	ethAcc := accounts.Account{Address: acc}
	funder.RegisterAsset(asset, dep, ethAcc)

	// Setup adjudicator.
	adj := ethchannel.NewAdjudicator(cb, adjudicator, acc, ethAcc)

	// Setup dispute watcher.
	watcher, err := local.NewWatcher(adj)
	if err != nil {
		return nil, fmt.Errorf("intializing watcher: %w", err)
	}

	// Setup Perun client.
	waddr := ethwallet.AsWalletAddr(acc)
	perunClient, err := client.New(waddr, bus, funder, adj, w, watcher)
	if err != nil {
		return nil, errors.WithMessage(err, "creating client")
	}

	// Create client and start request handler.
	c := &PaymentClient{
		ec:          ec,
		Name:        name,
		PerunClient: perunClient,
		Account:     waddr,
		currency:    &asset,
		channels:    make(chan *PaymentChannel, 1),
	}
	go perunClient.Handle(c, c)

	return c, nil
}

// OpenChannel opens a new channel with the specified peer and funding.
func (c *PaymentClient) OpenChannel(peer wire.Address, amount float64) *PaymentChannel {
	// We define the channel participants. The proposer has always index 0. Here
	// we use the on-chain addresses as off-chain addresses, but we could also
	// use different ones.
	log.Println("OpenChannel called")
	participants := []wire.Address{c.Account, peer}

	// We create an initial allocation which defines the starting balances.
	initAlloc := channel.NewAllocation(2, c.currency)
	initAlloc.SetAssetBalances(c.currency, []channel.Bal{
		EthToWei(big.NewFloat(amount)), // Our initial balance.
		big.NewInt(0),                  // Peer's initial balance.
	})
	log.Println("Created Allocation")

	// Prepare the channel proposal by defining the channel parameters.
	challengeDuration := uint64(10) // On-chain challenge duration in seconds.
	proposal, err := client.NewLedgerChannelProposal(
		challengeDuration,
		c.Account,
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
	c.Channel.ch.OnUpdate(c.notifyAll)
	c.notifyAll(nil, ch.State())
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
	c.Channel.ch.OnUpdate(c.notifyAll)
	c.notifyAll(nil, c.Channel.ch.State())
	return c.Channel
}

// Shutdown gracefully shuts down the client.
func (c *PaymentClient) Shutdown() {
	c.PerunClient.Close()
}

func (c *PaymentClient) GetLedgerBalance() *big.Float {
	bal, _ := c.ec.BalanceAt(context.TODO(), c.WalletAddress(), nil)
	return WeiToEth(bal)
}
