package algo

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/algod"
)

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
	log.Info(fmt.Sprintf("Connecting to Algorand node at:%s", serverAddr.String()))

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
