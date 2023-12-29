package main

import (
	"encoding/json"
	"io"
	"os"
)

type DestinationChoice struct {
	// If filename specified, then is explicit list of destinations and assumed asset send choice determines what is sent to each.
	CSVFilename string `json:"csvFilename"`
	// If it should only be sent to segments of specified Root
	SegmentsOfRoot string `json:"segmentsOfRoot"`
	// If RandomNFDs is filled out then target isn't 'all' it's random in some way
	// so if SegmentsOfRoot is set but RandomNFDs.xxx isn't then it's all
	RandomNFDs struct {
		// Only send to X number of nfds - not all
		Count int `json:"count"`
		// If doing random, but SegmentsOfRoot isn't set then only pick random roots
		OnlyRoots bool `json:"onlyRoots"`
	} `json:"randomNfDs"`
}

type AssetChoice struct {
	// If specifying a 'list' of assets to send - if so, assumed '1' base unit per chosen recipient (ie: 1 nft)
	//CSVFilename string `json:"csvFilename"`

	Asset struct {
		ASA uint64 `json:"asa"`
		// If IsPerRcp is NOT set then this is the TOTAL amount to send - and will be divided across destination
		// count - if IsPerRcp is set then amount is amount per recipient
		// Specified in whole units - not base units - ie 1 ALGO would be 1, not 1,000,000
		WholeAmount uint64 `json:"amount"`
		// Is the amount 'per recipient' or is it total amount to send.
		IsPerRecip bool `json:"isPerRecip"`
	} `json:"asset"`
}

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

type DestinationCSVData struct {
	Account string
}

//
//func main() {
//	csvFile, _ := os.Open("path_to_your_file.csv")
//	r := csv.NewReader(csvFile)
//	r.Comment = '#'
//
//	var accounts []DestinationCSVData
//	for {
//		line, error := r.Read()
//		if error != nil {
//			fmt.Println("End of file")
//			break
//		}
//		accounts = append(accounts, DestinationCSVData{
//			Account: line[0],
//		})
//	}
//	fmt.Println(accounts)
//}
