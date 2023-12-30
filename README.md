# batch-asset-send
Distribute asset(s) to Algorand accounts, NFDs, NFD Vaults, NFD segments, etc.

## Use of NFD Api

The NFD API client was generated via the interactive swagger link from https://api-docs.nf.domains using Generate Client->Go.
The downloaded library was inserted into the lib/nfdapi/swagger directory with minimal modifications.  

A simple example is that at least for the Go generated code, some generated types used int32 when they should be at least int64/uint64.
  The asa/app id fields for example.

