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

// Price breakdown on minting a segment off another NFD
type SegmentPriceResponseBody struct {
	// Price of ALGO in USD
	AlgoUsd float64 `json:"algoUsd"`
	// Algorand carry cost - amount required for MBR (Minimum Balance Requirement) of contracts, escrows, etc
	CarryCost int64 `json:"carryCost"`
	// Discount rate % that is applied for this segment name - 0 if discount point not reached - starting after 2500 NFDs
	DiscountRate float64 `json:"discountRate"`
	// Number of segments minted off of parent NFD
	ParentSegmentCount int64 `json:"parentSegmentCount"`
	// Total Price in microAlgo to mint including ALGO carry cost
	SellAmount int64 `json:"sellAmount"`
	// Price in USD for unlocked mint of this segment
	UnlockedSellPrice float64 `json:"unlockedSellPrice,omitempty"`
	// Minimum price in USD the segment has to be (not including ALGO carry cost).  If locked, the fixed price, or if unlocked, the platform price
	UsdMinCost float64 `json:"usdMinCost"`
}
