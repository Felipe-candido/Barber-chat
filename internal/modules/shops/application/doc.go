// Package application is the home of shop use cases.
//
// Planned workflows, not implemented yet:
//   - CreateShop: create the tenant, unique slug and initial timezone.
//   - UpdateShop: change public information and validated settings.
//   - GetPublicShop: resolve an active slug and return public shop information.
//
// Catalog data is composed through a narrow read port when needed; this package
// does not take ownership of services or professionals. Shop creation and the
// owner's first membership must eventually share one transaction.
//
// A timezone change must coordinate with booking: existing appointment instants
// stay fixed, while future availability and pending reminder policy need review.
// Administrative writes receive an authorized tenant scope, not a trusted
// shop_id copied from an HTTP body.
//
// Add one file per workflow, such as get_public_shop.go. Define only the ports
// used by that workflow; wire concrete adapters in cmd. No workflow runs yet.
package application
