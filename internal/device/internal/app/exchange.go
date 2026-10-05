package app

import "maps"

// clone copies the records' documents, so a caller cannot change what the
// session cached.
func (e Exchange) clone() Exchange {
	e.Receipt.Document = maps.Clone(e.Receipt.Document)
	e.Result.Document = maps.Clone(e.Result.Document)
	e.Receipt.RejectCode = cloneCode(e.Receipt.RejectCode)
	e.Result.ErrorCode = cloneCode(e.Result.ErrorCode)
	return e
}

func cloneCode(code *string) *string {
	if code == nil {
		return nil
	}
	value := *code
	return &value
}

// document preserves the provider result contract, including absent replies.
func (e Exchange) document() map[string]any {
	return map[string]any{"receipt": e.Receipt.Document, "result": e.Result.Document}
}
