package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"math"
	"math/rand"
	"os"
	"strings"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/algod"
	"github.com/algorand/go-algorand-sdk/v2/types"

	"github.com/TxnLab/batch-asset-send/lib/algo"
	"github.com/TxnLab/batch-asset-send/lib/misc"
	nfdapi "github.com/TxnLab/batch-asset-send/lib/nfdapi/swagger"
)

// This is simple CLI - global vars here are fine... get over it.
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

	// Collect set of assets to send so we can determine distribution
	assetsToSend, err := fetchAssets(sendConfig)
	if err != nil {
		log.Fatalln(err)
	}
	if len(assetsToSend) == 0 {
		log.Fatalln("No assets to send")
	}
	logger.Info(fmt.Sprintf("want to send:%s", assetsToSend[0]))
	//PromptForConfirmation("Are you sure you want to proceed? (y/n): ")

	logger.Info("Collecting data for:", "config", sendConfig.Destination.String())
	recipients, err := collectRecipients(sendConfig)
	logger.Info(fmt.Sprintf("Collected %d recipients", len(recipients)))
}

func collectRecipients(config *BatchSendConfig) ([]*Recipient, error) {
	var (
		nfds      []*nfdapi.NfdRecord
		err       error
		numToPick int
	)
	if config.Destination.SegmentsOfRoot != "" {
		nfds, err = getSegmentsOfRoot(config.Destination.SegmentsOfRoot)
		if err != nil {
			return nil, err
		}
		if config.Destination.RandomNFDs.OnlyRoots {
			// can't say only roots - but want segments of a root
			log.Fatalln("configured to get segments of a root but then specified wanting only roots! This is an invalid configuration")
		}
	} else {
		// Just grab all 'owned' nfds  - then filter off to those eligible for airdrops
		nfds, err = getAllNfds(config.Destination.RandomNFDs.OnlyRoots)
		if err != nil {
			return nil, err
		}
	}
	if config.Destination.RandomNFDs.Count != 0 {
		numToPick = config.Destination.RandomNFDs.Count
		logger.Info(fmt.Sprintf("Random count of NFDs chosen, count:%d", numToPick))
	}
	if numToPick == 0 {
		// we're not limiting the count - so we're done
		recips := make([]*Recipient, 0, len(nfds))
		for _, nfd := range nfds {
			recips = append(recips, &Recipient{
				NfdName:        nfd.Name,
				DepositAccount: nfd.DepositAccount,
			})
		}
		return recips, nil
	}
	// grab random unique nfds up through numToPick
	recipIndices := make(map[int]bool)
	for len(recipIndices) < numToPick {
		index := rand.Intn(len(nfds))
		recipIndices[index] = true
	}

	recips := make([]*Recipient, 0, numToPick)
	for index := range recipIndices {
		nfd := nfds[index]
		recips = append(recips, &Recipient{
			NfdName:        nfd.Name,
			DepositAccount: nfd.DepositAccount,
		})
	}

	return recips, nil
}

type Recipient struct {
	// For sending to NFD - just send to depositAccount if already opted-in, otherwise send to Vault.
	NfdName        string
	DepositAccount string
	SendToVault    bool
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
	// Fetch/verify asset info user specified in send configuration
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

func PromptForConfirmation(prompt string) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print(prompt)
	text, _ := reader.ReadString('\n')
	text = strings.TrimSpace(text)
	if text != "y" && text != "Y" {
		log.Fatalln("Operation cancelled")
	}
}
