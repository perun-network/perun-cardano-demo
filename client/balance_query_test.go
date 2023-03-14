package client_test

import (
	"encoding/hex"
	"github.com/stretchr/testify/require"
	"perun.network/go-perun/wire"
	"perun.network/perun-cardano-backend/channel"
	wallet2 "perun.network/perun-cardano-backend/wallet"
	"perun.network/perun-cardano-backend/wallet/address"
	"perun.network/perun-cardano-backend/wallet/test"
	"perun.network/perun-examples/payment-channel/client"
	pkgtest "polycry.pt/poly-go/test"
	"testing"
)

const (
	pubKeyAlice         = "5a3aeed83ffe0e41408a41de4cf9e1f1e39416643ea21231a2d00be46f5446a9"
	walletIDAlice       = "c35896086738b89c00f3ff41f2beced7449fc6e6"
	cardanoWalletServer = "http://localhost:8090/v2"
)

// The wallet server must be running for this test!
func TestBalanceQuery(t *testing.T) {
	rng := pkgtest.Prng(t)
	addrBytes, err := hex.DecodeString(pubKeyAlice)
	require.NoError(t, err)
	addr, err := address.MakeAddressFromByteSlice(addrBytes)
	require.NoError(t, err)
	r := test.NewGenericRemote([]address.Address{addr}, rng)
	w := wallet2.NewRemoteWallet(r, walletIDAlice)
	acc, err := w.Unlock(&addr)
	require.NoError(t, err)
	bus := wire.NewLocalBus()
	c, err := client.SetupPaymentClient("Alice", bus, acc.(wallet2.RemoteAccount), "", w, channel.Asset, cardanoWalletServer)
	require.NoError(t, err)
	b, err := c.QueryBalance()
	require.NoError(t, err)
	require.Equal(t, int64(420133769), b)
}
