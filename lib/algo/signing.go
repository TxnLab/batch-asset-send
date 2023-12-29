package algo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/algod"
	"github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
	"github.com/algorand/go-algorand-sdk/v2/encoding/msgpack"
	"github.com/algorand/go-algorand-sdk/v2/transaction"
	"github.com/algorand/go-algorand-sdk/v2/types"
)

func decodeAndSignTransactions(keys *localKeyStore, txnResponse string) ([]byte, error) {
	type TxnPair [2]string
	var (
		txns []TxnPair
		err  error
		resp []byte
	)

	err = json.Unmarshal([]byte(txnResponse), &txns)
	if err != nil {
		return nil, err
	}
	for i, txn := range txns {
		rawBytes, err := base64.StdEncoding.DecodeString(txn[1])
		if err != nil {
			log.Fatal("Error decoding txn:", i, " error:", err)
		}
		bytes := decodeAndSignTransaction(keys, txn[0], rawBytes)
		resp = append(resp, bytes...)
	}
	return resp, nil
}

func decodeAndSignTransaction(keys *localKeyStore, txnType string, msgPackBytes []byte) []byte {
	var (
		uTxn types.Transaction
	)

	if txnType == "s" {
		return msgPackBytes
	}
	dec := msgpack.NewDecoder(bytes.NewReader(msgPackBytes))
	err := dec.Decode(&uTxn)
	if err != nil {
		log.Fatal("error in unmarshalling :", msgPackBytes, " error:", err)
	}
	_, bytes, err := keys.SignWithAccount(context.Background(), uTxn, uTxn.Sender.String())
	if err != nil {
		log.Fatal("error signing txn for sender:", uTxn.Sender.String(), "error:", err)
	}
	return bytes
}

func sendAndWaitTxns(ctx context.Context, log *slog.Logger, algoClient *algod.Client, txnBytes []byte) (models.PendingTransactionInfoResponse, error) {
	txid, err := algoClient.SendRawTransaction(txnBytes).Do(ctx)
	if err != nil {
		return models.PendingTransactionInfoResponse{}, fmt.Errorf("sendAndWaitTxns failed to send txns: %w", err)
	}
	log.Info("sendAndWaitTxns", "txid", txid)
	resp, err := transaction.WaitForConfirmation(algoClient, txid, 100, ctx)
	if err != nil {
		return models.PendingTransactionInfoResponse{}, fmt.Errorf("sendAndWaitTxns failure in confirmation wait: %w", err)
	}
	log.Info("sendAndWaitTxns", "confirmed-round", resp.ConfirmedRound)
	return resp, nil
}
