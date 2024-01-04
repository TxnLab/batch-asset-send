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

type TotalsOkResponseBodyMintedTotals struct {
	Day      int32 `json:"day,omitempty"`
	Lifetime int32 `json:"lifetime,omitempty"`
	Month    int32 `json:"month,omitempty"`
	Week     int32 `json:"week,omitempty"`
}
