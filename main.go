package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
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
	ctx                  = context.Background()
	algoClient           *algod.Client
	api                  *nfdapi.APIClient
	logger               *slog.Logger
	signer               algo.MultipleWalletSigner
	sendConfig           *BatchSendConfig
	vaultNfd             *nfdapi.NfdRecord
	sourceAccount        types.Address // the account we truly send from -used for fetching sender balances, etc.
	maxSimultaneousSends = 40
)

func main() {
	network := flag.String("network", "mainnet", "network: mainnet, testnet, betanet, or override w/ ALGO_XX env vars")
	sender := flag.String("sender", "", "account which has to sign all transactions - must have mnemonics in a ALGO_MNEMONIC_xx var")
	vault := flag.String("vault", "", "Don't send from sender account but from the named NFD vault that sender is owner of")
	config := flag.String("config", "send.json", "path to json config file specifying what to send and to what recipients")
	dryrun := flag.Bool("dryrun", false, "dryrun just shows what would've been sent but doesn't actually send")
	parallel := flag.Int("parallel", maxSimultaneousSends, "maximum number of sends to do at once - target node may limit")
	flag.Parse()
	maxSimultaneousSends = *parallel

	initLogger()
	ensureValidParams(*network, *sender)
	loadEnvironmentSettings()
	initSigner(*sender)   // also ensures we have mnemonics for it
	initClients(*network) // algod and nfd api

	// Get account balance info for sender for later...
	senderInfo, err := algo.GetBareAccount(ctx, algoClient, *sender)
	if err != nil {
		log.Fatalln(err)
	}

	sourceAccount, _ = types.DecodeAddress(*sender)
	misc.Infof(logger, "loading json config from:%s", *config)
	sendConfig, err = loadJSONConfig(*config)
	if err != nil {
		log.Fatalln("error loading json config from:", *config, "error:", err)
	}

	// if vault specified - make sure its valid and sender is owner
	if *vault != "" {
		fetchedNfd, _, err := api.NfdApi.NfdGetNFD(ctx, *vault, nil)
		if err != nil {
			log.Fatalln("vault nfd:", *vault, "error:", err)
		}
		if fetchedNfd.Owner != *sender {
			log.Fatalln("vault nfd:", *vault, "is not owned by sender:", *sender)
		}
		vaultNfd = &fetchedNfd
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
	misc.Infof(logger, "Want to send")
	for _, asset := range assetsToSend {
		misc.Infof(logger, "  %s", asset)
	}

	var (
		recipients []*Recipient
	)

	misc.Infof(logger, "Collecting data for config:%s", sendConfig.Destination.String())
	recipients, err = collectRecipients(sendConfig, vaultNfd)
	misc.Infof(logger, "Collected %d recipients", len(recipients))

	if !sendConfig.Destination.AllowDuplicateAccounts {
		// They don't want dupes !
		uniqRecipients := getUniqueRecipients(recipients)
		if len(uniqRecipients) != len(recipients) {
			misc.Infof(logger, "Reduced to %d UNIQUE deposit accounts", len(uniqRecipients))
			recipients = uniqRecipients
		}
	}

	// If sending to vaults, assume worst case of each needing opting in, so MBR + 4 total outer/inner txns
	// if not to vaults, just asset-transfer but if target not opted-in most txns will fail
	if sendConfig.Destination.SendToVaults {
		checkBalanceReqs(senderInfo, uint64(104000*len(recipients)))
	} else {
		checkBalanceReqs(senderInfo, uint64(1000*len(recipients)))
	}
	// Make sure the balances are acceptable
	verifyAssetBalances(assetsToSend, len(recipients))

	sortByDepositAccount(recipients)
	PromptForConfirmation("Are you sure you want to proceed? (y/n): ")
	sendAssets(*sender, assetsToSend, recipients, vaultNfd, *dryrun)
}

func checkBalanceReqs(senderInfo algo.AccountWithMinBalance, expectedFees uint64) {
	misc.Infof(logger, "Sending may cost a maximum of %s ALGO in fees", algo.FormattedAlgoAmount(expectedFees))
	if (senderInfo.Amount - senderInfo.MinBalance) < expectedFees {
		log.Fatalf("You only have %s (minus MBR) ALGO and likely won't be able to perform this airdrop", algo.FormattedAlgoAmount(senderInfo.Amount-senderInfo.MinBalance))
	}
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
		AssetID:          assetId,
		AssetParams:      assetInfo.Params,
		ExistingBalance:  holdingInfo.AssetHolding.Amount,
		AmountToSend:     config.Send.Asset.Amount,
		IsAmountPerRecip: config.Send.Asset.IsPerRecip,
	})
	return assetsToSend, nil
}

func verifyAssetBalances(send []*SendAsset, numRecipients int) {
	for _, asset := range send {
		balance := asset.ExistingBalance
		amountToSend := asset.AmountToSend
		if asset.IsAmountPerRecip {
			amountToSend *= float64(numRecipients)
		}
		if balance < asset.amountInBaseUnits(amountToSend) {
			log.Fatalf("Insufficient balance for asset %d (%s): Existing balance: %s, Amount to send: %f", asset.AssetID, asset.AssetParams.UnitName, asset.formattedAmount(balance), amountToSend)
		}
	}
}

func ensureValidParams(network string, sender string) {
	switch network {
	case "betanet", "testnet", "mainnet":
		return
	default:
		flag.Usage()
		log.Fatalln("unknown network:", network)
	}
}

func initLogger() {
	log.SetOutput(os.Stdout)
	logger = slog.Default()
}

func loadEnvironmentSettings() {
	misc.LoadEnvironmentSettings()
}

func initSigner(sender string) {
	signer = algo.NewLocalKeyStore(logger)
	if sender == "" {
		flag.Usage()
		log.Fatalln("You must specify a sender account!")
	}
	if !signer.HasAccount(sender) {
		log.Fatalf("The sender account:%s has no mnemonics specified.", sender)
	}
}

func initClients(network string) {
	cfg := algo.GetNetworkConfig(network)
	var err error
	algoClient, err = algo.GetAlgoClient(logger, cfg, maxSimultaneousSends)
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
