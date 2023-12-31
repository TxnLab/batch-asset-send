package main

import (
	"fmt"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
	"github.com/algorand/go-algorand-sdk/v2/transaction"
	"github.com/algorand/go-algorand-sdk/v2/types"
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

func sendAssetToRecipient(sender string, params *types.SuggestedParams, send *SendAsset, baseUnitsToSend uint64, recipient *Recipient) *RecipientTransaction {
	retReceipt := &RecipientTransaction{
		sendAsset:       send,
		baseUnitsToSend: baseUnitsToSend,
		recip:           recipient}

	txParams := types.SuggestedParams{}
	txParams, err := algoClient.SuggestedParams().Do(ctx)
	if err != nil {
		retReceipt.Error = fmt.Errorf("SuggestedParams fail: %w", err)
		return retReceipt
	}
	txn, err := transaction.MakeAssetTransferTxn(sender, recipient.DepositAccount, baseUnitsToSend, nil, txParams, "", send.AssetID)
	if err != nil {
		retReceipt.Error = fmt.Errorf("MakeAssetTransferTxn fail: %w", err)
		return retReceipt
	}
	txnid, signedBytes, err := signer.SignWithAccount(ctx, txn, sender)
	if err != nil {
		retReceipt.Error = fmt.Errorf("SignWithAccount fail: %w", err)
		return retReceipt
	}
	//sendandw
	//_, err = algoClient.SendRawTransaction(txn).Do(ctx)
	//if err != nil {
	//	return err
	//}
	return nil
}

func sendAndWaitTxns(txnBytes []byte) (models.PendingTransactionInfoResponse, error) {
	txid, err := algoClient.SendRawTransaction(txnBytes).Do(ctx)
	if err != nil {
		return models.PendingTransactionInfoResponse{}, fmt.Errorf("sendAndWaitTxns failed to send txns: %w", err)
	}
	logger.Info("sendAndWaitTxns", "txid", txid)
	resp, err := transaction.WaitForConfirmation(algoClient, txid, 100, ctx)
	if err != nil {
		return models.PendingTransactionInfoResponse{}, fmt.Errorf("sendAndWaitTxns failure in confirmation wait: %w", err)
	}
	logger.Info("sendAndWaitTxns", "confirmed-round", resp.ConfirmedRound)
	return resp, nil
}
