package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
	"github.com/algorand/go-algorand-sdk/v2/transaction"
	"github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/mailgun/holster/v4/syncutil"
	"github.com/ssgreg/repeat"

	"github.com/TxnLab/batch-asset-send/lib/algo"
	"github.com/TxnLab/batch-asset-send/lib/misc"
	nfdapi "github.com/TxnLab/batch-asset-send/lib/nfdapi/swagger"
)

// RecipientTransaction is for tracking what was sent or what was meant to be sent to each recipient
type RecipientTransaction struct {
	sendAsset       *SendAsset
	baseUnitsToSend uint64
	recip           *Recipient
	// Either has an error on its send or success
	Error   error
	Success struct {
		round uint64
		txid  string
	}
}

func (rt *RecipientTransaction) String() string {
	if rt == nil {
		return "{nil}"
	}
	var retStr strings.Builder

	if rt.recip.SendToVault {
		retStr.WriteString(fmt.Sprintf("Recipient: %s VAULT, ", rt.recip.NfdName))
	} else {
		retStr.WriteString(fmt.Sprintf("Recipient: %s (DEPOSIT), ", rt.recip.NfdName))
	}
	retStr.WriteString(fmt.Sprintf("Asset ID: %d, Amount: %s, ",
		rt.sendAsset.AssetID,
		rt.sendAsset.formattedAmount(rt.baseUnitsToSend)))
	if rt.Error != nil {
		retStr.WriteString(fmt.Sprintf("Error: %v", rt.Error))
	}
	if rt.Success.round != 0 {
		retStr.WriteString(fmt.Sprintf("Success: Round %d, TxID %s", rt.Success.round, rt.Success.txid))
	}
	return retStr.String()
}

type SendRequest struct {
	sender           string
	params           types.SuggestedParams
	asset            SendAsset
	amount           uint64
	recipient        Recipient
	sendFromVaultNFD *nfdapi.NfdRecord
	note             string
}

func sendAssets(sender string, send []*SendAsset, recipients []*Recipient, vaultNfd *nfdapi.NfdRecord, dryRun bool) {
	var (
		sendRequests = make(chan SendRequest, maxSimultaneousSends)
		sendResults  = make(chan *RecipientTransaction, maxSimultaneousSends)
		fanOut       = syncutil.NewFanOut(maxSimultaneousSends)
		wg           sync.WaitGroup
		successes    int
		failures     int
		startTime    = time.Now()
	)
	// ensure file appending is possible
	appendToFile("Starting", "failure.txt")
	appendToFile("Starting", "success.txt")

	// Queues to sendRequests then closes the channel once done
	go QueueSends(sendRequests, send, sender, recipients, vaultNfd)

	// Handle parallel results that will soon be coming from the parallel sends - exiting once handled all sends...
	wg.Add(1)
	go func() {
		defer wg.Done()
		for result := range sendResults {
			misc.Infof(logger, "Send result:%s", result.String())
			// save off to separate files - success, failure - opening/closing each to allow for clean
			// exit
			if result.Error != nil {
				appendToFile(result.String(), "failure.txt")
				failures++
			} else {
				appendToFile(result.String(), "success.txt")
				successes++
			}
		}
	}()

	// Now handle all the send requests (in parallel fanout)
	for send := range sendRequests {
		fanOut.Run(func(val any) error {
			sendReq := val.(SendRequest)
			misc.Infof(logger, "  %s: %s", sendReq.recipient.DepositAccount, sendReq.recipient.NfdName)
			sendResults <- sendAssetToRecipient(sender, &sendReq, dryRun)
			return nil
		}, send)
	}
	fanOut.Wait()      // returns once all results are queued..
	close(sendResults) // we've queued all results at this point
	wg.Wait()          // now wait to have processed them all.

	if failures > 0 {
		misc.Infof(logger, "%d successful sends", successes)
		misc.Infof(logger, "%d FAILED sends - check failure.txt for issues", failures)
	} else {
		misc.Infof(logger, "All %d sends successful", successes)
	}
	misc.Infof(logger, "Elapsed time:%v", time.Since(startTime))
}

func appendToFile(message string, filename string) {
	// open file in append mode
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed opening file: %s", err))
		os.Exit(1)
	}
	defer file.Close()

	_, err = fmt.Fprintln(file, message)
	if err != nil {
		// ouch ??
		logger.Error(fmt.Sprintf("Failed writing to the file: %s", err))
		os.Exit(1)
	}
}

func QueueSends(sendRequests chan SendRequest, sendAsset []*SendAsset, sender string, recipients []*Recipient, sendFromVaultNFD *nfdapi.NfdRecord) {
	var (
		// Get new params every 30 secs or so
		txParams = algo.SuggestedParams(ctx, logger, algoClient)
		ticker   = time.NewTicker(30 * time.Second)
	)
	for _, asset := range sendAsset {
		var amount float64
		if asset.IsAmountPerRecip {
			amount = asset.AmountToSend
		} else {
			amount = asset.AmountToSend / float64(len(recipients))
		}
		baseUnitAmount := asset.amountInBaseUnits(amount)
		misc.Infof(logger, "Sending %s of asset %d to %d recipients", asset.formattedAmount(baseUnitAmount), asset.AssetID, len(recipients))

		for _, recipient := range recipients {
			select {
			case <-ticker.C:
				txParams = algo.SuggestedParams(ctx, logger, algoClient)
			default:
			}
			// just queue the request to send
			sendRequests <- SendRequest{
				sender:           sender,
				params:           txParams,
				asset:            *asset,
				amount:           baseUnitAmount,
				recipient:        *recipient,
				sendFromVaultNFD: sendFromVaultNFD,
			}
		}
	}
	close(sendRequests)
}

func sendAssetToRecipient(sender string, sendReq *SendRequest, dryRun bool) *RecipientTransaction {
	var sendFromVaultName string

	retReceipt := &RecipientTransaction{
		sendAsset:       &sendReq.asset,
		baseUnitsToSend: sendReq.amount,
		recip:           &sendReq.recipient,
	}

	// First, is this a send FROM a vault or from an account?
	if sendReq.sendFromVaultNFD != nil {
		sendFromVaultName = sendReq.sendFromVaultNFD.Name
	}

	// Call NFD api to do the work for us (prob get rate limited - but handle that as well)
	recipAsString := sendReq.recipient.DepositAccount
	if sendReq.recipient.SendToVault {
		recipAsString = sendReq.recipient.NfdName
	}
	if dryRun {
		senderStr := sender
		if sendFromVaultName != "" {
			senderStr = sendFromVaultName + " vault"
		}
		misc.Infof(logger, "DryRun: Would send %s of %s from %s to %s", sendReq.asset.formattedAmount(sendReq.amount), sendReq.asset.AssetParams.UnitName, senderStr, recipAsString)
		return retReceipt
	}

	txnId, signedBytes, err := getAssetSendTxns(
		sender,
		sendFromVaultName,
		recipAsString,
		sendReq.recipient.SendToVault,
		sendReq.asset.AssetID,
		sendReq.amount,
		sendReq.asset.Note,
		sendReq.params,
	)
	if err != nil {
		retReceipt.Error = fmt.Errorf("failure getting txns: %w", err)
		return retReceipt
	}

	pendResponse, err := sendAndWaitTxns(signedBytes, uint64(sendReq.params.LastRoundValid-sendReq.params.FirstRoundValid))
	if err != nil {
		retReceipt.Error = fmt.Errorf("waiting for txn: %w", err)
		return retReceipt
	}
	retReceipt.Success.round = pendResponse.ConfirmedRound
	retReceipt.Success.txid = txnId
	return retReceipt
}

func sendAndWaitTxns(txnBytes []byte, waitRounds uint64) (models.PendingTransactionInfoResponse, error) {
	var (
		txid string
		resp models.PendingTransactionInfoResponse
		err  error
	)
	err = retryAlgoCalls(func() error {
		txid, err = algoClient.SendRawTransaction(txnBytes).Do(ctx)
		return err
	})
	if err != nil {
		return models.PendingTransactionInfoResponse{}, fmt.Errorf("sendAndWaitTxns failed to send txns: %w", err)
	}

	err = retryAlgoCalls(func() error {
		resp, err = transaction.WaitForConfirmation(algoClient, txid, waitRounds, ctx)
		return err
	})
	if err != nil {
		return models.PendingTransactionInfoResponse{}, fmt.Errorf("sendAndWaitTxns failure in confirmation wait: %w", err)
	}
	return resp, nil
}

func retryAlgoCalls(meth func() error) error {
	return repeat.Repeat(
		repeat.Fn(func() error {
			err := meth()
			if err != nil {
				errStr := err.Error()
				if strings.Contains(errStr, "429") || strings.Contains(errStr, "502") || strings.Contains(errStr, "503") {
					return repeat.HintTemporary(err)
				}
			}
			return err
		}),
		repeat.StopOnSuccess(),
		repeat.WithDelay(repeat.ExponentialBackoff(1*time.Second).Set()),
	)
}
