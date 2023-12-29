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

type EscrowOfferRequestBody struct {
	Buyer string `json:"buyer"`
	// WholeAmount in microAlgo to escrow as new floor price
	Offer int32 `json:"offer"`
}
