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
	"log"
	"os"
	gpchannel "perun.network/go-perun/channel"
	gpwallet "perun.network/go-perun/wallet"
	"perun.network/go-perun/wire"
	"perun.network/perun-cardano-backend/channel"
	"perun.network/perun-cardano-backend/wallet"
	vc "perun.network/perun-demo-tui/client"
	"perun.network/perun-demo-tui/view"
)

const (
	pabHost                = "localhost:9080"
	cardanoWalletServerURL = "http://localhost:8090/v2"

	pubKeyAlice            = "5a3aeed83ffe0e41408a41de4cf9e1f1e39416643ea21231a2d00be46f5446a9"
	pubKeyBob              = "04960fbc5fe4f1ae939fdfed8a13569384474db2a38ce7b65b328d1cd578fded"
	alicePaymentIdentifier = "9706069d2e482d1612cdf062d0d2f9bb3db01ab074f7c3eeb741bcd4"
	bobPaymentIdentifier   = "b50a436ae002343d30c9ddd48608a13e0e38b6785a47121c80cf45ff"

	walletIDAlice = "c35896086738b89c00f3ff41f2beced7449fc6e6"
	walletIDBob   = "34dd5c2bc7ec25850765242b83a31053ac3d3fb5"
)

func SetLogFile(path string) {
	logFile, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	log.SetOutput(logFile)
}

func main() {
	SetLogFile("payment-client.log")
	r := wallet.NewPerunCardanoWallet("http://localhost:8888")
	wb := wallet.MakeRemoteBackend(r)

	gpwallet.SetBackend(wb)
	channel.SetWalletBackend(wb)
	gpchannel.SetBackend(channel.Backend)

	// Setup clients.
	log.Println("Setting up clients.")
	bus := wire.NewLocalBus() // Message bus used for off-chain communication.
	alice := setupPaymentClient("Alice", bus, pabHost, pubKeyAlice, alicePaymentIdentifier, walletIDAlice, r, cardanoWalletServerURL)
	bob := setupPaymentClient("Bob", bus, pabHost, pubKeyBob, bobPaymentIdentifier, walletIDBob, r, cardanoWalletServerURL)
	clients := []vc.DemoClient{alice, bob}
	_ = view.RunDemo("Cardano Payment Channel Demo", clients)
}
