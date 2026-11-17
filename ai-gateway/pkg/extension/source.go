package extension

// Source names where a call's selected model and its provider keys come from.
// It is informational: the set may grow, so extensions decide where they apply
// by key ownership ([Definition.Keys]), not by branching on the source.
type Source string

const (
	// SourceOperatorCatalog is the operator's configured model catalog, with
	// server-held provider keys.
	SourceOperatorCatalog Source = "operator-catalog"
	// SourceRequestBYOK is the request's own provider accounts.
	SourceRequestBYOK Source = "request-byok"
	// SourceStackCatalog is a stack's own model catalog, with the stack's keys.
	SourceStackCatalog Source = "stack-catalog"
)

// Keys returns who owns the provider keys of a call from the source:
// [KeysOperator] for [SourceOperatorCatalog] and [KeysCustomer] for every
// other source, including ones added later.
func (s Source) Keys() KeyOwnership {
	if s == SourceOperatorCatalog {
		return KeysOperator
	}
	return KeysCustomer
}

// KeyOwnership names who owns the provider keys a call uses.
type KeyOwnership string

const (
	// KeysOperator is a call on the operator's provider keys.
	KeysOperator KeyOwnership = "operator"
	// KeysCustomer is a call on the customer's provider keys.
	KeysCustomer KeyOwnership = "customer"
)
