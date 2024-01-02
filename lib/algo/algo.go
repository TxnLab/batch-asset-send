package algo

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/algod"
	"github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/ssgreg/repeat"

	"github.com/TxnLab/batch-asset-send/lib/misc"
)

// Just do 100 max valid block range
const DefaultValidRoundRange = 100

func GetAlgoClient(log *slog.Logger, config NetworkConfig) (*algod.Client, error) {
	var (
		apiURL     string
		apiToken   string
		serverAddr *url.URL
		err        error
	)
	if config.NodeDataDir != "" {
		// Read address and token from main-net directory
		apiURL, apiToken, err = GetNetAndTokenFromFiles(
			filepath.Join(config.NodeDataDir, "algod.net"),
			filepath.Join(config.NodeDataDir, "algod.token"))
		if err != nil {
			return nil, fmt.Errorf("error reading config: %w", err)
		}
	} else {
		apiURL = config.NodeURL
		apiToken = config.NodeToken
		// Strip off trailing slash if present in url which the Algorand client doesn't handle properly
		apiURL = strings.TrimRight(apiURL, "/")
	}
	serverAddr, err = url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse url:%v, error:%w", apiURL, err)
	}
	if serverAddr.Scheme == "tcp" {
		serverAddr.Scheme = "http"
	}
	misc.Infof(log, "Connecting to Algorand node at:%s", serverAddr.String())

	// Override the default transport so we can properly support multiple parallel connections to same
	// host (and allow connection resuse)
	customTransport := http.DefaultTransport.(*http.Transport).Clone()
	customTransport.MaxIdleConns = 100
	customTransport.MaxConnsPerHost = 100
	customTransport.MaxIdleConnsPerHost = 100
	client, err := algod.MakeClientWithTransport(serverAddr.String(), apiToken, nil, customTransport)
	if err != nil {
		return nil, fmt.Errorf(`failed to make algod client (url:%s), error:%w`, serverAddr.String(), err)
	}
	// Immediately hit server to verify connectivity
	_, err = client.SuggestedParams().Do(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get suggested params from algod client, error:%w", err)
	}
	return client, nil
}

func SuggestedParams(ctx context.Context, logger *slog.Logger, client *algod.Client) types.SuggestedParams {
	var (
		txParams types.SuggestedParams
		err      error
	)
	// don't accept no for an answer from this api... just keep trying..
	err = repeat.Repeat(
		repeat.Fn(func() error {
			txParams, err = client.SuggestedParams().Do(ctx)
			if err != nil {
				return repeat.HintTemporary(err)
			}
			return nil
		}),
		repeat.StopOnSuccess(),
		repeat.FnOnError(func(err error) error {
			misc.Infof(logger, "retrying suggestedparams call, error:%s", err.Error())
			return err
		}),
		repeat.WithDelay(repeat.ExponentialBackoff(1*time.Second).Set()),
	)

	// move FirstRoundValid back 1 just to cover for different nodes maybe being 'slightly' behind - so we
	// don't create a transaction starting at round 100 but the node we submit to is only at round 99
	txParams.FirstRoundValid--
	txParams.LastRoundValid = txParams.FirstRoundValid + DefaultValidRoundRange
	// Just set fixed fee for now - we don't want to send during high cost periods anyway.
	txParams.FlatFee = true
	txParams.Fee = types.MicroAlgos(txParams.MinFee)
	return txParams
}
