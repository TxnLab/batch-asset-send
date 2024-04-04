package main

import (
	"fmt"
	"log"
	"math/rand"
	"sort"

	"github.com/algorand/go-algorand-sdk/v2/types"

	"github.com/TxnLab/batch-asset-send/lib/misc"
	nfdapi "github.com/TxnLab/batch-asset-send/lib/nfdapi/swagger"
)

type Recipient struct {
	// For sending to NFD - just send to depositAccount if already opted-in, otherwise send to Vault.
	NfdName        string
	DepositAccount string
	SendToVault    bool
}

// collectRecipients collects recipients based on the given configuration and sendingFromVault record.
// If the number of recipients to pick is 0 or more than the number of available NFDs, it returns recipients from all NFDs.
// Otherwise, it returns recipients from randomly selected NFDs.
func collectRecipients(config *BatchSendConfig, sendingFromVault *nfdapi.NfdRecord) ([]*Recipient, error) {
	nfdsToChooseFrom, err := getNfdsToChooseFrom(config)
	if err != nil {
		return nil, err
	}

	numToPick := getNumToPick(config, nfdsToChooseFrom)

	if numToPick == 0 || len(nfdsToChooseFrom) <= numToPick {
		return getRecipientsFromAllNFds(config, nfdsToChooseFrom, sendingFromVault), nil
	}

	return getRecipientsFromRandomNFds(numToPick, config, nfdsToChooseFrom, sendingFromVault), nil
}

// Get unique recipients by deposit account (really only applicable if not sending to vaults)
func getUniqueRecipients(recipients []*Recipient) []*Recipient {
	uniqueRecipients := make(map[string]*Recipient)
	for _, recipient := range recipients {
		uniqueRecipients[recipient.DepositAccount] = recipient
	}

	uniqueRecipientsList := make([]*Recipient, 0, len(uniqueRecipients))
	for _, recipient := range uniqueRecipients {
		uniqueRecipientsList = append(uniqueRecipientsList, recipient)
	}

	return uniqueRecipientsList
}

func sortByDepositAccount(recipients []*Recipient) {
	// sort the recipients by deposit account
	sort.Slice(recipients, func(i, j int) bool {
		return recipients[i].DepositAccount < recipients[j].DepositAccount
	})
}

// getNfdsToChooseFrom retrieves the list of NfdRecord objects to choose from
// based on the provided BatchSendConfig. If the SegmentsOfRoot field is
// specified in the DestinationChoice of the config, it fetches the segments of
// the specified rootNfdName and returns them. It also checks if SendToVault is set
// and ensures that choice is passed through to filter out ineligible vaults (NFDs not upgraded or vault locked)
func getNfdsToChooseFrom(config *BatchSendConfig) ([]*nfdapi.NfdRecord, error) {
	var (
		nfdRecords []*nfdapi.NfdRecord
		err        error
	)
	if config.Destination.SegmentsOfRoot != "" {
		if config.Destination.RandomNFDs.OnlyRoots {
			log.Fatalln("configured to get segments of a root but then specified wanting only roots! This is an invalid configuration")
		}
		nfdRecords, err = getSegmentsOfRoot(config)
	} else {
		nfdRecords, err = getAllNfds(config)
	}
	if err != nil {
		return nil, fmt.Errorf("error in getNfdsToChooseFrom: %w", err)
	}
	misc.Infof(logger, "..total of %d NFDs found before next filter step", len(nfdRecords))
	return filterNfds(config, nfdRecords)
}

func filterNfds(config *BatchSendConfig, records []*nfdapi.NfdRecord) ([]*nfdapi.NfdRecord, error) {
	// do any additional filtering here, if necessary
	if len(config.Destination.VerifiedRequirements) == 0 {
		return records, nil
	}
	// Return only those nfds having ALL the specified verified requirements.
	var (
		filteredRecords            = make([]*nfdapi.NfdRecord, 0, len(records))
		vaultExcludedByVer         int
		vaultExcludedBecauseLocked int
		verifiedExcluded           int
	)
	for _, nfd := range records {
		if nfd.DepositAccount == "" {
			continue
		}
		if config.Destination.SendToVaults {
			var excluded bool
			// contract has to be at least 2.11 and not be locked for vault receipt
			if !IsContractVersionAtLeast(nfd.Properties.Internal["ver"], 2, 11) {
				vaultExcludedByVer++
				excluded = true
			}
			if IsVaultAutoOptInLockedForSender(nfd, types.ZeroAddress.String()) {
				vaultExcludedBecauseLocked++
				excluded = true
			}
			if excluded {
				continue
			}
		}

		verifiedProps := nfd.Properties.Verified
		if len(verifiedProps) > 0 {
			verified := true
			for _, requirement := range config.Destination.VerifiedRequirements {
				if _, ok := verifiedProps[requirement]; !ok {
					verified = false
					break
				}
			}
			if !verified {
				verifiedExcluded++
				continue
			}
		}
		filteredRecords = append(filteredRecords, nfd)
	}
	if vaultExcludedByVer > 0 || vaultExcludedBecauseLocked > 0 {
		misc.Infof(logger, "..vault requirement excluded:%d [NOT UPGRADED], and %d [LOCKED]", vaultExcludedByVer, vaultExcludedBecauseLocked)
	}
	if verifiedExcluded > 0 {
		misc.Infof(logger, "..filtered out %d NFDs due to verified requirements", verifiedExcluded)
	}
	return filteredRecords, nil
}

func getNumToPick(config *BatchSendConfig, nfdsToChooseFrom []*nfdapi.NfdRecord) int {
	numToPick := 0
	if config.Destination.RandomNFDs.Count != 0 {
		numToPick = config.Destination.RandomNFDs.Count
		misc.Infof(logger, "Choosing %d random NFDs out of %d", numToPick, len(nfdsToChooseFrom))
	}

	if len(nfdsToChooseFrom) <= numToPick {
		misc.Infof(logger, "..however, the number of nfds to choose from:%d is smaller, so just using all", len(nfdsToChooseFrom))
	}

	return numToPick
}

func getRecipientsFromAllNFds(config *BatchSendConfig, nfdsToChooseFrom []*nfdapi.NfdRecord, sendingFromVault *nfdapi.NfdRecord) []*Recipient {
	recips := make([]*Recipient, 0, len(nfdsToChooseFrom))
	for _, nfd := range nfdsToChooseFrom {
		if recip := createRecipient(config, nfd, sendingFromVault); recip != nil {
			recips = append(recips, recip)
		}
	}

	return recips
}

func getRecipientsFromRandomNFds(numToPick int, config *BatchSendConfig, nfdsToChooseFrom []*nfdapi.NfdRecord, sendingFromVault *nfdapi.NfdRecord) []*Recipient {
	// grab random unique nfdsToChooseFrom up through numToPick
	recipIndices := make(map[int]bool)
	for len(recipIndices) < numToPick {
		index := rand.Intn(len(nfdsToChooseFrom))
		recipIndices[index] = true
	}

	recips := make([]*Recipient, 0, numToPick)
	for index := range recipIndices {
		nfd := nfdsToChooseFrom[index]
		if recip := createRecipient(config, nfd, sendingFromVault); recip != nil {
			recips = append(recips, recip)
		}
	}

	return recips
}

func createRecipient(config *BatchSendConfig, destNfd *nfdapi.NfdRecord, sendingFromVault *nfdapi.NfdRecord) *Recipient {
	deposit := destNfd.DepositAccount
	if config.Destination.SendToVaults {
		deposit = destNfd.NfdAccount
		if sendingFromVault != nil && sendingFromVault.NfdAccount == deposit {
			return nil // don't send to self!
		}
	} else {
		// since not sending to a vault
	}
	return &Recipient{
		NfdName:        destNfd.Name,
		DepositAccount: deposit,
		SendToVault:    config.Destination.SendToVaults,
	}
}

//func collectRecipients(config *BatchSendConfig, sendingFromVault *nfdapi.NfdRecord) ([]*Recipient, error) {
//	var (
//		nfdsToChooseFrom []*nfdapi.NfdRecord
//		err              error
//		numToPick        int
//	)
//	if config.Destination.SegmentsOfRoot != "" {
//		nfdsToChooseFrom, err = getSegmentsOfRoot(config.Destination.SegmentsOfRoot, config.Destination.SendToVaults)
//		if err != nil {
//			return nil, err
//		}
//		if config.Destination.RandomNFDs.OnlyRoots {
//			// can't say only roots - but want segments of a root
//			log.Fatalln("configured to get segments of a root but then specified wanting only roots! This is an invalid configuration")
//		}
//	} else {
//		// Just grab all 'owned' nfdsToChooseFrom  - then filter off to those eligible for airdrops
//		nfdsToChooseFrom, err = getAllNfds(config.Destination.RandomNFDs.OnlyRoots, config.Destination.SendToVaults)
//		if err != nil {
//			return nil, err
//		}
//	}
//	if config.Destination.RandomNFDs.Count != 0 {
//		numToPick = config.Destination.RandomNFDs.Count
//		misc.Infof(logger, "Choosing %d random NFDs out of %d", numToPick, len(nfdsToChooseFrom))
//	}
//	if numToPick == 0 || len(nfdsToChooseFrom) <= numToPick {
//		if len(nfdsToChooseFrom) <= numToPick {
//			misc.Infof(logger, "..however, the number of nfds to choose from:%d is smaller, so just using all", len(nfdsToChooseFrom))
//		}
//		// we're not limiting the count (or num to choose from < than count they want) - so grab them all
//		recips := make([]*Recipient, 0, len(nfdsToChooseFrom))
//		for _, nfd := range nfdsToChooseFrom {
//			deposit := nfd.DepositAccount
//
//			if config.Destination.SendToVaults {
//				deposit = nfd.NfdAccount
//				if sendingFromVault != nil && sendingFromVault.NfdAccount == deposit {
//					continue // don't send to self.
//				}
//			}
//			recips = append(recips, &Recipient{
//				NfdName:        nfd.Name,
//				DepositAccount: deposit,
//				SendToVault:    config.Destination.SendToVaults,
//			})
//		}
//		return recips, nil
//	}
//	// grab random unique nfdsToChooseFrom up through numToPick
//	recipIndices := make(map[int]bool)
//	for len(recipIndices) < numToPick {
//		index := rand.Intn(len(nfdsToChooseFrom))
//		recipIndices[index] = true
//	}
//
//	recips := make([]*Recipient, 0, numToPick)
//	for index := range recipIndices {
//		nfd := nfdsToChooseFrom[index]
//		deposit := nfd.DepositAccount
//
//		if config.Destination.SendToVaults {
//			deposit = nfd.NfdAccount
//			if sendingFromVault != nil && sendingFromVault.NfdAccount == deposit {
//				continue // don't send to self.
//			}
//		}
//		recips = append(recips, &Recipient{
//			NfdName:        nfd.Name,
//			DepositAccount: deposit,
//			SendToVault:    config.Destination.SendToVaults,
//		})
//	}
//
//	return recips, nil
//}
