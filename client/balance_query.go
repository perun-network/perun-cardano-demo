package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type WalletResponse struct {
	Balance B `json:"balance"`
}

type B struct {
	Available BalanceResult `json:"available"`
}

type BalanceResult struct {
	Amount int64  `json:"quantity"`
	Unit   string `json:"unit"`
}

func (w WalletResponse) Decode() int64 {
	return w.Balance.Available.Amount
}

func (c *PaymentClient) QueryBalance() (int64, error) {
	req, err := http.NewRequest("GET", c.WalletURL.String()+"/wallets/"+c.Account.GetCardanoWalletID(), nil)
	if err != nil {
		return 0, err
	}
	client := &http.Client{}
	response, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		errorBody, err := io.ReadAll(response.Body)
		if err != nil {
			return 0, fmt.Errorf("failed to interact with wallet server: %s", response.Status)
		}
		return 0, fmt.Errorf(
			"failed to interact with wallet server: %s with error: %s",
			response.Status,
			string(errorBody),
		)
	}
	jsonResponse, err := io.ReadAll(response.Body)
	if err != nil {
		return 0, err
	}
	var walletResponse WalletResponse
	if err := json.Unmarshal(jsonResponse, &walletResponse); err != nil {
		return 0, err
	}
	return walletResponse.Decode(), nil
}
