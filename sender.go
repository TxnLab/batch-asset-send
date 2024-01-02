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

	"github.com/TxnLab/batch-asset-send/lib/algo"
	"github.com/TxnLab/batch-asset-send/lib/misc"
)

const MaxSimultaneousSends = 40

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
	var retStr strings.Builder
	retStr.WriteString(fmt.Sprintf("Recipient: %s, Asset ID: %d, Amount: %s, ",
		rt.recip.DepositAccount,
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
	sender    string
	params    types.SuggestedParams
	asset     SendAsset
	amount    uint64
	recipient Recipient
}

func sendAssets(sender string, send []*SendAsset, recipients []*Recipient) {
	var (
		sendRequests = make(chan SendRequest, MaxSimultaneousSends)
		sendResults  = make(chan *RecipientTransaction, MaxSimultaneousSends)
		fanOut       = syncutil.NewFanOut(MaxSimultaneousSends)
		wg           sync.WaitGroup
		successes    int
		failures     int
		startTime    = time.Now()
	)
	// ensure file appending is possible
	appendToFile("Starting", "failure.txt")
	appendToFile("Starting", "success.txt")
	// Queues to sendResults then closes once done
	go QueueSends(sendRequests, send, sender, recipients)
	// Handle parallel results that will soon be coming from the parallel sends
	wg.Add(1)
	go func() {
		defer wg.Done()
		for result := range sendResults {
			misc.Infof(logger, "Send result:%s", result.String())
			// save off to separate files - success, failure
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
			sendResults <- sendAssetToRecipient(sender, &sendReq.params, &sendReq.asset, sendReq.amount, &sendReq.recipient)
			return nil
		}, send)
	}
	fanOut.Wait() // returns once all results are queued..
	wg.Wait()     // now wait to have processed them all.
	if failures > 0 {
		misc.Infof(logger, "%d successful sends", successes)
		misc.Infof(logger, "%d FAILED sends - check failure.txt for issues", failures)
	} else {
		misc.Infof(logger, "All %d sends successful", successes)
	}
	misc.Infof(logger, "Elapsed time:%v", time.Since(startTime))
}

// appendToFile adds the specified message as a new line to the specified file
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

func QueueSends(sendRequests chan SendRequest, send []*SendAsset, sender string, recipients []*Recipient) {
	var (
		// Get new params every 30 secs or so
		txParams = algo.SuggestedParams(ctx, logger, algoClient)
		ticker   = time.NewTicker(30 * time.Second)
	)
	for _, asset := range send {
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
			// queue the send
			sendRequests <- SendRequest{
				sender:    sender,
				params:    txParams,
				asset:     *asset,
				amount:    baseUnitAmount,
				recipient: *recipient,
			}
		}
	}
	close(sendRequests)
}

func sendAssetToRecipient(sender string, params *types.SuggestedParams, send *SendAsset, baseUnitsToSend uint64, recipient *Recipient) *RecipientTransaction {
	retReceipt := &RecipientTransaction{
		sendAsset:       send,
		baseUnitsToSend: baseUnitsToSend,
		recip:           recipient}

	txn, err := transaction.MakeAssetTransferTxn(sender, recipient.DepositAccount, baseUnitsToSend, nil, *params, "", send.AssetID)
	if err != nil {
		retReceipt.Error = fmt.Errorf("MakeAssetTransferTxn fail: %w", err)
		return retReceipt
	}
	txnid, signedBytes, err := signer.SignWithAccount(ctx, txn, sender)
	if err != nil {
		retReceipt.Error = fmt.Errorf("SignWithAccount fail: %w", err)
		return retReceipt
	}
	pendResponse, err := sendAndWaitTxns(signedBytes, uint64(txn.LastValid-txn.FirstValid))
	if err != nil {
		retReceipt.Error = fmt.Errorf("waiting for txn: %w", err)
		return retReceipt
	}
	retReceipt.Success.round = pendResponse.ConfirmedRound
	retReceipt.Success.txid = txnid
	return nil
}

func sendAndWaitTxns(txnBytes []byte, waitRounds uint64) (models.PendingTransactionInfoResponse, error) {
	txid, err := algoClient.SendRawTransaction(txnBytes).Do(ctx)
	if err != nil {
		return models.PendingTransactionInfoResponse{}, fmt.Errorf("sendAndWaitTxns failed to send txns: %w", err)
	}
	resp, err := transaction.WaitForConfirmation(algoClient, txid, waitRounds, ctx)
	if err != nil {
		return models.PendingTransactionInfoResponse{}, fmt.Errorf("sendAndWaitTxns failure in confirmation wait: %w", err)
	}
	return resp, nil
}
