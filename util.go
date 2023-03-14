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

package main

import (
	"encoding/hex"
	"log"
	"perun.network/go-perun/wire"
	"perun.network/perun-cardano-backend/channel"
	"perun.network/perun-cardano-backend/wallet"
	"perun.network/perun-cardano-backend/wallet/address"
	"perun.network/perun-examples/payment-channel/client"
)

// setupPaymentClient sets up a new client with the given parameters.
func setupPaymentClient(
	name string,
	bus wire.Bus,
	pabHost string,
	pubKey string,
	walletId string,
	r wallet.Remote,
	cardanoWalletServerURL string,
) *client.PaymentClient {
	pubKeyBytes, _ := hex.DecodeString(pubKey)
	addr, _ := address.MakeAddressFromByteSlice(pubKeyBytes)

	w := wallet.NewRemoteWallet(r, walletId)
	acc, err := w.Unlock(&addr)
	if err != nil {
		log.Fatalf("error unlocking alice's account: %v", err)
	}

	c, err := client.SetupPaymentClient(name, bus, acc.(wallet.RemoteAccount), pabHost, w, channel.Asset, cardanoWalletServerURL)
	if err != nil {
		panic(err)
	}

	return c
}
