package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/transaction"
	"github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/antihax/optional"
	"github.com/ssgreg/repeat"

	"github.com/TxnLab/batch-asset-send/lib/algo"
	"github.com/TxnLab/batch-asset-send/lib/misc"
	nfdapi "github.com/TxnLab/batch-asset-send/lib/nfdapi/swagger"
)

// IsContractVersionAtLeast returns true if the specified version string (ie: 1.13a) is at least the specified
// major.minor version.  ie: "2.0" and major.minor of 1.3 would be true - because 2.0 > 1.3
func IsContractVersionAtLeast(version string, major, minor int) bool {
	majMinReg := regexp.MustCompile(`^(?P<major>\d+)\.(?P<minor>\d+)`)
	matches := majMinReg.FindStringSubmatch(version)
	if matches == nil || len(matches) != 3 {
		return false
	}
	var contractMajor, contractMinor int
	if val := matches[majMinReg.SubexpIndex("major")]; val != "" {
		contractMajor, _ = strconv.Atoi(val)
	}
	if val := matches[majMinReg.SubexpIndex("minor")]; val != "" {
		contractMinor, _ = strconv.Atoi(val)
	}
	if contractMajor > major || (contractMajor >= major && contractMinor >= minor) {
		return true
	}
	return false
}

func retryNfdApiCalls(meth func() error) error {
	return repeat.Repeat(
		repeat.Fn(func() error {
			err := meth()
			if err != nil {
				if rate, match := isRateLimited(err); match {
					logger.Warn("rate limited", "waiting", rate.SecsRemaining)
					time.Sleep(time.Duration(rate.SecsRemaining+1) * time.Second)
					return repeat.HintTemporary(err)
				}
				var swaggerError nfdapi.GenericSwaggerError
				if errors.As(err, &swaggerError) {
					if moderr, match := swaggerError.Model().(nfdapi.ModelError); match {
						return fmt.Errorf("message:%s, err:%w", moderr.Message, err)
					}
				}
			}
			return err
		}),
		repeat.StopOnSuccess(),
	)
}

func getAllNfds(onlyRoots bool, requireVaults bool) ([]*nfdapi.NfdRecord, error) {
	var (
		offset, limit int32 = 0, 200
		records       nfdapi.NfdV2SearchRecords
		err           error
		nfds          []*nfdapi.NfdRecord
	)

	for ; ; offset += limit {
		err = retryNfdApiCalls(func() error {
			searchOpts := &nfdapi.NfdApiNfdSearchV2Opts{
				State:  optional.NewInterface("owned"),
				View:   optional.NewString("brief"),
				Limit:  optional.NewInt32(limit),
				Offset: optional.NewInt32(offset),
			}
			if onlyRoots {
				searchOpts.Traits = optional.NewInterface("pristine")
			}
			records, _, err = api.NfdApi.NfdSearchV2(ctx, searchOpts)
			return err
		})

		if err != nil {
			return nil, fmt.Errorf("error while fetching segments: %w", err)
		}

		if records.Nfds == nil || len(*records.Nfds) == 0 {
			break
		}
		for _, record := range *records.Nfds {
			if record.DepositAccount == "" {
				continue
			}
			if requireVaults {
				// contract has to be at least 2.11 and not be locked for vault receipt
				if !IsContractVersionAtLeast(record.Properties.Internal["ver"], 2, 11) || record.Properties.Internal["vaultOptInLocked"] == "1" {
					continue
				}
			}

			newRecord := record
			nfds = append(nfds, &newRecord)
		}
	}
	return nfds, nil
}

func getSegmentsOfRoot(rootNfdName string, requireVaults bool) ([]*nfdapi.NfdRecord, error) {
	// Fetch root NFD - all we really want is its app id
	nfd, _, err := api.NfdApi.NfdGetNFD(ctx, rootNfdName, nil)
	if err != nil {
		log.Fatalln(err)
	}
	misc.Infof(logger, fmt.Sprintf("nfd app id for %s is:%v", nfd.Name, nfd.AppID))

	nfds, err := getAllSegments(ctx, nfd.AppID, requireVaults)
	if err != nil {
		log.Fatalln(err)
	}
	logger.Debug(fmt.Sprintf("fetched segments of root:%s, count:%d", rootNfdName, len(nfds)))
	return nfds, nil
}

func getAllSegments(ctx context.Context, parentAppID int64, requireVaults bool) ([]*nfdapi.NfdRecord, error) {
	var (
		offset, limit int32 = 0, 200
		records       nfdapi.NfdV2SearchRecords
		err           error
		nfds          []*nfdapi.NfdRecord
	)

	for ; ; offset += limit {
		err = retryNfdApiCalls(func() error {
			records, _, err = api.NfdApi.NfdSearchV2(ctx, &nfdapi.NfdApiNfdSearchV2Opts{
				ParentAppID: optional.NewInt64(parentAppID),
				State:       optional.NewInterface("owned"),
				Limit:       optional.NewInt32(limit),
				Offset:      optional.NewInt32(offset),
			})
			return err
		})

		if err != nil {
			return nil, fmt.Errorf("error while fetching segments: %w", err)
		}

		if records.Nfds == nil || len(*records.Nfds) == 0 {
			break
		}
		for _, record := range *records.Nfds {
			if record.DepositAccount == "" {
				continue
			}
			if requireVaults {
				// contract has to be at least 2.11 and not be locked for vault receipt
				if !IsContractVersionAtLeast(record.Properties.Internal["ver"], 2, 11) || record.Properties.Internal["vaultOptInLocked"] == "1" {
					continue
				}
			}
			newRecord := record
			nfds = append(nfds, &newRecord)
		}
	}
	return nfds, nil
}

func getAssetSendTxns(sender string, sendFromVaultName string, recipient string, recipientIsVault bool, assetID uint64, amount uint64, params types.SuggestedParams) (string, []byte, error) {
	var (
		encodedTxns string
		err         error
	)

	if sendFromVaultName == "" && recipientIsVault == false {
		// Not sending from vault, nor sending to a vault - so just plain asset transfer
		txn, err := transaction.MakeAssetTransferTxn(sender, recipient, amount, nil, params, "", assetID)
		if err != nil {
			return "", nil, fmt.Errorf("MakeAssetTransferTxn fail: %w", err)
		}
		txnid, signedBytes, err := signer.SignWithAccount(ctx, txn, sender)
		return txnid, signedBytes, err
	}

	err = retryNfdApiCalls(func() error {
		if sendFromVaultName != "" {
			receiverType := "account"
			if recipientIsVault {
				receiverType = "nfdVault"
			}
			encodedTxns, _, err = api.NfdApi.NfdSendFromVault(
				ctx,
				nfdapi.SendFromVaultRequestBody{
					Amount:       amount,
					Assets:       []uint64{assetID},
					Receiver:     recipient,
					ReceiverType: receiverType,
					Sender:       sender, // owner address
				},
				sendFromVaultName,
			)
		} else {
			if recipientIsVault {
				encodedTxns, _, err = api.NfdApi.NfdSendToVault(
					ctx,
					nfdapi.SendToVaultRequestBody{
						Amount: amount,
						Assets: []uint64{assetID},
						Sender: sender, // owner address
					},
					recipient,
				)
			} else {
				panic("never should have arrived here - pre-checks should have handled")
			}
		}
		return err
	})

	if err != nil {
		return "", nil, fmt.Errorf("error in NfdSendToVault call: %w", err)
	}
	return algo.DecodeAndSignNFDTransactions(encodedTxns, signer)
}

func isRateLimited(err error) (*nfdapi.RateLimited, bool) {
	if swaggerError, match := isSwaggerError(err); match {
		if limit, match := swaggerError.Model().(nfdapi.RateLimited); match {
			return &limit, true
		}
	}
	return nil, false
}

func isSwaggerError(err error) (*nfdapi.GenericSwaggerError, bool) {
	var swaggerError nfdapi.GenericSwaggerError
	if errors.As(err, &swaggerError) {
		return &swaggerError, true
	}
	return nil, false
}
