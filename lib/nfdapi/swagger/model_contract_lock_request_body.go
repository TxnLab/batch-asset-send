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

type ContractLockRequestBody struct {
	// Whether to lock (true), or unlock (false)
	Lock bool `json:"lock"`
	// Sender of transaction - needs to be owner of NFD
	Sender string `json:"sender"`
}
