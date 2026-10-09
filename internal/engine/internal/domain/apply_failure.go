package domain

import "errors"

type RuleFailure struct {
	Step string
	Err  error
}

func (f *RuleFailure) Error() string { return f.Step + ": " + f.Err.Error() }

func (f *RuleFailure) Unwrap() error { return f.Err }

func AsRuleFailure(err error) (*RuleFailure, bool) {
	var failure *RuleFailure
	return failure, errors.As(err, &failure)
}

type ApplyFailure struct {
	PartitionID int
	EventID     string
	Position    int64
	Step        string
	ErrorText   string
}
