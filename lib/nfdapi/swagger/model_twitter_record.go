/*
 * NFD Management Service
 *
 * Service for querying and managing NFDs
 *
 * API version: 1.0
 * Contact: feedback@txnlab.dev
 * Generated by: Swagger Codegen (https://github.com/swagger-api/swagger-codegen.git)
 */
package swagger

import (
	"time"
)

// TwitterRecord contains information about an NFD w/ Verified Twitter account and basic info on its twitter metrics
type TwitterRecord struct {
	Followers     int32     `json:"followers"`
	Following     int32     `json:"following"`
	Nfd           *Nfd      `json:"nfd"`
	TimeChanged   time.Time `json:"timeChanged"`
	Tweets        int32     `json:"tweets"`
	TwitterHandle string    `json:"twitterHandle"`
}
