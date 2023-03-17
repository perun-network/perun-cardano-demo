package client

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"

	"perun.network/go-perun/channel"
	"perun.network/go-perun/client"
)

// PaymentChannel is a wrapper for a Perun channel for the payment use case.
type PaymentChannel struct {
	ch       *client.Channel
	currency channel.Asset
}

func FormatState(c *PaymentChannel, state *channel.State) string {
	id := c.ch.ID()
	parties := c.ch.Params().Parts
	balA, _ := LovelaceToAda(state.Allocation.Balance(0, c.currency)).Float64()
	balAStr := strconv.FormatFloat(balA, 'f', 4, 64)

	balB, _ := LovelaceToAda(state.Allocation.Balance(1, c.currency)).Float64()
	balBStr := strconv.FormatFloat(balB, 'f', 4, 64)
	if len(parties) != 2 {
		panic("invalid parties length: " + strconv.Itoa(len(parties)))
	}
	ret := fmt.Sprintf(
		"Channel ID: [green]%s[white]\nBalances:\n    %s: [green]%s[white] Ada\n    %s: [green]%s[white] Ada\nFinal: [green]%t[white]\nVersion: [green]%d[white]",
		hex.EncodeToString(id[:]),
		parties[0].String(),
		balAStr,
		parties[1].String(),
		balBStr,
		state.IsFinal,
		state.Version,
	)
	return ret
}

// newPaymentChannel creates a new payment channel.
func newPaymentChannel(ch *client.Channel, currency channel.Asset) *PaymentChannel {
	return &PaymentChannel{
		ch:       ch,
		currency: currency,
	}
}

// SendPayment sends a payment to the channel peer.
func (c PaymentChannel) SendPayment(amount float64) {
	// Transfer the given amount from us to peer.
	// Use UpdateBy to update the channel state.
	err := c.ch.Update(context.TODO(), func(state *channel.State) { // We use context.TODO to keep the code simple.
		lovelaceAmount := AdaToLovelace(big.NewFloat(amount))
		actor := c.ch.Idx()
		peer := 1 - actor
		state.Allocation.TransferBalance(actor, peer, c.currency, lovelaceAmount)
	})
	if err != nil {
		panic(err)
	}
	if err != nil {
		panic(err) // We panic on error to keep the code simple.
	}
}

// Settle settles the payment channel and withdraws the funds.
func (c PaymentChannel) Settle() {
	// Finalize the channel to enable fast settlement.
	if !c.ch.State().IsFinal {
		err := c.ch.Update(context.TODO(), func(state *channel.State) {
			state.IsFinal = true
		})
		if err != nil {
			panic(err)
		}
	}

	// Settle concludes the channel and withdraws the funds.
	err := c.ch.Settle(context.TODO(), false)
	if err != nil {
		panic(err)
	}

	// Close frees up channel resources.
	c.ch.Close()
}
