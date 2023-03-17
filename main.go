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
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/google/uuid"
	"github.com/rivo/tview"
	"log"
	"os"
	gpchannel "perun.network/go-perun/channel"
	gpwallet "perun.network/go-perun/wallet"
	"perun.network/go-perun/wire"
	"perun.network/perun-cardano-backend/channel"
	"perun.network/perun-cardano-backend/wallet"
	"perun.network/perun-examples/payment-channel/client"
	"polycry.pt/poly-go/sync"
	"strconv"
	"time"
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

	PartySelectionPage = "PartySelectionPage"
	PartyMenuPage      = "PartyMenuPage"
	OpenChannelPage    = "OpenChannelPage"
	ViewChannelPage    = "ViewChannelPage"
)

func SetClientAndSwitchToPartyMenuPage(client *client.PaymentClient, view *View) func() {
	return func() {
		view.SetClient(client)
		log.Println("Switching to PartyMenuPage")
		view.Pages.SwitchToPage(PartyMenuPage)
	}
}

type View struct {
	id            uuid.UUID
	Client        *client.PaymentClient
	Pages         *tview.Pages
	title         *tview.TextView
	onStateUpdate func(string)
	updateLock    sync.Mutex
}

func NewView() *View {
	return &View{
		id:    uuid.New(),
		title: tview.NewTextView().SetTextAlign(tview.AlignCenter).SetDynamicColors(true).SetChangedFunc(func() { App.Draw() }),
	}
}

func (v *View) UpdateState(s string) {
	v.updateLock.Lock()
	defer v.updateLock.Unlock()
	if v.onStateUpdate != nil {
		v.onStateUpdate(s)
	}
}

func (v *View) UpdateBalance(s string) {
	v.updateLock.Lock()
	defer v.updateLock.Unlock()
	log.Printf("UpdateBalance of view: %s", v.id.String())
	v.title.SetText("[red]" + v.Client.Name + "[white]: " + v.Client.Account.Address().String() + "\nOn-Chain Balance: [green]" + s + "[white] Ada")

}

func (v *View) GetID() uuid.UUID {
	return v.id
}

func (v *View) SetClient(paymentClient *client.PaymentClient) {
	if v.Client == paymentClient {
		return
	}
	if v.Client != nil {
		v.Client.Deregister(v)
	}
	v.Client = paymentClient
	v.Client.Register(v)
}

var App *tview.Application
var PaymentClients []*client.PaymentClient

var Left = NewView()
var Right = NewView()

var digitRunes = []rune("0123456789")

func newPartiesPage(title string, view *View) tview.Primitive {
	content := tview.NewFlex().SetDirection(tview.FlexRow)
	content.AddItem(tview.NewTextView().SetTextAlign(tview.AlignCenter).SetDynamicColors(true).SetText("[red]"+title), 2, 0, false)
	list := tview.NewList()

	for i, c := range PaymentClients {
		if i > 8 {
			break
		}

		list.AddItem(c.Name, "Addr: "+c.Account.Address().String(), digitRunes[i+1], SetClientAndSwitchToPartyMenuPage(c, view))
	}

	content.AddItem(list, 0, 1, true)
	list.SetSelectedFocusOnly(true)
	return content
}

func newPartyMenuPage(view *View) tview.Primitive {
	content := tview.NewFlex().SetDirection(tview.FlexRow)
	content.AddItem(view.title, 2, 0, false)
	header := tview.NewTextView().SetTextAlign(tview.AlignCenter).SetText("Menu")
	content.AddItem(header, 2, 0, false)

	list := tview.NewList().SetSelectedFocusOnly(true)
	list.AddItem("Open Channel", "Open a new Channel with another party", 'o', func() {
		view.Pages.SwitchToPage(OpenChannelPage)
	})
	list.AddItem("View Channels", "View open channel", 'v', func() {
		view.Pages.SwitchToPage(ViewChannelPage)
	})
	content.AddItem(list, 0, 1, true)

	content.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'r':
			view.Pages.SwitchToPage(PartySelectionPage)
		}
		return event
	})
	return content
}

func newViewChannelPage(view *View) (tview.Primitive, func(string)) {
	content := tview.NewFlex().SetDirection(tview.FlexRow)
	content.AddItem(view.title, 2, 0, false)
	content.AddItem(tview.NewTextView().SetTextAlign(tview.AlignCenter).SetText("View Channel"), 2, 0, false)
	channelView := tview.NewTextView().SetDynamicColors(true).SetChangedFunc(func() { App.Draw() })
	sendForm := tview.NewForm()
	channelView.SetText("Currently no open channel for this client")
	content.AddItem(channelView, 0, 1, true)
	channelView.SetFocusFunc(func() {
		App.SetFocus(sendForm)

	})
	content.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'r':
			view.Pages.SwitchToPage(PartyMenuPage)
		}
		return event
	})
	content.AddItem(sendForm, 0, 1, false)
	setForm := func() {
		sendField := tview.NewInputField().SetLabel("Send Payment").SetFieldWidth(20).SetText("")
		microPaymentAmount := tview.NewInputField().SetLabel("Micro Payment Amount").SetFieldWidth(20).SetText("")
		microPaymentRepetitions := tview.NewInputField().SetLabel("Repetitions").SetFieldWidth(20).SetText("")
		*sendForm = *tview.NewForm().AddFormItem(sendField).
			AddButton("Send", func() {
				amount, err := strconv.ParseFloat(sendField.GetText(), 64)
				if err != nil {
					return
				}
				go view.Client.Channel.SendPayment(amount)
			}).
			AddFormItem(microPaymentAmount).AddFormItem(microPaymentRepetitions).
			AddButton("Send Micro Payment", func() {
				amount, err := strconv.ParseFloat(microPaymentAmount.GetText(), 64)
				if err != nil {
					return
				}
				repetitions, err := strconv.ParseInt(microPaymentRepetitions.GetText(), 10, 64)
				if err != nil {
					return
				}
				go func() {
					for i := int64(0); i < repetitions; i++ {
						view.Client.Channel.SendPayment(amount)
						time.Sleep(50 * time.Millisecond)
					}
				}()
			}).
			AddButton("Settle", func() {
				go view.Client.Channel.Settle()
			})
	}
	sendForm.SetFocusFunc(func() {
		if view.Client.Channel != nil {
			setForm()
		}
	})

	return content, func(s string) {
		channelView.SetText(s)
		if view.Client.Channel != nil {
			setForm()
		}
	}
}

func newOpenChannelPage(view *View) tview.Primitive {
	content := tview.NewFlex().SetDirection(tview.FlexRow)
	content.AddItem(view.title, 2, 0, false)
	content.AddItem(tview.NewTextView().SetTextAlign(tview.AlignCenter).SetText("Open Channel"), 2, 0, false)
	form := tview.NewForm()
	content.AddItem(form, 0, 1, false)
	content.SetFocusFunc(func() {
		clientSelection := make(map[int]*client.PaymentClient)
		var clientNames []string
		i := 0
		for _, c := range PaymentClients {
			if c == view.Client {
				continue
			}
			clientSelection[i] = c
			str := fmt.Sprintf("%s (%s)", c.Name, c.Account.Address().String())
			clientNames = append(clientNames, str)
			i++
		}
		peerField := tview.NewDropDown().SetLabel("Party").SetOptions(clientNames, nil).SetCurrentOption(0)
		depositField := tview.NewInputField().SetLabel("Deposit").SetFieldWidth(20).SetText("")
		*form = *tview.NewForm().AddFormItem(peerField).AddFormItem(depositField).
			AddButton("Open Channel", func() {
				deposit, err := strconv.ParseFloat(depositField.GetText(), 64)
				if err != nil {
					return
				}
				peerIndex, _ := peerField.GetCurrentOption()
				peer := clientSelection[peerIndex]
				log.Println("open channel called next")
				go view.Client.OpenChannel(peer.WireAddress(), deposit)
				view.Pages.SwitchToPage(ViewChannelPage)
			}).
			AddButton("Cancel", func() {
				view.Pages.SwitchToPage(PartyMenuPage)
			})
		App.SetFocus(form)
	})
	content.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'r':
			view.Pages.SwitchToPage(PartyMenuPage)
		}
		return event
	})
	var clientNames []string
	for _, c := range PaymentClients {
		if c == view.Client {
			continue
		}
		str := fmt.Sprintf("%s (%s)", c.Name, c.Account.Address().String())
		clientNames = append(clientNames, str)
	}
	return content
}

func SetLogFile(path string) {
	logFile, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	log.SetOutput(logFile)
}

// main runs a demo of the payment client. It assumes that a blockchain node is
// available at `chainURL` and that the accounts corresponding to the specified
// secret keys are provided with sufficient funds.
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
	PaymentClients = []*client.PaymentClient{alice, bob}

	App = tview.NewApplication()
	header := tview.NewBox().SetBorder(true).SetTitle(" Perun Payment Channel Demo ")

	right := tview.NewPages()
	right.SetFocusFunc(func() {
		_, frontPage := right.GetFrontPage()
		App.SetFocus(frontPage)
	})
	left := tview.NewPages()
	left.SetFocusFunc(func() {
		_, frontPage := left.GetFrontPage()
		App.SetFocus(frontPage)
	})
	App.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyCtrlA:
			App.SetFocus(left)
		case tcell.KeyCtrlB:
			App.SetFocus(right)
		default:
			switch event.Rune() {
			case 'q':
				App.Stop()
			}
		}
		return event
	})
	Left.Pages = left
	Right.Pages = right

	left.AddPage(PartySelectionPage, newPartiesPage("Party A", Left), true, true)
	right.AddPage(PartySelectionPage, newPartiesPage("Party B", Right), true, true)

	left.AddPage(PartyMenuPage, newPartyMenuPage(Left), true, false)
	right.AddPage(PartyMenuPage, newPartyMenuPage(Right), true, false)

	leftViewChannelPage, leftUpdateHandler := newViewChannelPage(Left)
	Left.onStateUpdate = leftUpdateHandler
	left.AddPage(ViewChannelPage, leftViewChannelPage, true, false)

	rightViewChannelPage, rightUpdateHandler := newViewChannelPage(Right)
	Right.onStateUpdate = rightUpdateHandler
	right.AddPage(ViewChannelPage, rightViewChannelPage, true, false)

	left.AddPage(OpenChannelPage, newOpenChannelPage(Left), true, false)
	right.AddPage(OpenChannelPage, newOpenChannelPage(Right), true, false)

	left.SwitchToPage(PartySelectionPage)
	right.SwitchToPage(PartySelectionPage)

	flex := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(header, 3, 0, false).
		AddItem(
			tview.NewFlex().SetDirection(tview.FlexColumn).
				AddItem(left, 0, 1, false).
				AddItem(tview.NewBox().SetBorder(true), 2, 0, false).
				AddItem(right, 0, 1, false),
			0,
			1,
			false,
		)

	if err := App.SetRoot(flex, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}

	/*
		newPrimitive := func(text string) tview.Primitive {
			return tview.NewTextView().
				SetTextAlign(tview.AlignCenter).
				SetText(text)
		}
		left := newPrimitive("Left")
		right := newPrimitive("Right")

		grid := tview.NewGrid().
			SetRows(3, 0, 3).
			SetColumns(0, 0).
			SetBorders(true).
			AddItem(newPrimitive("Header"), 0, 0, 1, 2, 0, 0, false).
			AddItem(newPrimitive("Footer"), 2, 0, 1, 2, 0, 0, false)

		// Layout for screens narrower than 100 cells (menu and side bar are hidden).
		/*grid.AddItem(menu, 0, 0, 0, 0, 0, 0, false).
		AddItem(main, 1, 0, 1, 3, 0, 0, false).
		AddItem(sideBar, 0, 0, 0, 0, 0, 0, false)

		// Layout for screens wider than 100 cells.
		grid.AddItem(left, 1, 0, 1, 1, 0, 100, false).
			AddItem(right, 1, 1, 1, 1, 0, 100, false)

		if err := tview.NewApplication().SetRoot(grid, true).EnableMouse(true).Run(); err != nil {
			panic(err)
		}
	*/
}

/*
func startDemo() {
	// Deploy contracts.
	log.Println("Deploying contracts.")
	adjudicator, assetHolder := deployContracts(chainURL, chainID, keyDeployer)
	asset := *ethwallet.AsWalletAddr(assetHolder)

	// Setup clients.
	log.Println("Setting up clients.")
	bus := wire.NewLocalBus() // Message bus used for off-chain communication.
	alice := setupPaymentClient("Alice", bus, chainURL, adjudicator, asset, keyAlice)
	bob := setupPaymentClient("Bob", bus, chainURL, adjudicator, asset, keyBob)

	// Print balances before transactions.
	l := newBalanceLogger(chainURL)
	l.LogBalances(alice.WalletAddress(), bob.WalletAddress())

	// Open channel, transact, close.
	log.Println("Opening channel and depositing funds.")
	chAlice := alice.OpenChannel(bob.WireAddress(), 5)
	chBob := bob.AcceptedChannel()

	log.Println("Sending payments...")
	fmt.Println(chAlice.String())
	chAlice.SendPayment(3)
	fmt.Println(chAlice.String())
	chBob.SendPayment(1)
	fmt.Println(chAlice.String())
	chAlice.SendPayment(1)
	fmt.Println(chAlice.String())

	log.Println("Settling channel.")
	chAlice.Settle() // Conclude and withdraw.
	chBob.Settle()   // Withdraw.

	// Print balances after transactions.
	l.LogBalances(alice.WalletAddress(), bob.WalletAddress())

	// Cleanup.
	alice.Shutdown()
	bob.Shutdown()
}
*/
