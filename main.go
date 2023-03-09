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
	ethwallet "perun.network/go-perun/backend/ethereum/wallet"
	"perun.network/go-perun/wire"
	"perun.network/perun-examples/payment-channel/client"
	"strconv"
)

const (
	chainURL = "ws://127.0.0.1:8545"
	chainID  = 1337

	// Private keys.
	keyDeployer = "79ea8f62d97bc0591a4224c1725fca6b00de5b2cea286fe2e0bb35c5e76be46e"
	keyAlice    = "1af2e950272dd403de7a5760d41c6e44d92b6d02797e51810795ff03cc2cda4f"
	keyBob      = "f63d7d8e930bccd74e93cf5662fde2c28fd8be95edb70c73f1bdd863d07f412e"

	PartySelectionPage = "PartySelectionPage"
	PartyMenuPage      = "PartyMenuPage"
	OpenChannelPage    = "OpenChannelPage"
	ViewChannelPage    = "ViewChannelPage"
)

func SetClientAndSwitchToPartyMenuPage(client *client.PaymentClient, view *View) func() {
	return func() {
		view.SetClient(client)
		view.Pages.SwitchToPage(PartyMenuPage)
	}
}

type View struct {
	id            uuid.UUID
	Client        *client.PaymentClient
	Pages         *tview.Pages
	onStateUpdate func(string)
}

func NewView() *View {
	return &View{
		id: uuid.New(),
	}
}

func (v *View) Update(s string) {
	if v.onStateUpdate != nil {
		v.onStateUpdate(s)
	}
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
	content.AddItem(tview.NewTextView().SetTextAlign(tview.AlignCenter).SetDynamicColors(true).SetText("[red]"+title), 1, 0, false)
	list := tview.NewList()

	for i, c := range PaymentClients {
		if i > 8 {
			break
		}

		list.AddItem(c.Name, "Addr: "+c.Account.String(), digitRunes[i+1], SetClientAndSwitchToPartyMenuPage(c, view))
	}

	content.AddItem(list, 0, 1, true)
	list.SetSelectedFocusOnly(true)
	return content
}

func newPartyMenuPage(view *View) tview.Primitive {
	content := tview.NewFlex().SetDirection(tview.FlexRow)
	title := tview.NewTextView().SetTextAlign(tview.AlignCenter).SetDynamicColors(true)
	content.AddItem(title, 2, 0, false)

	list := tview.NewList().SetSelectedFocusOnly(true)
	list.SetFocusFunc(func() {
		bal, _ := view.Client.GetLedgerBalance().Float64()
		balString := strconv.FormatFloat(bal, 'f', 4, 64)
		title.SetText("[red]" + view.Client.Name + "[white]: " + view.Client.Account.String() + "\nOn-Chain Balance: " + balString + " Eth" + "\n Menu")
	})
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
	title := tview.NewTextView().SetTextAlign(tview.AlignCenter).SetDynamicColors(true)
	content.AddItem(title, 2, 0, false)
	channelView := tview.NewTextView()
	sendForm := tview.NewForm()
	channelView.SetText("Currently no open channel for this client")
	content.AddItem(channelView, 0, 1, true)
	channelView.SetFocusFunc(func() {
		bal, _ := view.Client.GetLedgerBalance().Float64()
		balString := strconv.FormatFloat(bal, 'f', 4, 64)
		title.SetText("[red]" + view.Client.Name + "[white]: " + view.Client.Account.String() + "\nOn-Chain Balance: " + balString + " Eth" + "\n View Channel")
		App.SetFocus(sendForm)

	})
	content.SetFocusFunc(func() {
		bal, _ := view.Client.GetLedgerBalance().Float64()
		balString := strconv.FormatFloat(bal, 'f', 4, 64)
		title.SetText("[red]" + view.Client.Name + "[white]: " + view.Client.Account.String() + "\nOn-Chain Balance: " + balString + " Eth" + "\n View Channel")
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
		sendField := tview.NewInputField().SetLabel("Send Payment").SetFieldWidth(20).SetText("0")
		*sendForm = *tview.NewForm().AddFormItem(sendField).
			AddButton("Send", func() {
				amount, err := strconv.ParseFloat(sendField.GetText(), 64)
				if err != nil {
					return
				}
				go view.Client.Channel.SendPayment(amount)
			}).
			AddButton("Settle", func() {
				go view.Client.Channel.Settle()
			})
	}
	sendForm.SetFocusFunc(func() {
		bal, _ := view.Client.GetLedgerBalance().Float64()
		balString := strconv.FormatFloat(bal, 'f', 4, 64)
		title.SetText("[red]" + view.Client.Name + "[white]: " + view.Client.Account.String() + "\nOn-Chain Balance: " + balString + " Eth" + "\n View Channel")
		if view.Client.Channel != nil {
			setForm()
		}
	})

	return content, func(s string) {
		App.Draw()
		channelView.SetText(s)
		bal, _ := view.Client.GetLedgerBalance().Float64()
		balString := strconv.FormatFloat(bal, 'f', 4, 64)
		title.SetText("[red]" + view.Client.Name + "[white]: " + view.Client.Account.String() + "\nOn-Chain Balance: " + balString + " Eth" + "\n View Channel")
		if view.Client.Channel != nil {
			setForm()
		}
		App.Draw()
	}
}

func newOpenChannelPage(view *View) tview.Primitive {
	content := tview.NewFlex().SetDirection(tview.FlexRow)
	title := tview.NewTextView().SetTextAlign(tview.AlignCenter).SetDynamicColors(true)
	content.AddItem(title, 1, 0, false)
	form := tview.NewForm()
	content.AddItem(form, 0, 1, false)
	content.SetFocusFunc(func() {
		bal, _ := view.Client.GetLedgerBalance().Float64()
		balString := strconv.FormatFloat(bal, 'f', 4, 64)
		title.SetText("[red]" + view.Client.Name + "[white]: " + view.Client.Account.String() + "\nOn-Chain Balance: " + balString + " Eth" + "\n Open Channel")
		clientSelection := make(map[int]*client.PaymentClient)
		var clientNames []string
		i := 0
		for _, c := range PaymentClients {
			if c == view.Client {
				continue
			}
			clientSelection[i] = c
			str := fmt.Sprintf("%s (%s)", c.Name, c.Account.String())
			clientNames = append(clientNames, str)
			i++
		}
		peerField := tview.NewDropDown().SetLabel("Party").SetOptions(clientNames, nil).SetCurrentOption(0)
		depositField := tview.NewInputField().SetLabel("Deposit").SetFieldWidth(20).SetText("0")
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
		str := fmt.Sprintf("%s (%s)", c.Name, c.Account.String())
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
	log.Println("Deploying contracts.")
	adjudicator, assetHolder := deployContracts(chainURL, chainID, keyDeployer)
	asset := *ethwallet.AsWalletAddr(assetHolder)

	// Setup clients.
	log.Println("Setting up clients.")
	bus := wire.NewLocalBus() // Message bus used for off-chain communication.
	alice := setupPaymentClient("Alice", bus, chainURL, adjudicator, asset, keyAlice)
	bob := setupPaymentClient("Bob", bus, chainURL, adjudicator, asset, keyBob)
	PaymentClients = []*client.PaymentClient{alice, bob}

	App = tview.NewApplication()
	header := tview.NewBox().SetBorder(true).SetTitle("Header")

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
