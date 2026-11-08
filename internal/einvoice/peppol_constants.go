// Copyright 2024 Patrick Gundlach, Berlin
// SPDX-License-Identifier: BSD-3-Clause
// Adapted from github.com/speedata/einvoice; see NOTICE.md.

package einvoice

// SpecPEPPOLBilling30 identifies the PEPPOL BIS Billing 3.0 invoice profile.
// Only the profile identifier is needed for the writer's profile hierarchy;
// EAS registries and the upstream validation engine are outside this package.
const SpecPEPPOLBilling30 = "urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0"

// isPEPPOL checks the specification identifier (BT-24).
func (inv *Invoice) isPEPPOL() bool {
	return inv.GuidelineSpecifiedDocumentContextParameter == SpecPEPPOLBilling30
}
