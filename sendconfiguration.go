package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type DestinationChoice struct {
	// If it should only be sent to segments of specified Root
	SegmentsOfRoot string `json:"segmentsOfRoot"`
	// If RandomNFDs is filled out then target isn't 'all' it's random in some way
	// so if SegmentsOfRoot is set but RandomNFDs.xxx isn't then it's all
	RandomNFDs struct {
		// Only send to X number of nfds - not all
		Count int `json:"count"`
	} `json:"randomNFDs"`

	// Ignore segments, only pick roots
	OnlyRoots bool `json:"onlyRoots"`

	SendToVaults bool `json:"sendToVaults"`

	// Only send if v.XXXX is present in NFD (ie: verifiedRequirements: ["twitter"] would require v.twitter to be set)
	VerifiedRequirements []string `json:"verifiedRequirements,omitempty"`

	// If user w/ single account owns 10 eligible NFDS do they get 10 drops or just 1.  Defaults to just going to
	// unique owner accounts.  Leave as false (default) to send '1' per nfd regardless
	AllowDuplicateAccounts bool `json:"allowDuplicateAccounts"`
}

func (dc DestinationChoice) String() string {
	var sb strings.Builder
	if dc.SendToVaults {
		sb.WriteString("Sending TO vaults, ")
	}
	if dc.SegmentsOfRoot != "" {
		sb.WriteString(fmt.Sprintf("Segments of root:%s, ", dc.SegmentsOfRoot))
	}
	if dc.OnlyRoots {
		sb.WriteString(fmt.Sprintf("Grabbing 'roots' only, "))
	}
	if dc.RandomNFDs.Count != 0 {
		sb.WriteString(fmt.Sprintf("Limited to maximum of %d recipients", dc.RandomNFDs.Count))
	}
	if len(dc.VerifiedRequirements) > 0 {
		sb.WriteString(fmt.Sprintf("Verified (v.*) requirements: %v, ", dc.VerifiedRequirements))
	}
	if dc.SegmentsOfRoot == "" && !dc.OnlyRoots && dc.RandomNFDs.Count == 0 && len(dc.VerifiedRequirements) == 0 {
		sb.WriteString("Sending to ALL owned NFDs")
	}
	return sb.String()
}

type AssetChoice struct {
	// If specifying a 'list' of assets to send - if so, assumed '1' base unit per chosen recipient (ie: 1 nft)
	//CSVFilename string `json:"csvFilename"`

	Asset struct {
		ASA uint64 `json:"asa"`
		// If IsPerRcp is NOT set then this is the TOTAL amount to send - and will be divided across destination
		// count - if IsPerRcp is set then amount is amount per recipient
		// Specified in user-friendly units - not base units - ie 1.5 ALGO would be 1.5, not 1,500,000
		Amount float64 `json:"amount"`
		// Is the amount 'per recipient' or is it total amount to send.
		IsPerRecip bool `json:"isPerRecip"`
	} `json:"asset"`
}

//  1.000000 ALGO -> 1,000,000 microAlgo

type BatchSendConfig struct {
	Send AssetChoice `json:"send"`

	Destination DestinationChoice `json:"destination"`
}

func loadJSONConfig(filename string) (*BatchSendConfig, error) {
	jsonFile, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer jsonFile.Close()
	fileBytes, err := io.ReadAll(jsonFile)
	if err != nil {
		return nil, err
	}

	var data BatchSendConfig
	if err := json.Unmarshal(fileBytes, &data); err != nil {
		return nil, err
	}

	return &data, nil
}
