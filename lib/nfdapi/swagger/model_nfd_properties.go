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

// NFDProperties contains the expanded metadata stored within an NFD contracts' global-state
type NfdProperties struct {
	// Internal properties
	Internal map[string]string `json:"internal,omitempty"`
	// User properties
	UserDefined map[string]string `json:"userDefined,omitempty"`
	// Verified properties
	Verified map[string]string `json:"verified,omitempty"`
}
