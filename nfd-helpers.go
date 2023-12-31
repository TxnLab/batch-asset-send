package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/antihax/optional"
	"github.com/ssgreg/repeat"

	"github.com/TxnLab/batch-asset-send/lib/misc"
	nfdapi "github.com/TxnLab/batch-asset-send/lib/nfdapi/swagger"
)

func getAllNfds(onlyRoots bool) ([]*nfdapi.NfdRecord, error) {
	var (
		offset, limit int32 = 0, 200
		records       nfdapi.NfdV2SearchRecords
		err           error
		nfds          []*nfdapi.NfdRecord
	)

	fetchOp := func() error {
		start := time.Now()
		searchOpts := &nfdapi.NfdApiNfdSearchV2Opts{
			State:  optional.NewInterface("owned"),
			View:   optional.NewString("tiny"),
			Limit:  optional.NewInt32(limit),
			Offset: optional.NewInt32(offset),
		}
		if onlyRoots {
			searchOpts.Traits = optional.NewInterface("pristine")
		}
		records, _, err = api.NfdApi.NfdSearchV2(ctx, searchOpts)
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
		err = repeat.Repeat(repeat.Fn(fetchOp), repeat.StopOnSuccess())

		if err != nil {
			return nil, fmt.Errorf("error while fetching segments: %w", err)
		}

		if records.Nfds == nil || len(*records.Nfds) == 0 {
			break
		}
		for _, record := range *records.Nfds {
			if record.DepositAccount == "" {
				continue
			}
			newRecord := record
			nfds = append(nfds, &newRecord)
		}
	}
	return nfds, nil
}

func getSegmentsOfRoot(rootNfdName string) ([]*nfdapi.NfdRecord, error) {
	// Fetch root NFD - all we really want is its app id
	nfd, _, err := api.NfdApi.NfdGetNFD(ctx, rootNfdName, nil)
	if err != nil {
		log.Fatalln(err)
	}
	misc.Infof(logger, fmt.Sprintf("nfd app id for %s is:%v", nfd.Name, nfd.AppID))

	// brief view is fine for all segments... we just need depositAccount, owner, ...
	nfds, err := getAllSegments(ctx, nfd.AppID, "")
	if err != nil {
		log.Fatalln(err)
	}
	logger.Debug(fmt.Sprintf("fetched segments of root:%s, count:%d", rootNfdName, len(nfds)))
	return nfds, nil
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
			State:       optional.NewInterface("owned"),
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
			if record.DepositAccount == "" {
				continue
			}
			newRecord := record
			nfds = append(nfds, &newRecord)
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
