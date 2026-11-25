package task

import "fmt"

// TW_STORES_CHLSJ is a placeholder for the missing Total Wine store-lookup
// Charles session JSON export. The original constant was not included in this
// source dump; it is left empty so the package compiles. Runtime use of the
// Total Wine store path will fail until real data is supplied.
const TW_STORES_CHLSJ = ""

// GsCheckout is a stub for the missing greenspaces.id checkout implementation.
// The original method was not included in this source dump.
func (t *CheckoutTask) GsCheckout() error {
	return fmt.Errorf("greenspaces.id checkout not implemented")
}
