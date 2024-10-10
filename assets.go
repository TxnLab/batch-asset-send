package main

import (
	"fmt"
	"math"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
)

type SendAsset struct {
	AssetID          uint64
	AssetParams      models.AssetParams
	ExistingBalance  uint64
	AmountToSend     float64
	IsAmountPerRecip bool
	Note             string
}

// write String method for SendAsset
func (a *SendAsset) String() string {
	return fmt.Sprintf("AssetID: %d (%s), ExistingBalance: %s, AmountToSend: %f, IsAmountPerRecip: %t, Note:%s",
		a.AssetID,
		a.AssetParams.UnitName,
		a.formattedAmount(a.ExistingBalance),
		a.AmountToSend,
		a.IsAmountPerRecip,
		a.Note)
}

func (s *SendAsset) formattedAmount(amount uint64) string {
	return fmt.Sprintf("%.*f", s.AssetParams.Decimals, float64(amount)/math.Pow10(int(s.AssetParams.Decimals)))
}

func (s *SendAsset) amountInBaseUnits(amount float64) uint64 {
	// ie: 1 (algo) becomes 1 million microAlgo.
	return uint64(amount * math.Pow10(int(s.AssetParams.Decimals)))
}
