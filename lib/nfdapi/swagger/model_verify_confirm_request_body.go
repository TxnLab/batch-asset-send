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

type VerifyConfirmRequestBody struct {
	// Challenge value, optional depending on verification type
	Challenge string `json:"challenge,omitempty"`
}
