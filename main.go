package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"math"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/algod"
	"github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/antihax/optional"
	"github.com/ssgreg/repeat"

	"github.com/TxnLab/batch-asset-send/lib/algo"
	"github.com/TxnLab/batch-asset-send/lib/misc"
	nfdapi "github.com/TxnLab/batch-asset-send/lib/nfdapi/swagger"
)

var (
	ctx           = context.Background()
	algoClient    *algod.Client
	api           *nfdapi.APIClient
	logger        *slog.Logger
	signer        algo.MultipleWalletSigner
	sendConfig    *BatchSendConfig
	vaultNfd      nfdapi.NfdRecord
	sourceAccount types.Address // the account we truly send from -used for fetching sender balances, etc.
)

func main() {
	network := flag.String("network", "mainnet", "network: mainnet, testnet, betanet, or override w/ ALGO_XX env vars")
	sender := flag.String("sender", "", "account which has to sign all transactions - must have mnemonics in a ALGO_MNEMONIC_xx var")
	vault := flag.String("vault", "", "Don't send from sender account but from the named NFD vault that sender is owner of")
	config := flag.String("config", "send.json", "path to json config file specifying what to send and to what recipients")
	flag.Parse()

	initLogger()
	ensureValidParams(*network, *sender)
	loadEnvironmentSettings()
	initSigner(*sender)   // also ensures we have mnemonics for it
	initClients(*network) // algod and nfd api

	sourceAccount, _ = types.DecodeAddress(*sender)

	var err error
	sendConfig, err = loadJSONConfig(*config)
	if err != nil {
		log.Fatalln("error loading json config from:", *config, "error:", err)
	}

	// if vault specified - make sure its valid and sender is owner
	if *vault != "" {
		vaultNfd, _, err = api.NfdApi.NfdGetNFD(ctx, *vault, nil)
		if err != nil {
			log.Fatalln("vault nfd:", *vault, "error:", err)
		}
		if vaultNfd.Owner != *sender {
			log.Fatalln("vault nfd:", *vault, "is not owned by sender:", *sender)
		}
		// set 'source account' to the vault account
		sourceAccount, _ = types.DecodeAddress(vaultNfd.NfdAccount)
	}

	assetsToSend, err := fetchAssets(sendConfig)
	if err != nil {
		log.Fatalln(err)
	}
	logger.Info(fmt.Sprintf("want to send:%s", assetsToSend[0]))

	//// Get app id of specific nfd
	//nfd, _, err := api.NfdApi.NfdGetNFD(ctx, "barb.algo", nil)
	//if err != nil {
	//	log.Fatalln(err)
	//}
	//logger.Info(fmt.Sprintf("nfd app id for barb.algo is:%v", nfd.AppID))
	//
	//nfds, err := getAllSegments(ctx, nfd.AppID, "brief")
	//if err != nil {
	//	log.Fatalln(err)
	//}
	//logger.Info("fetched segments", "count", len(nfds))
}

type SendAsset struct {
	AssetID           uint64
	Decimals          uint64
	ExistingBalance   uint64
	WholeAmountToSend uint64
	IsAmountPerRecip  bool
}

// write String method for SendAsset
func (a *SendAsset) String() string {
	return fmt.Sprintf("AssetID: %d, Decimals: %d, ExistingBalance: %s, WholeAmountToSend: %d, IsAmountPerRecip: %t",
		a.AssetID, a.Decimals,
		formattedAmount(a.ExistingBalance, a.Decimals),
		a.WholeAmountToSend,
		//formattedAmount(uint64(float64(a.WholeAmountToSend)*math.Pow10(int(a.Decimals))), a.Decimals),
		a.IsAmountPerRecip)
}

func formattedAmount(amount, decimals uint64) string {
	return fmt.Sprintf("%.*f", decimals, float64(amount)/math.Pow10(int(decimals)))
}

func fetchAssets(config *BatchSendConfig) ([]*SendAsset, error) {
	// using algorand sdk via algoClient, fetch the asset specified by config.Send.Asset.ASA into SendAsset struct
	assetsToSend := []*SendAsset{}
	assetId := sendConfig.Send.Asset.ASA
	assetInfo, err := algoClient.GetAssetByID(assetId).Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("error fetching asset info for ASA:%d, err:%w", assetId, err)
	}

	holdingInfo, err := algoClient.AccountAssetInformation(sourceAccount.String(), assetId).Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("error fetching asset info for ASA:%d from account:%s, err:%w", assetId, sourceAccount.String(), err)
	}
	assetsToSend = append(assetsToSend, &SendAsset{
		AssetID:           assetId,
		Decimals:          assetInfo.Params.Decimals,
		ExistingBalance:   holdingInfo.AssetHolding.Amount,
		WholeAmountToSend: config.Send.Asset.WholeAmount,
		IsAmountPerRecip:  config.Send.Asset.IsPerRecip,
	})
	return assetsToSend, nil
}

func ensureValidParams(network string, sender string) {
	switch network {
	case "betanet", "testnet", "mainnet":
		return
	default:
		flag.Usage()
		log.Fatalln("unknown network:", network)
	}

	_, err := types.DecodeAddress(sender)
	if err != nil {
		flag.Usage()
		log.Fatalln("invalid sender address:", err)
	}
}

func initLogger() {
	logger = slog.Default()
}

func loadEnvironmentSettings() {
	misc.LoadEnvironmentSettings()
}

func initSigner(from string) {
	signer = algo.NewLocalKeyStore(logger)
	if from == "" {
		log.Fatalln("must specify from account!")
	}
	// TODO add back later
	//if !signer.HasAccount(from) {
	//	log.Fatalf("The from account:%s has no mnemonics specified.", from)
	//}
}

func initClients(network string) {
	cfg := algo.GetNetworkConfig(network)
	var err error
	algoClient, err = algo.GetAlgoClient(logger, cfg)
	if err != nil {
		log.Fatalln(err)
	}
	nfdApiCfg := nfdapi.NewConfiguration()
	nfdApiCfg.BasePath = cfg.NFDAPIUrl
	api = nfdapi.NewAPIClient(nfdApiCfg)
}

func getAllSegments(ctx context.Context, parentAppID int64, view string) ([]*nfdapi.NfdRecord, error) {
	var (
		offset, limit int32 = 0, 200
		records       nfdapi.NfdV2SearchRecords
		err           error
		nfds          []*nfdapi.NfdRecord
	)

	if view == "" {
		view = "brief"
	}
	searchOp := func() error {
		start := time.Now()
		records, _, err = api.NfdApi.NfdSearchV2(ctx, &nfdapi.NfdApiNfdSearchV2Opts{
			ParentAppID: optional.NewInt64(parentAppID),
			View:        optional.NewString(view),
			Limit:       optional.NewInt32(limit),
			Offset:      optional.NewInt32(offset),
		})
		if err != nil {
			if rate, match := isRateLimited(err); match {
				logger.Warn("rate limited", "cur length", len(nfds), "responseDelay", time.Since(start), "waiting", rate.SecsRemaining)
				time.Sleep(time.Duration(rate.SecsRemaining+1) * time.Second)
				return repeat.HintTemporary(err)
			}
			return err
		}
		return err
	}

	for ; ; offset += limit {
		err = repeat.Repeat(repeat.Fn(searchOp), repeat.StopOnSuccess())

		if err != nil {
			return nil, fmt.Errorf("error while fetching segments: %w", err)
		}

		if records.Nfds == nil || len(*records.Nfds) == 0 {
			break
		}
		for _, record := range *records.Nfds {
			nfds = append(nfds, &record)
		}
	}
	return nfds, nil
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
