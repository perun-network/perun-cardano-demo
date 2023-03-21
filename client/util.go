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
	"math/big"
)

// AdaToLovelace converts a given amount in Ada to Lovelace.
func AdaToLovelace(adaAmount *big.Float) (lovelaceAmount *big.Int) {
	lovelacePerAda := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
	lovelacePerAdaFloat := new(big.Float).SetInt(lovelacePerAda)
	lovelaceAmountFloat := new(big.Float).Mul(adaAmount, lovelacePerAdaFloat)
	lovelaceAmount, _ = lovelaceAmountFloat.Int(nil)
	return lovelaceAmount
}

// LovelaceToAda converts a given amount in Lovelace to Ada.
func LovelaceToAda(lovelaceAmount *big.Int) (adaAmount *big.Float) {
	lovelacePerAda := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
	lovelacePerAdaFloat := new(big.Float).SetInt(lovelacePerAda)
	lovelaceAmountFloat := new(big.Float).SetInt(lovelaceAmount)
	return new(big.Float).Quo(lovelaceAmountFloat, lovelacePerAdaFloat)
}
